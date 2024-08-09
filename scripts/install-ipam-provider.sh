#!/bin/bash

echo "Installing In-cluster IPAM provider" 
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/cluster-api/main/config/crd/bases/ipam.cluster.x-k8s.io_ipaddressclaims.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/cluster-api/main/config/crd/bases/ipam.cluster.x-k8s.io_ipaddresses.yaml
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster/releases/download/v0.1.0/ipam-components.yaml 
