# Operator

The Elemental operator extends Rancher introducing OS provisioning and management capabilities.

Machines booting from an Elemental live ISO register to the Elemental Operator, get provisioned
with the OS and a k8s distro, forming a new Kubernetes cluster immediately available in Rancher.

See the [Elemental docs](https://elemental.docs.rancher.com) for more information.

## Installation

The Elemental operator should be installed on a K8s cluster running Rancher Multi Cluster
Management server.

A step by step [guide](https://elemental.docs.rancher.com/quickstart-ui) is available in the [official documentation](https://elemental.docs.rancher.com).
