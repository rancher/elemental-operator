#!/bin/bash

get_docker_image_version() {
  local store_var="$1"
  local image_name="$2"

  [ -z "$image_name" -o -z "$store_var" ] && return 1

  outarray=($(docker images | grep $image_name))
  [ $? -ne 0 ] && return 1

  eval $store_var=\${outarray[1]}
  return 0
}

# get_json_val "VARNAME" "JSONDATA" "JSONFIELD"
# receives a json data snippet as JSONDATA and the json field to retrieve as JSONFIELD.
# puts the value extracted in a variable called as VARNAME.
get_json_val() {
    local local_var="$1"
    local jsondata="$2"
    local local_val="$3"

    eval $local_var=$(echo $jsondata | jq $local_val)
}

check_channel_json() {
  local channel_file="$1"

  for i in $(seq 0 20); do
    local item item_type item_image item_name item_url_field
    item=$(jq .[$i] $channel_file)
    [ "$item" = "null" ] && break

    get_json_val item_name "$item" ".metadata.name"
    get_json_val item_type "$item" ".spec.type"
    get_json_val item_display "$item" ".spec.metadata.displayName"

    case $item_type in
      container)
      item_url_field=".spec.metadata.upgradeImage"
      ;;
      iso)
      item_url_field=".spec.metadata.uri"
      ;;
      *)
      echo "ERR: unexpected entry type in channel: '$item_type'"
      return 1
      ;;
    esac
    get_json_val item_image "$item" "$item_url_field"

    echo " found image '$item_image'"
    orig_image=${item_image#$LOCAL_REGISTRY/}
    if [ "$orig_image" = "$item_image" ]; then
      echo "ERR: OS images in the created channel image are not prepended with the private registry URL"
      return 1 
    fi

    if ! docker manifest inspect $orig_image > /dev/null; then
      echo "ERR: source OS image '$orig_image' cannot be found"
      return 1
    fi
  done
  
  return 0
SKIP_ARCHIVE_CREATION
}


LOCAL_REGISTRY="private.reg:5000"
AIRGAPSCRIPT_PATH="scripts/elemental-airgap.sh"

echo "AIRGAP ELEMENTAL SCRIPT TESTING"
for ELEMENTAL_RELEASE in stable staging dev; do

  echo "testing ELEMENTAL RELEASE '$ELEMENTAL_RELEASE'"

  uuid=$(cat /proc/sys/kernel/random/uuid)
  CHANNEL_IMAGE_NAME="rancher/elemental-channel-$uuid"
  export CHANNEL_IMAGE_NAME


  cmd="${AIRGAPSCRIPT_PATH} -d -sa -r ${LOCAL_REGISTRY} ${ELEMENTAL_RELEASE}"
  echo "RUN: $cmd"
  if ! $cmd; then
    echo "['$ELEMENTAL_RELEASE'] FAILED: airgap script returned error"
    exit 1
  fi
  echo "['$ELEMENTAL_RELEASE'] PASSED: archive creation"

  version=""
  if ! get_docker_image_version version "$CHANNEL_IMAGE_NAME"; then
    echo "['$ELEMENTAL_RELEASE'] FAILED: cannot retrieve local $CHANNEL_IMAGE_NAME"
    exit 1
  fi

  if ! docker run --entrypoint busybox ${CHANNEL_IMAGE_NAME}:$version cat channel.json > "${uuid}.json"; then
    echo "['$ELEMENTAL_RELEASE'] FAILED: cannot extract the list of OS URLs from the local channel image: ${CHANNEL_IMAGE_NAME}:$version"
    exit 1
  fi

  if ! check_channel_json "${uuid}.json"; then
    echo "['$ELEMENTAL_RELEASE'] FAILED: generated OS channel inspection"
    exit 1
  fi
  echo "['$ELEMENTAL_RELEASE'] PASSED: inspected generated OS channel"

  rm ${uuid}.json
done

echo "ALL TESTS PASSED!"
exit 0
