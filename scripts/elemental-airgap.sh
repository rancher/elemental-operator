#!/bin/bash

set -e
set -o pipefail

: ${EOC_REGISTRY:="oci://registry.opensuse.org/isv/rancher/elemental/dev/charts/rancher"}
: ${EOC_OPERATOR_CHART:=$EOC_REGISTRY/elemental-operator-chart}
: ${EOC_CRDS_CHART:=$EOC_REGISTRY/elemental-operator-crds-chart}
: ${CONTAINER_IMAGES_NAME:="elemental-image"}
: ${CONTAINER_IMAGES_FILE:=$CONTAINER_IMAGES_NAME".txt"}
: ${CONTAINER_IMAGES_ARCHIVE:=$CONTAINER_IMAGES_NAME".tar.gz"}
: ${DEBUG:="false"}
: ${LOCAL_REGISTRY:=\$LOCAL_REGISTRY}

# global vars
CHART_NAME_OPERATOR=""
CHART_NAME_CRDS=""

print_help() {
    cat <<- EOF
Usage: $0 [--chart-repo $EOC_REGISTRY]
    [-c|--chart-repo path] source chart repo to pull elemental charts from (without the chart names).
    [-l|--image-list path] generated text file with the list of saved images (one image per line).
    [-i|--images path] tar.gz gernerated by docker save.
    [-d|--debug] enable debug output on screen.
    [-h|--help] Usage message.

    Parameters could also be set passing env vars:
    EOC_REGISTRY (-c)               : $EOC_REGISTRY
    EOC_OPERATOR_CHART              : $EOC_OPERATOR_CHART
    EOC_CRDS_CHART                  : $EOC_CRDS_CHART
    CONTAINER_IMAGES_NAME           : $CONTAINER_IMAGES_NAME
    CONTAINER_IMAGES_FILE (-l)      : $CONTAINER_IMAGES_FILE
    CONTAINER_IMAGES_ARCHIVE (-i)   : $CONTAINER_IMAGES_ARCHIVE
    DEBUG (-d)                      : $DEBUG
    LOCAL_REGISTRY                  : $LOCAL_REGISTRY

EOF
}

parse_parameters() {

    local help

    POSITIONAL=()
    while [[ $# -gt 0 ]]; do
        key="$1"
        case $key in
            -i|--images)
            CONTAINER_IMAGES_ARCHIVE="$2"
            shift # past argument
            shift # past value
            ;;
            -l|--image-list)
            CONTAINER_IMAGES_FILE="$2"
            shift # past argument
            shift # past value
            ;;
            -s|--source-registry)
            EOC_REGISTRY="$2"
            shift # past argument
            shift # past value
            ;;
            -d|--debug)
            DEBUG="true"
            shift
            ;;
            -h|--help)
            help="true"
            shift
            ;;
            *)
            print_help
            exit 1
            ;;
        esac
    done

    if [ "$help" = "true" ]; then
        print_help
        exit 0
    fi
}

exit_error() {
    eval msg=\"$1\"
    echo -e "ERR: $msg"
    exit 1   
}

log_debug() {
    [ "$DEBUG" = "false" ] && return
    eval msg=\"$1\"
    echo -e "$msg"
}

log_info() {
    eval msg=\"$1\"
    echo -e "$msg"
}

get_chart_val() {
    var="$1"
    val="$2"

    eval $var=$(helm show values $CHART_NAME_OPERATOR | eval yq eval '.${val}' | sed s/\"//g 2>&1)
    eval log_debug \"extracted $var\\t: \$$var\ \($val\)\"
}

prereq_checks() {
    log_debug "Check required binaries availability"
    for cmd in helm yq sed docker; do
        if ! command -v "$cmd" > /dev/null; then
            exit_error "'$cmd' not found."
        fi
        log_debug "$cmd found"
    done
}

fetch_charts() {
    local outstr chart

    log_debug "fetching chart '$EOC_OPERATOR_CHART'"
    if ! outstr=$(helm pull ${EOC_OPERATOR_CHART} 2>&1); then
        exit_error "downloading ${EOC_OPERATOR_CHART}:\n $outstr"
    fi
    CHART_NAME_OPERATOR=$(ls -t1 | head -n 1)
    log_info "Downloaded Elemental Operator chart: $CHART_NAME_OPERATOR"

    log_debug "fetching chart '$EOC_CRDS_CHART'"
    if ! outstr=$(helm pull ${EOC_CRDS_CHART} 2>&1); then
        exit_error "downloading ${EOC_CRDS_CHART}:\n $outstr"
    fi
    CHART_NAME_CRDS=$(ls -t1 | head -n 1)
    log_info "Downloaded Elemental Operator CRDS chart: $CHART_NAME_CRDS"
}

write_elemental_images_file() {
    local oprtimg_repo oprtimg_tag seedimg_repo seedimg_tag

    get_chart_val oprtimg_repo "image.repository"
    get_chart_val oprtimg_tag "image.tag"
    get_chart_val seedimg_repo "seedImage.repository"
    get_chart_val seedimg_tag "seedImage.tag"

    log_info "Creating $CONTAINER_IMAGES_FILE"
    cat <<- EOF >> $CONTAINER_IMAGES_FILE
${oprtimg_repo}:${oprtimg_tag}
${seedimg_repo}:${seedimg_tag}
EOF

    sort -u $CONTAINER_IMAGES_FILE -o $CONTAINER_IMAGES_FILE
}

pull_images() {
    local registry_url
    get_chart_val source_registry "registryUrl"

    pulled=""

    while IFS= read -r i; do
        [ -z "${i}" ] && continue
        i="${source_registry}/${i}"
        if docker pull "${i}" > /dev/null 2>&1; then
            log_info "Image pull success: ${i}"
            pulled="${pulled} ${i}"
        else
            if docker inspect "${i}" > /dev/null 2>&1; then
                pulled="${pulled} ${i}"
            else
                log_info "Image pull failed: ${i}"
            fi
        fi
    done < "${CONTAINER_IMAGES_FILE}"

    log_info "Creating ${CONTAINER_IMAGES_ARCHIVE} with $(echo ${pulled} | wc -w | tr -d '[:space:]') images"
    docker save $(echo ${pulled}) | gzip --stdout > ${CONTAINER_IMAGES_ARCHIVE}
}

print_next_steps() {
    local registry_url

    get_chart_val registry_url "registryUrl"

    cat <<- EOF


NEXT STEPS:

1) Load the '$CONTAINER_IMAGES_ARCHIVE' to the local registry ($LOCAL_REGISTRY)
   available in the airgapped infrastructure:

./rancher-load-images.sh \\
   --image-list $CONTAINER_IMAGES_FILE \\
   --images $CONTAINER_IMAGES_ARCHIVE \\
   --registry $LOCAL_REGISTRY

2) Install the elemental charts downloaded in the current directory passing the local registry:

helm upgrade --create-namespace -n cattle-elemental-system --install elemental-operator-crds $CHART_NAME_CRDS

helm upgrade --create-namespace -n cattle-elemental-system --install elemental-operator $CHART_NAME_OPERATOR \\
  --set registryUrl=$LOCAL_REGISTRY \\
  --set channel.repository=""

EOF
}

parse_parameters "$@"

prereq_checks

fetch_charts

write_elemental_images_file

pull_images

print_next_steps
