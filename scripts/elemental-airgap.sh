#!/bin/bash

set -e
set -o pipefail

# global vars
CHART_NAME_OPERATOR=""
ELEMENTAL_OPERATOR_CRDS_CHART_NAME="\$ELEMENTAL_CRDS_CHART"

: ${CONTAINER_IMAGES_NAME:="elemental-image"}
: ${CONTAINER_IMAGES_FILE:=$CONTAINER_IMAGES_NAME".txt"}
: ${CONTAINER_IMAGES_ARCHIVE:=$CONTAINER_IMAGES_NAME".tar.gz"}
: ${DEBUG:="false"}
: ${LOCAL_REGISTRY:=\$LOCAL_REGISTRY}
: ${CHART_NAME_CRDS:=$ELEMENTAL_OPERATOR_CRDS_CHART_NAME}
: ${CHART_VERSION:="latest"}

print_help() {
    cat <<- EOF
Usage: $0 [OPTION] ELEMENTAL_OPERATOR_CHART
    [-l|--image-list path] generated text file with the list of saved images (one image per line).
    [-i|--images path] tar.gz gernerated by docker save.
    [-c|--crds-chart] Elemental CRDS chart (if URL, will be downloaded).
    [-cv|--chart-version] Specify the chart version (only used if passing chart as urls).
    [-r|--local-registry] registry where to load the images to (used in the next steps).
    [-d|--debug] enable debug output on screen.
    [-h|--help] Usage message.

    Parameters could also be set passing env vars:
    CONTAINER_IMAGES_NAME           : $CONTAINER_IMAGES_NAME
    CONTAINER_IMAGES_FILE (-l)      : $CONTAINER_IMAGES_FILE
    CONTAINER_IMAGES_ARCHIVE (-i)   : $CONTAINER_IMAGES_ARCHIVE
    DEBUG (-d)                      : $DEBUG
    LOCAL_REGISTRY (-r)             : $LOCAL_REGISTRY
    CHART_NAME_CRDS (-c)            : $CHART_NAME_CRDS
    CHART_VERSION (-cv)             : $CHART_VERSION

    example:
    $0 oci://registry.opensuse.org/isv/rancher/elemental/staging/charts/rancher/elemental-operator-chart
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
            -c|--crds-chart)
            CHART_NAME_CRDS="$2"
            shift
            shift
            ;;
            -cv|--chart-version)
            CHART_VERSION="$2"
            shift
            shift
            ;;
            -r|--local-registry)
            LOCAL_REGISTRY="$2"
            shift
            shift
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
            [ -n "$CHART_NAME_OPERATOR" ] && exit_error "unrecognized command: $1"
            CHART_NAME_OPERATOR="$1"
            shift
            ;;
        esac
    done

    if [ "$help" = "true" ]; then
        print_help
        exit 0
    fi
    if [ -z "$CHART_NAME_OPERATOR" ]; then
        print_help
        echo ""
        exit_error "ELEMENTAL_OPERATOR_CHART is a required argument"
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
    local local_var="$1"
    local local_val="$2"
    local local_condition="[ \"\$$local_var\" = \"null\" ]"

    eval $local_var=$(helm show values $CHART_NAME_OPERATOR | eval yq eval '.${local_val}' | sed s/\"//g 2>&1)
    if eval $local_condition ; then
        exit_error "cannot find \$local_val in $CHART_NAME_OPERATOR (is it an elemental-operator chart?)"
    fi
    eval log_debug \"extracted $local_var\\t: \$$local_var\ \($local_val\)\"
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
    local outstr charts="CHART_NAME_OPERATOR"

    [ "$CHART_NAME_CRDS" != "$ELEMENTAL_OPERATOR_CRDS_CHART_NAME" ] && charts="${charts} CHART_NAME_CRDS"

    for c in $charts; do
        local chart chart_ver

        # helm pull only supports smever tags: for the "latest" tag just don't put the version.
        [ "$CHART_VERSION" != "latest" ] && chart_ver="--version $CHART_VERSION"

        # 'c' var holds the name (e.g., CHART_NAME_OPERATOR),
        # 'chart' var holds the value (e.g., elemental-operator-chart-1.4.tgz)
        eval chart=\$$c
        case $chart in
            "oci://"*|"https//"*)
            log_debug "fetching chart '$chart' $chart_ver"
            if ! outstr=$(helm pull ${chart} $chart_ver 2>&1); then
                exit_error "downloading ${chart}:\n $outstr"
            fi
            eval $c=$(ls -t1 | head -n 1)
            log_info "Downloaded Elemental Operator chart: $chart"
            ;;
            *)
            [ ! -f "$chart" ] && exit_error "chart file $chart not found"
            log_debug "using chart $chart"
            ;;
        esac
    done
}

write_elemental_images_file() {
    local oprtimg_repo oprtimg_tag seedimg_repo seedimg_tag

    get_chart_val oprtimg_repo "image.repository"
    get_chart_val oprtimg_tag "image.tag"
    get_chart_val seedimg_repo "seedImage.repository"
    get_chart_val seedimg_tag "seedImage.tag"

    log_info "Creating $CONTAINER_IMAGES_FILE"
    cat <<- EOF > $CONTAINER_IMAGES_FILE
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
