#!/bin/bash 

function checkimages {
  local images=$1
  local img

  for img in ${images}; do
    echo -n "Looking for image ${img} ..."
    docker manifest inspect "${img}" >/dev/null || return 1
    echo "OK"
  done
}

function findChannels {
  local images=$1
  local img

  for img in ${images}; do
    if echo "${img}" | grep -q channel ;then
      echo "${img}"
    fi
  done
}

function checkChannel {
  local channel=$1
  local os_images
  local iso_images

  echo "Checking channel contents for: ${channel}"

  os_images=$(docker run --rm --pull=always --entrypoint busybox "${channel}" cat /channel.json | jq -r -c '.[] | select(.spec.type | contains("container")).spec.metadata.upgradeImage')
  checkimages "${os_images}"

  iso_images=$(docker run --rm --pull=always --entrypoint busybox "${channel}" cat /channel.json | jq -r -c '.[] | select(.spec.type | contains("iso")).spec.metadata.uri')
  checkimages "${iso_images}"

  echo "Deleting channel ${channel} from local storage"
  docker rmi "${channel}" >/dev/null
}

declare OPERATOR_IMAGES
declare channels
declare channel

set -e

if [ "$1" == "" ]; then
  OPERATOR_IMAGES=$(</dev/stdin)
else
  OPERATOR_IMAGES=$(cat "${1}")
fi

checkimages "${OPERATOR_IMAGES}"

channels=$(findChannels "${OPERATOR_IMAGES}")

for channel in ${channels}; do
  checkChannel ${channel}
done
