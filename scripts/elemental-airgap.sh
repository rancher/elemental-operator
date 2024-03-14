#!/bin/bash

set -e
set -o pipefail
set -o nounset

# global vars
CHART_NAME_OPERATOR=""
ELEMENTAL_OPERATOR_CRDS_CHART_NAME="\$ELEMENTAL_CRDS_CHART"
CHANNEL_IMAGE_VAR=""
CHANNEL_IMAGE_VAL=""
IMAGES_TO_SAVE=""
HAULER_REG_DIR=$(mktemp -d)
HAULER_REG_PID=""
HAULER_REG_DUMMY_FILE="hauler-registry.txt"

: ${CONTAINER_IMAGES_NAME:="elemental-images"}
: ${CONTAINER_IMAGES_FILE:=$CONTAINER_IMAGES_NAME".txt"}
: ${CONTAINER_IMAGES_ARCHIVE:=$CONTAINER_IMAGES_NAME".tar.gz"}
: ${DEBUG:="false"}
: ${HAULER:="false"}
: ${HAULER_STORE:=$(pwd)"/elemental-collection"}
: ${LOCAL_REGISTRY:=\$LOCAL_REGISTRY}
: ${CHART_NAME_CRDS:=$ELEMENTAL_OPERATOR_CRDS_CHART_NAME}
: ${CHART_VERSION:="latest"}
: ${CHANNEL_ONLY:="false"}
: ${CHANNEL_IMAGE_NAME:="\$CHANNEL_IMAGE_NAME"}
: ${SKIP_ARCHIVE_CREATION:="false"}

print_help() {
    cat <<- EOF
Usage: $0  [OPTION]  -r LOCAL_REGISTRY  ELEMENTAL_OPERATOR_CHART
    [-c|--crds-chart] Elemental CRDS chart (if URL, will be downloaded).
    [-co|--channel-only] just extract and rebuild the ManagedOSVersionChannel container image
    [-cv|--chart-version] Specify the chart version (only used if passing chart as URLs).
    [-d|--debug] enable debug output on screen.
    [-i|--images path] tar.gz gernerated by docker save.
    [-h|--help] Usage message.
    [-ha|--hauler] Use hauler to generate the archive (an "Haul") of the Elemental "Collection".
    [-l|--image-list path] generated text file with the list of saved images (one image per line).
    [-r|--local-registry] registry where to load the images to (used in the next steps).
    [-sa|--skip-archive] put the list of images in the $CONTAINER_IMAGES_FILE but skip $CONTAINER_IMAGES_ARCHIVE creation

    ELEMENTAL_OPERATOR_CHART could be either a chart tgz file or an url (in that case will be downloaded first)
    it could even be 'dev', 'staging' or 'stable' to allow automatic download of the charts.

    Parameters could also be set passing env vars:
    CONTAINER_IMAGES_NAME           : $CONTAINER_IMAGES_NAME
    CONTAINER_IMAGES_FILE (-l)      : $CONTAINER_IMAGES_FILE
    CONTAINER_IMAGES_ARCHIVE (-i)   : $CONTAINER_IMAGES_ARCHIVE
    DEBUG (-d)                      : $DEBUG
    LOCAL_REGISTRY (-r)             : $LOCAL_REGISTRY
    CHART_NAME_CRDS (-c)            : $CHART_NAME_CRDS
    CHART_VERSION (-cv)             : $CHART_VERSION
    CHANNEL_ONLY (-co)              : $CHANNEL_ONLY
    CHANNEL_IMAGE_NAME              : $CHANNEL_IMAGE_NAME
    SKIP_ARCHIVE_CREATION (-sa)     : $SKIP_ARCHIVE_CREATION

    examples:

    $0 -r $LOCAL_REGISTRY staging

    $0 oci://registry.opensuse.org/isv/rancher/elemental/staging/charts/rancher/elemental-operator-chart \\
      -c oci://registry.opensuse.org/isv/rancher/elemental/staging/charts/rancher/elemental-operator-crds-chart \\
      -r $LOCAL_REGISTRY

    # For creating an Hauler haul
    $0 -ha -r $LOCAL_REGISTRY stable
EOF
}

parse_parameters() {

    local help="false"
    local container_images_archive="false"

    POSITIONAL=()
    while [[ $# -gt 0 ]]; do
        key="$1"
        case $key in
            -i|--images)
            CONTAINER_IMAGES_ARCHIVE="$2"
            container_images_archive="true"
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
            -co|--channel-only)
            CHANNEL_ONLY=true
            shift
            ;;
            -sa|--skip-archive)
            SKIP_ARCHIVE_CREATION=true
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
            -ha|--hauler)
            HAULER="true"
            if [[ "${container_images_archive}" == "false" ]]; then
                CONTAINER_IMAGES_ARCHIVE="elemental-haul.tar.zst"
            fi
            shift
            ;;
            *)
            [[ -n "$CHART_NAME_OPERATOR" ]] && exit_error "unrecognized command: $1"
            CHART_NAME_OPERATOR="$1"
            shift
            ;;
        esac
    done
    if [[ "$help" = "true" ]]; then
        print_help
        exit 0
    fi
    if [[ -z "$CHART_NAME_OPERATOR" ]]; then
        print_help
        echo ""
        exit_error "ELEMENTAL_OPERATOR_CHART is a required argument"
    fi
    if [[ "$LOCAL_REGISTRY" == "\$LOCAL_REGISTRY" ]]; then
        print_help
        echo ""
        exit_error "LOCAL_REGISTRY is required"
    fi
    case "$CHART_NAME_OPERATOR" in
        Dev|dev|DEV)
        CHART_NAME_OPERATOR="oci://registry.opensuse.org/isv/rancher/elemental/dev/charts/rancher/elemental-operator-chart"
        [[ "$CHART_NAME_CRDS" == "$ELEMENTAL_OPERATOR_CRDS_CHART_NAME" ]] && \
            CHART_NAME_CRDS="oci://registry.opensuse.org/isv/rancher/elemental/dev/charts/rancher/elemental-operator-crds-chart"
        ;;
        Staging|staging|STAGING)
        CHART_NAME_OPERATOR="oci://registry.opensuse.org/isv/rancher/elemental/staging/charts/rancher/elemental-operator-chart"
        [[ "$CHART_NAME_CRDS" == "$ELEMENTAL_OPERATOR_CRDS_CHART_NAME" ]] && \
            CHART_NAME_CRDS="oci://registry.opensuse.org/isv/rancher/elemental/staging/charts/rancher/elemental-operator-crds-chart"
        ;;
        Stable|stable|STABLE)
        CHART_NAME_OPERATOR="oci://registry.suse.com/rancher/elemental-operator-chart"
        [[ "$CHART_NAME_CRDS" == "$ELEMENTAL_OPERATOR_CRDS_CHART_NAME" ]] && \
            CHART_NAME_CRDS="oci://registry.suse.com/rancher/elemental-operator-crds-chart"
        ;;
    esac

}

exit_error() {
    eval msg=\"$1\"
    echo -e "ERR: $msg"
    is_hauler && hauler_stop_local_registry
    exit 1   
}

log_debug() {
    [[ "$DEBUG" == "false" ]] && return 0
    eval msg=\'${1}\'
    echo -e "$msg"
}

log_info() {
    eval msg=\"$1\"
    echo -e "$msg"
}

is_hauler() {
    [[ "${HAULER}" != "false" ]] && return 0 || return 1
}

# get_chart_val "VARNAME" "CHARTVAR" ["false"]
# Retrieves CHARTVAR from the chart and puts the value in VARNAME.
# If CHARTVAR is not found in the chart, it will error and exit, BUT if the optional third parameter
# is passed and is == "false" would just log the missing value and not terminate the script.
get_chart_val() {
    local local_var="$1"
    local local_val="$2"
    local local_fail=${3:-"true"}
    local local_condition="[[ \"\$$local_var\" == \"null\" ]]"

    eval $local_var=$(helm show values $CHART_NAME_OPERATOR | eval yq eval '.${local_val}' | sed s/\"//g 2>&1)
    if eval $local_condition; then
        if [[ "$local_fail" == "false" ]]; then
            log_debug "cannot find $local_val in $CHART_NAME_OPERATOR"
            eval $local_var=""
            return 0
        fi
        exit_error "cannot find \$local_val in $CHART_NAME_OPERATOR (likely not an elemental-operator chart with airgap support)"
    fi
    eval log_debug \"extracted $local_var\\t: \$$local_var\ \($local_val\)\"
}

# get_json_val "VARNAME" "JSONDATA" "JSONFIELD"
# receives a json data snippet as JSONDATA and the json field to retrieve as JSONFIELD.
# puts the value extracted in a variable called as VARNAME.
get_json_val() {
    local local_var="$1"
    local jsondata="$2"
    local local_val="$3"

    eval $local_var=$(echo $jsondata | jq $local_val)
    eval log_debug \"extracted $local_var\\t: \$$local_var\ \($local_val\)\"
}

# set_json_val "VARNAME" "JSONDATA" "JSONFIELD" "VALUE"
# sets JSONFIELD to VALUE in the received JSONDATA json snippet and outputs the changed json data in
# variable VARNAME
set_json_val() {
    local local_var="$1"
    local jsondata="$2"
    local json_field="$3"
    local local_val="$4"

    log_debug "set $json_field to $local_val"
    eval $local_var=\'$(echo $jsondata | jq $json_field=\"$local_val\")\'
    eval log_debug \"\$$local_var\"
}

# add_image_to_export_list
# this just adds the passed image to the list of the images to be saved in the images list text file and
# in the tar.gz archive containing the saved images to be loaded in the local registry.
add_image_to_export_list() {
    is_hauler && return 0
    local img="${1}"
    if [[ -z "${img}" ]]; then
        log_debug "cannot add image to export list: empty image passed"
        return 0
    fi
    case "${IMAGES_TO_SAVE} " in
        *" ${img} "*)
        log_debug "skip adding image to export list: '$img' already added"
        ;;
        *)
        log_debug "mark '$img' image for export"
        IMAGES_TO_SAVE="$IMAGES_TO_SAVE $img"
        ;;
    esac
}

prereq_checks() {
    log_debug "Check required binaries availability"
    local cmd_list="helm yq jq sed docker"
    is_hauler && cmd_list="${cmd_list} hauler"
    for cmd in $cmd_list; do
        if ! command -v "$cmd" > /dev/null; then
            exit_error "'$cmd' not found."
        fi
        log_debug "$cmd found"
    done
}

fetch_charts() {
    local outstr charts="CHART_NAME_OPERATOR"

    [[ "$CHART_NAME_CRDS" != "$ELEMENTAL_OPERATOR_CRDS_CHART_NAME" ]] && charts="${charts} CHART_NAME_CRDS"

    for c in $charts; do
        local chart=""
        local chart_ver=""

        # helm pull only supports semver tags: for the "latest" tag just don't put the version.
        [[ "$CHART_VERSION" != "latest" ]] && chart_ver="--version $CHART_VERSION"

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
            log_info "Downloaded Elemental Operator chart: \$$c"
            if is_hauler; then
                eval hauler_store_add_file "\$$c"
            fi
            ;;
            *)
            [[ ! -f "$chart" ]] && exit_error "chart file $chart not found"
            log_debug "using chart $chart"
            ;;
        esac
    done
}

pull_chart_container_images() {
    local oprtimg_repo oprtimg_tag seedimg_repo seedimg_tag registry_url

    [[ "$CHANNEL_ONLY" == "true" ]] && return 0

    get_chart_val oprtimg_repo "image.repository"
    get_chart_val oprtimg_tag "image.tag"
    get_chart_val seedimg_repo "seedImage.repository"
    get_chart_val seedimg_tag "seedImage.tag"
    get_chart_val source_registry "registryUrl"

    for img in "${oprtimg_repo}:${oprtimg_tag}" "${seedimg_repo}:${seedimg_tag}"; do
        [[ -z "$img" ]] && continue

        if is_hauler; then
            hauler_store_add_image "${source_registry}/${img}"
            continue
        elif pull_image "${source_registry}/${img}"; then
            docker tag "${source_registry}/${img}" "${img}"
            add_image_to_export_list "${img}"
        fi
    done
}

pull_image() {
    local image_url="$1"

    [[ -z "$image_url" ]] && return 1
    if docker pull "$image_url" > /dev/null 2>&1; then
        log_info "Image pull success: ${image_url}"
    else
        if docker inspect "$image_url" > /dev/null 2>&1; then
            log_debug "error downloading ${image_url} but the image is saved"
        else
            log_info "Image pull failed: ${image_url}"
            return 1
        fi
    fi
    return 0
}


build_os_channel() {
    local channel_img
    local channel_tag
    local channel_repo

    log_info "Creating OS channel"
     # name of the new channel container image we are going to create
    if [[ "${CHANNEL_IMAGE_NAME}" == "\$CHANNEL_IMAGE_NAME" ]]; then
        CHANNEL_IMAGE_NAME="rancher/elemental-channel-${LOCAL_REGISTRY%:*}"
    fi

    # channel.repository was changed to channel.image around 1.4 - 1.5 versions
    # channel.image contains the full URL (with registry) while channel.repository not (full URL by prepending with registryUrl)
    # example, full image URL: registry.suse.com/rancher/elemental-channel
    # - operator < v1.4
    #   . channel.repository = "rancher/elemental-channel"
    #   . registryUrl        = "registry.suse.com"
    # - operator > v1.4
    #   . channel.image = "registry.suse.com/rancher/elemental-channel"
    #
    # we want in channel_img the full image URL for both cases
    get_chart_val channel_img "channel.image" "false"

    if [[ -z "$channel_img" ]]; then
        # legacy chart
        get_chart_val channel_img "channel.repository"
        get_chart_val channel_repo "registryUrl"
        channel_img=${channel_repo}/${channel_img}
        CHANNEL_IMAGE_VAR="channel.repository"
        CHANNEL_IMAGE_VAL=${CHANNEL_IMAGE_NAME}
    else
        CHANNEL_IMAGE_VAR="channel.image"
        CHANNEL_IMAGE_VAL=${LOCAL_REGISTRY}/${CHANNEL_IMAGE_NAME}
    fi

    get_chart_val channel_tag "channel.tag"

    if [[ -z "$channel_img" || -z "$channel_tag" ]]; then
        log_info "\nWARNING: channel image not found: you will need to provide your own Elemental OS images\n"
        return 0
    fi
    log_info "Found channel image: ${channel_img}:${channel_tag}"

    TEMPDIR=$(mktemp -d)
    log_debug "build channel image in $TEMPDIR"
    pushd $TEMPDIR > /dev/null
    # extract the channel.json
    if ! docker run --entrypoint busybox ${channel_img}:${channel_tag} cat channel.json > channel.json; then
        exit_error "cannot extract OS images"
    fi

    # write the new channel and identify OS images to save
    local new_channel=""
    for i in $(seq 0 20); do
        local item item_type item_image item_name item_url_field
        item=$(jq .[$i] channel.json)
        [[ "$item" == "null" ]] && break

        get_json_val item_name "$item" ".metadata.name"
        get_json_val item_type "$item" ".spec.type"
        get_json_val item_display "$item" ".spec.metadata.displayName"

        # drop the items in the channel which are not containers
        case $item_type in
            container)
            item_url_field=".spec.metadata.upgradeImage"
            get_json_val item_image "$item" "$item_url_field"
            ;;
            iso)
            item_url_field=".spec.metadata.uri"
            get_json_val item_image "$item" "$item_url_field"
            case $item_image in
                "http://"*|"https://"*)
                log_debug "skip OS $item_type entry:\t$item_name\t$item_image"
                continue
                ;;
            esac
            ;;
            *)
            log_info "ERR: unknown image type `$item_type`, skip OS entry '$item_name'"
            continue
            ;;
        esac

        log_info "Extract OS image:\n\t$item_name ($item_display)\n\t$item_image"

        if [[ "$SKIP_ARCHIVE_CREATION" != "true" ]]; then
            # save the OS image
            if is_hauler; then
                if ! hauler_store_add_image "${item_image}" "neverquit"; then
                    continue
                fi
            elif ! pull_image "${item_image}"; then
                continue
            fi
        fi
        add_image_to_export_list "${item_image}"

        # prepend the private registry name
        set_json_val item "$item" "$item_url_field" "${LOCAL_REGISTRY}/${item_image}"

        [[ -z "$new_channel" ]] && new_channel="[${item}" || new_channel="${new_channel},${item}"
    done
    new_channel="${new_channel}]"

    local channel_full_url="${CHANNEL_IMAGE_NAME}:${channel_tag}"
    is_hauler && channel_full_url=localhost:5000/${channel_full_url}
    log_info "Create new channel image for the private registry: ${channel_full_url}"
    jq -n "$new_channel" > channel.json
    cat << EOF > Dockerfile
FROM registry.suse.com/bci/bci-busybox:latest
ADD channel.json /channel.json
USER 10010:10010
ENTRYPOINT ["busybox", "cp"]
CMD ["/channel.json", "/data/output"]
EOF
    docker build . -t ${channel_full_url}
    if is_hauler; then
        hauler_start_local_registry
        docker push ${channel_full_url}
        hauler_store_add_image ${channel_full_url}
        hauler_stop_local_registry
    fi

    popd > /dev/null
    [[ "$DEBUG" == "false" ]] && rm -rf $TEMPDIR

    add_image_to_export_list "${channel_full_url}"
}

hauler_start_local_registry() {
    # hauler needs something in the store to start the registry
    log_debug "start Hauler registry in $HAULER_REG_DIR"
    pushd ${HAULER_REG_DIR} > /dev/null
    touch ${HAULER_REG_DUMMY_FILE}
    hauler store add file ${HAULER_REG_DUMMY_FILE}
    hauler store serve registry > /dev/null 2>&1 &
    if (( $? )); then
        hauler_stop_local_registry
        exit_error "hauler: cannot start registry"
    fi
    HAULER_REGISTRY_PID=$!
    # wait the hauler registry to be up and running
    sleep 1
    popd > /dev/null
}

hauler_stop_local_registry() {
    if [[ -n "{HAULER_REGISTRY_PID}" ]]; then
        kill ${HAULER_REGISTRY_PID} > /dev/null 2>&1
    fi
    rm -rf ${HAULER_REG_DIR}
}

hauler_store_add_file() {
    local file="$1"
    local neverquit=${2:-""}

    if ! hauler store -s ${HAULER_STORE} add file ${file}; then
      # !hauler does not return error (yet): https://github.com/rancherfederal/hauler/issues/185
      if [[ -z "$neverquit" ]]; then
          exit_error "hauler: cannot add file ${file}"
      fi
    fi
}

hauler_store_add_image() {
    local img="$1"
    local neverquit=${2:-""}

    if ! hauler store -s ${HAULER_STORE} add image ${img}; then
      # !hauler does not return error (yet): https://github.com/rancherfederal/hauler/issues/185
      if [[ -z "$neverquit" ]]; then
          exit_error "hauler: cannot add image ${img}"
      fi
    fi
}

create_container_images_archive() {
    if is_hauler; then
        log_info "Creating haul ${CONTAINER_IMAGES_ARCHIVE}"
        hauler store -s ${HAULER_STORE} save -f ${CONTAINER_IMAGES_ARCHIVE}
        return 0
    fi

    echo -n "" > "${CONTAINER_IMAGES_FILE}"
    for i in ${IMAGES_TO_SAVE} ; do
        log_debug "* $i"
        echo "$i" >> "${CONTAINER_IMAGES_FILE}"
    done
    sort -u ${CONTAINER_IMAGES_FILE} -o ${CONTAINER_IMAGES_FILE}

    [[ "$SKIP_ARCHIVE_CREATION" == "true" ]] && return 0

    log_info "Creating ${CONTAINER_IMAGES_ARCHIVE} with $(echo ${IMAGES_TO_SAVE} | wc -w | tr -d '[:space:]') images (may take a while)"
    docker save $(echo ${IMAGES_TO_SAVE}) | gzip --stdout > ${CONTAINER_IMAGES_ARCHIVE}
}

print_next_steps() {
    local registry_url

    [[ "$SKIP_ARCHIVE_CREATION" == "true" ]] && return 0

    get_chart_val registry_url "registryUrl"

    if is_hauler; then
        print_next_steps_hauler
    else
        print_next_steps_docker
    fi
}

print_next_steps_docker() {
    cat <<- EOF


NEXT STEPS:

1) Load the '$CONTAINER_IMAGES_ARCHIVE' to the local registry ($LOCAL_REGISTRY)
   available in the airgapped infrastructure:

./rancher-load-images.sh \\
   --image-list $CONTAINER_IMAGES_FILE \\
   --images $CONTAINER_IMAGES_ARCHIVE \\
   --registry $LOCAL_REGISTRY

2) Install the elemental charts downloaded in the current directory passing the local registry
   and the newly created channel image:

helm upgrade --create-namespace -n cattle-elemental-system --install elemental-operator-crds $CHART_NAME_CRDS

helm upgrade --create-namespace -n cattle-elemental-system --install elemental-operator $CHART_NAME_OPERATOR \\
  --set registryUrl=$LOCAL_REGISTRY \\
  --set $CHANNEL_IMAGE_VAR=$CHANNEL_IMAGE_VAL
EOF
}

print_next_steps_hauler() {
    cat <<- EOF


NEXT STEPS:

1) Load the '$CONTAINER_IMAGES_ARCHIVE' Haul archive in the Hauler instance in the airgapped infrastructure:

hauler store load '$CONTAINER_IMAGES_ARCHIVE'

2) If the local registry is not served by Hauler, copy the content of the Haul archive to the local registry:

hauler store copy registry://$LOCAL_REGISTRY

   for more options and information on Hauler, check the official docs:
   https://rancherfederal.github.io/hauler-docs/

3) Extract the elemental charts from the Hauler store in the airgapped infrastructure:

hauler store extract $CHART_NAME_CRDS

hauler store extract $CHART_NAME_OPERATOR

   and install them passing the local registry and the newly created channel image:

helm upgrade --create-namespace -n cattle-elemental-system --install elemental-operator-crds $CHART_NAME_CRDS

helm upgrade --create-namespace -n cattle-elemental-system --install elemental-operator $CHART_NAME_OPERATOR \\
  --set registryUrl=$LOCAL_REGISTRY \\
  --set $CHANNEL_IMAGE_VAR=$CHANNEL_IMAGE_VAL
EOF
}

clean_up() {
    is_hauler || return 0

    # only hauler stuffs need to be cleaned at the end
    [[ "$DEBUG" == "false" ]] && \
        rm -rf ${HAULER_STORE} ${CHART_NAME_CRDS} ${CHART_NAME_OPERATOR}
}

parse_parameters "$@"

prereq_checks

fetch_charts

pull_chart_container_images

build_os_channel

create_container_images_archive

print_next_steps

clean_up
