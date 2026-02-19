/*
Copyright 2025 The Ali Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AliClusterConfig is the Schema for the aliclusterconfigs API.
type AliClusterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AliClusterConfigSpec   `json:"spec,omitempty"`
	Status AliClusterConfigStatus `json:"status,omitempty"`
}

// AliClusterConfigSpec defines the desired state of AliClusterConfig.
type AliClusterConfigSpec struct {
	// ClusterName allows you to specify the name of the ACK cluster in Alibaba Cloud.
	ClusterName string `json:"clusterName,omitempty"`
	// ClusterID is the id of the created ACK cluster which is used to add nodepools to the cluster.
	ClusterID string `json:"clusterId,omitempty" norman:"noupdate"`
	// AlibabaCredentialSecret is the name of the secret containing the Alibaba credentials.
	AlibabaCredentialSecret string `json:"alibabaCredentialSecret,omitempty"`
	// ClusterType is the type of Alibaba cluster
	ClusterType string `json:"clusterType,omitempty" norman:"noupdate"`
	// ClusterSpec is spec for the cluster. Relevant in case ClusterType is ManagedKubernetes
	ClusterSpec string `json:"clusterSpec,omitempty" norman:"noupdate"`
	// KubernetesVersion defines the desired Kubernetes version.
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`
	// RegionID is the id of the region in which the cluster is deployed.
	RegionID string `json:"regionId,omitempty" norman:"noupdate"`
	// VpcID is the id of the vpc in which the cluster is deployed.
	VpcID string `json:"vpcId,omitempty" norman:"noupdate"`
	// ContainerCIDR is the pod CIDR block.
	// The pod CIDR block cannot overlap with the CIDR block of the VPC
	ContainerCIDR string `json:"containerCidr,omitempty" norman:"noupdate"`
	// ServiceCIDR is the Service CIDR block for the cluster.
	// It cannot overlap with the VPC CIDR block or the CIDR blocks of existing clusters in the VPC.
	ServiceCIDR string `json:"serviceCidr,omitempty" norman:"noupdate"`
	// NodeCIDRMask is the maximum number of IP addresses that can be assigned to each node.
	// This number is determined by the subnet mask of the specified CIDR block
	// only applicable for flannel network plugin
	NodeCIDRMask int `json:"nodeCidrMask,omitempty" norman:"noupdate"`
	// VSwitchIDs is the vSwitches for nodes in the cluster.
	VSwitchIDs []string `json:"vswitchIds,omitempty" norman:"noupdate"`
	// PodVswitchIDs is the vSwitches for pods.
	// For Terway network plug-in, you must allocate vSwitches to pods.
	PodVswitchIDs []string `json:"podVswitchIds,omitempty" norman:"noupdate"`
	// SNATEntry specifies whether to configure SNAT rules for the VPC in which your cluster is deployed.
	// if false nodes and applications in the cluster cannot access the Internet.
	SNATEntry *bool `json:"snatEntry,omitempty"`
	// ProxyMode the kube-proxy mode
	ProxyMode string `json:"proxyMode,omitempty"`
	// EndpointPublicAccess specifies whether to enable Internet access for the cluster.
	EndpointPublicAccess *bool `json:"endpointPublicAccess,omitempty"`
	// SecurityGroupID specifies security group for cluster nodes.
	SecurityGroupID string `json:"securityGroupId,omitempty" norman:"noupdate"`
	// ResourceGroupID the Id of the resource group to which the cluster belongs.
	ResourceGroupID string `json:"resourceGroupId,omitempty" norman:"noupdate"`
	// ZoneIDs are the ids of the zones in which vpc and vswitches will be auto created if those fields are not set.
	ZoneIDs []string `json:"zoneIds,omitempty" norman:"noupdate"`
	// IsEnterpriseSecurityGroup is used to specify if enterprise security group needs to be created or basic.
	IsEnterpriseSecurityGroup *bool `json:"isEnterpriseSecurityGroup,omitempty" norman:"noupdate"`
	// Importer indicates that the cluster was imported.
	// +optional
	// +kubebuilder:default=false
	Imported bool `json:"imported" norman:"noupdate"`
	// Nodepools is a list of node pools associated with the ACK cluster.
	NodePools []AliNodePool `json:"nodePools,omitempty"`
	Addons    []AliAddon    `json:"addons,omitempty" norman:"noupdate"`
}

type AliNodePool struct {
	// nodepool info
	// NodepoolID is the id of the node pool created. received from Alibaba Cloud.
	NodePoolID string `json:"nodePoolId,omitempty"`
	// Name is the name of the node pool
	Name string `json:"name,omitempty"`

	// auto_scaling
	// DesiredSize is the desired number of nodes in case of auto scaling disabled.
	DesiredSize *int64 `json:"desiredSize,omitempty"`
	// ScalingType is nodepool auto scaling type.
	ScalingType string `json:"scalingType,omitempty"`
	// EnableAutoScaling specified if auto scaling is enabled for a node pool.
	EnableAutoScaling *bool `json:"enableAutoScaling,omitempty"`
	// MinInstances for auto scaling of node pool.
	MinInstances *int64 `json:"minInstances,omitempty"`
	// MaxInstances for auto scaling of node pool.
	MaxInstances *int64 `json:"maxInstances,omitempty"`
	// scaling_group
	// AutoRenew specifies whether to enable auto-renewal for nodes in the node pool.
	// This parameter takes effect only when you set InstanceChargeType to PrePaid
	AutoRenew bool `json:"autoRenew,omitempty" norman:"noupdate"`
	// AutoRenewPeriod is the auto-renewal period in Month.
	AutoRenewPeriod int64 `json:"autoRenewPeriod,omitempty" norman:"noupdate"`
	// DataDisks is the configurations of the data disks that are attached to nodes in the node pool.
	DataDisks []AliDisk `json:"dataDisks,omitempty"`
	// InstanceChargeType is the billing method of nodes in the node pool. Valid values: PrePaid, PostPaid
	InstanceChargeType string `json:"instanceChargeType,omitempty" norman:"noupdate"`
	// InstanceTypes the instance types of nodes in the node pool.
	// When the system adds a node to the node pool the system selects the most appropriate one from the specified instance types
	// for the node.
	InstanceTypes []string `json:"instanceTypes,omitempty"`
	// KeyPair is the name of the key pair used to log in to nodes in the nodepool.
	KeyPair string `json:"keyPair,omitempty"`
	// Period is the subscription duration of nodes in the node pool.
	// This parameter takes effect and is required if InstanceChargeType is set to PrePaid.
	Period int64 `json:"period,omitempty" norman:"noupdate"`
	// PeriodUnit the billing cycle of nodes in the node pool.
	PeriodUnit string `json:"periodUnit,omitempty" norman:"noupdate"`
	// ImageID is the id of the image for nodes.
	ImageID string `json:"imageId,omitempty"`
	// ImageType is the type of the OS image
	ImageType string `json:"imageType,omitempty"`
	// VSwitchIDs are the VSwitchIds for the nodes.
	VSwitchIDs []string `json:"vswitchIds,omitempty"`
	// SystemDiskCategory is the category of the system disk.
	SystemDiskCategory string `json:"systemDiskCategory,omitempty"`
	// SystemDiskSize is the size of the system disk
	SystemDiskSize int64 `json:"systemDiskSize,omitempty"`

	// Runtime is the name of the container runtime.
	Runtime string `json:"runtime,omitempty"`
	// RuntimeVersion is the version of the container runtime.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
}

type AliDisk struct {
	// Category is the category of data disk
	Category string `json:"category"`
	// Size is the size of data disks. Unit: GiB
	Size int64 `json:"size"`
	// Encrypted specifies whether to encrypt the data disk. Valid values:
	Encrypted string `json:"encrypted"`
	// AutoSnapshotPolicyID is the Id of the automatic snapshot policy
	AutoSnapshotPolicyID string `json:"autoSnapshotPolicyId"`
}

type AliAddon struct {
	// Name is the Addon Component name
	Name string `json:"name" norman:"noupdate"`
	// Config is the configuration of the component. usually a JSON string.
	Config string `json:"config" norman:"noupdate"`
}

// AliClusterConfigStatus defines the observed state of AliClusterConfig.
type AliClusterConfigStatus struct {
	Phase          string `json:"phase"`
	FailureMessage string `json:"failureMessage"`
	// UpgradeTaskID is the id of the latest upgrade task performed on the cluster.
	// it is set to empty
	UpgradeTaskID string `json:"upgradeTaskId"`
}
