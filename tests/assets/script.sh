#!/bin/bash

mkdir -p /etc/rancher/rancherd/
cat > /etc/rancher/rancherd/config.yaml <<-EOF
rancherd:
  role: server
  rancherValues:
    replicas: -3
EOF
curl -fL https://raw.githubusercontent.com/rancher/rancherd/master/install.sh | sh -
curl -fL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | sh -

