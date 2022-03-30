#!/bin/bash
set -ex

KUBE_VERSION=${KUBE_VERSION:-v1.22.7}

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

kind delete cluster --name "ros-e2e"
kind create cluster --name "ros-e2e" --config kind.config
rm -rf kind.config

kubectl cluster-info --context kind-ros-e2e
echo "Sleep to give times to node to populate with all info"
kubectl wait --for=condition=Ready node/ros-e2e-control-plane
export EXTERNAL_IP=$(kubectl get nodes -o jsonpath='{.items[].status.addresses[?(@.type == "InternalIP")].address}')
kubectl get nodes -o wide
cd $ROOT_DIR/tests &&  ginkgo -r -v ./e2e