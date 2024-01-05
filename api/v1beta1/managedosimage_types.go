/*
Copyright © 2022 - 2024 SUSE LLC

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

package v1beta1

import (
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type ManagedOSImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedOSImageSpec   `json:"spec,omitempty"`
	Status ManagedOSImageStatus `json:"status,omitempty"`
}

type ManagedOSImageSpec struct {
	// +optional
	OSImage string `json:"osImage,omitempty"`
	// +optional
	CloudConfig *fleet.GenericMap `json:"cloudConfig,omitempty"`
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	// +optional
	Concurrency *int64 `json:"concurrency,omitempty"`

	// +optional
	Prepare *upgradev1.ContainerSpec `json:"prepare,omitempty"`
	// +optional
	Cordon *bool `json:"cordon,omitempty"`
	// +optional
	Drain *upgradev1.DrainSpec `json:"drain,omitempty"`
	// +optional
	UpgradeContainer *upgradev1.ContainerSpec `json:"upgradeContainer,omitempty"`
	// +optional
	ManagedOSVersionName string `json:"managedOSVersionName,omitempty"`

	// +optional
	ClusterRolloutStrategy *fleet.RolloutStrategy `json:"clusterRolloutStrategy,omitempty"`
	// +optional
	Targets []BundleTarget `json:"clusterTargets,omitempty"`
}

type BundleTarget struct {
	fleet.BundleDeploymentOptions `json:""` //nolint
	// +optional
	Name string `json:"name,omitempty"`
	// +optional
	ClusterName string `json:"clusterName,omitempty"`
	// +optional
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
	// +optional
	ClusterGroup string `json:"clusterGroup,omitempty"`
	// +optional
	ClusterGroupSelector *metav1.LabelSelector `json:"clusterGroupSelector,omitempty"`
}

type ManagedOSImageStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true

// ManagedOSImageList contains a list of ManagedOSImages.
type ManagedOSImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedOSImage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedOSImage{}, &ManagedOSImageList{})
}
