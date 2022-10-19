#!/bin/bash

KUBE_VERSION=${KUBE_VERSION:-v1.24.6}
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
# Label the nodes with node-role.kubernetes.io/master as it appears that
# label is no longer added on >=1.24.X clusters while it was set on <=1.23.X
# https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/kubeadm/2067-rename-master-label-taint
# https://kubernetes.io/blog/2022/04/07/upcoming-changes-in-kubernetes-1-24/#api-removals-deprecations-and-other-changes-for-kubernetes-1-24
# system-upgrade-controller 0.9.1 still uses it to schedule pods
kubectl label nodes --all node-role.kubernetes.io/master=
kubectl get nodes -o wide
