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
# Usage: prepare-charts.sh <tag> <output dir>
#   e.g. prepare-charts.sh v1.9.3 "$RUNNER_TEMP/charts"
#
# The tag is a release, vX.Y.Z: it is the version of the charts that OBS published.
# Environment:
#   OBS_REGISTRY  where OBS published the charts (default registry.suse.com)
#
# It changes the names, the chart that is installed first and the version
# placeholder, and nothing else: the annotations of the chart are the ones OBS
# published, and they have to be right there. In particular catalog.cattle.io/
# rancher-version and catalog.cattle.io/kube-version must give the Rancher version
# the release is for and the Kubernetes versions it supports, both bounds, as in
# ">= 2.16.0-0 < 2.17.0-0": the chart major of the catalog is worked out from the
# first one, and the checks of cargueiro refuse a chart without them.
#
# Requires helm on PATH.
set -euo pipefail

TAG=${1:?usage: $0 <tag> <output dir>}
OUT_DIR=${2:?usage: $0 <tag> <output dir>}
OBS_REGISTRY=${OBS_REGISTRY:-registry.suse.com}

if ! [[ "$TAG" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "Error: tag $TAG must look like vX.Y.Z: only releases are staged" >&2
  exit 1
fi
APP_VERSION="${BASH_REMATCH[1]}"

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
    "$dir/Chart.yaml"
done
