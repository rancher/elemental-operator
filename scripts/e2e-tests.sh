#!/bin/bash

KUBE_VERSION=${KUBE_VERSION:-v1.22.7}
CLUSTER_NAME="${CLUSTER_NAME:-operator-e2e}"

if ! kind get clusters | grep "$CLUSTER_NAME"; then
cat << EOF > kind.config
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    image: kindest/node:$KUBE_VERSION
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
EOF
    kind create cluster --name $CLUSTER_NAME --config kind.config
    rm -rf kind.config
fi

set -e

kubectl cluster-info --context kind-$CLUSTER_NAME
echo "Sleep to give times to node to populate with all info"
kubectl wait --for=condition=Ready node/operator-e2e-control-plane
export EXTERNAL_IP=$(kubectl get nodes -o jsonpath='{.items[].status.addresses[?(@.type == "InternalIP")].address}')
export BRIDGE_IP="172.18.0.1"
kubectl get nodes -o wide
cd $ROOT_DIR/tests &&  ginkgo -r -v ./e2e
