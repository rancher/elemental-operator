#!/usr/bin/env bash
# Prepares the charts of a release for the ship-family-staged action of rancher/charts.
#
# OBS builds the two charts of Elemental and publishes them to registry.suse.com as
# rancher/elemental-operator-chart and rancher/elemental-operator-crds-chart. This
# pulls the two finished charts of a release from there, copies each to
# <output dir>/<chart>, and gives them the names and annotations that the Rancher
# catalog has: elemental and elemental-crd. The action stamps, checks, packages and
# pushes the copies to the staging registry of Rancher.
#
# Usage: prepare-charts.sh <tag> <rancher minor> <output dir>
#   e.g. prepare-charts.sh v1.9.3 2.16 "$RUNNER_TEMP/charts"
#
# The tag is a release, vX.Y.Z: it is the version of the charts that OBS published.
# The Rancher minor version is to define the target Rancher minor the generated
# chart is targeting. It defines the rancher-version annotation, and the kube-version
# annotation, which is the one that rancher-versions.yaml (next to this script) gives for
# that minor. A minor with no entry there is refused.
#
# Environment:
#   OBS_REGISTRY  where OBS published the charts (default registry.suse.com)
#
# It changes the names, the chart that is installed first, the version placeholder, and
# the rancher-version and kube-version annotations, and nothing else: the other annotations of
# the chart are the ones OBS published. The catalog needs both bounds in each of the two, as in
# ">= 1.33.0-0 < 1.38.0-0", and the chart major of the catalog is worked out from the
# rancher-version. The checks of cargueiro refuse a chart without them.
#
# Requires helm and yq on PATH.
set -euo pipefail

TAG=${1:?usage: $0 <tag> <rancher minor> <output dir>}
RANCHER_MINOR=${2:?usage: $0 <tag> <rancher minor> <output dir>}
OUT_DIR=${3:?usage: $0 <tag> <rancher minor> <output dir>}
OBS_REGISTRY=${OBS_REGISTRY:-registry.suse.com}

if ! [[ "$TAG" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "Error: tag $TAG must look like vX.Y.Z: only releases are staged" >&2
  exit 1
fi
APP_VERSION="${BASH_REMATCH[1]}"

if ! [[ "$RANCHER_MINOR" =~ ^2\.([0-9]+)$ ]]; then
  echo "Error: Rancher minor $RANCHER_MINOR} must look like 2.X: rancher-version is required" >&2
  exit 1
fi
R_VER_NEXT="${BASH_REMATCH[1]}"
R_VER=$((R_VER_NEXT++))

# The Kubernetes versions that Elemental supports on this Rancher minor, which the Elemental
# team decides in rancher-versions.yaml, next to this script.
CONFIG="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/rancher-versions.yaml"
KUBE_VERSION=$(RM="$RANCHER_MINOR" yq e '.rancher-versions[strenv(RM)].kube-version // ""' "$CONFIG")
if [ -z "$KUBE_VERSION" ]; then
  echo "Error: ${CONFIG##*/} has no kube-version for Rancher $RANCHER_MINOR: add its entry, with the Kubernetes versions that Elemental supports on it, before staging for it" >&2
  exit 1
fi

# The two charts of the release: the repository OBS published each to, the name it has
# there, and the name it has in the Rancher catalog.
OBS_CHARTS=(rancher/elemental-operator-chart rancher/elemental-operator-crds-chart)
OBS_NAMES=(elemental-operator elemental-operator-crds)
NAMES=(elemental elemental-crd)

mkdir -p "$OUT_DIR"
pulled=$(mktemp -d)
trap 'rm -rf "$pulled"' EXIT

for i in "${!OBS_CHARTS[@]}"; do
  helm pull "oci://${OBS_REGISTRY}/${OBS_CHARTS[$i]}" --version "$APP_VERSION" --untar --untardir "$pulled"
  if [ ! -f "$pulled/${OBS_NAMES[$i]}/Chart.yaml" ]; then
    echo "Error: ${OBS_CHARTS[$i]}:${APP_VERSION} has no chart called ${OBS_NAMES[$i]}" >&2
    exit 1
  fi
  dir="$OUT_DIR/${NAMES[$i]}"
  rm -rf "$dir"
  mv "$pulled/${OBS_NAMES[$i]}" "$dir"

  # The name in the catalog. The version is a placeholder, because the action stamps
  # the final one, and the chart it installs first is the one with the catalog name.
  sed -i \
    -e "s|^name: ${OBS_NAMES[$i]}\$|name: ${NAMES[$i]}|" \
    -e "s|^version:.*|version: \"0\"|" \
    -e "s|catalog.cattle.io/auto-install: ${OBS_NAMES[1]}=match|catalog.cattle.io/auto-install: ${NAMES[1]}=match|" \
    -e "s|catalog.cattle.io/rancher-version: .*$|catalog.cattle.io/rancher-version: '>= 2.${R_VER}.0-0 < 2.${R_VER_NEXT}.0-0'|" \
    -e "s|catalog.cattle.io/kube-version: .*$|catalog.cattle.io/kube-version: '${KUBE_VERSION}'|" \
    "$dir/Chart.yaml"

  # The catalog needs the Kubernetes versions on the main chart only, not on the CRD chart. A main
  # chart without the annotation would be left without it, and the catalog check would only say
  # that later: say it here.
  if [ "$i" -eq 0 ] && ! grep -q "catalog.cattle.io/kube-version: '${KUBE_VERSION}'" "$dir/Chart.yaml"; then
    echo "Error: ${OBS_CHARTS[$i]}:${APP_VERSION} has no catalog.cattle.io/kube-version annotation to set" >&2
    exit 1
  fi
done
