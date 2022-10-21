/*
Copyright © 2022 SUSE LLC

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

	Spec   ManagedOSImageSpec   `json:"spec"`
	Status ManagedOSImageStatus `json:"status"`
}

type ManagedOSImageSpec struct {
	OSImage      string                `json:"osImage,omitempty"`
	CloudConfig  *fleet.GenericMap     `json:"cloudConfig,omitempty"`
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	Concurrency  *int64                `json:"concurrency,omitempty"`

	Prepare              *upgradev1.ContainerSpec `json:"prepare,omitempty"`
	Cordon               *bool                    `json:"cordon,omitempty"`
	Drain                *upgradev1.DrainSpec     `json:"drain,omitempty"`
	UpgradeContainer     *upgradev1.ContainerSpec `json:"upgradeContainer,omitempty"`
	ManagedOSVersionName string                   `json:"managedOSVersionName,omitempty"`

	ClusterRolloutStrategy *fleet.RolloutStrategy `json:"clusterRolloutStrategy,omitempty"`
	Targets                []BundleTarget         `json:"clusterTargets,omitempty"`
}

type BundleTarget struct {
	fleet.BundleDeploymentOptions `json:""`             //nolint
	Name                          string                `json:"name,omitempty"`
	ClusterName                   string                `json:"clusterName,omitempty"`
	ClusterSelector               *metav1.LabelSelector `json:"clusterSelector,omitempty"`
	ClusterGroup                  string                `json:"clusterGroup,omitempty"`
	ClusterGroupSelector          *metav1.LabelSelector `json:"clusterGroupSelector,omitempty"`
}

type ManagedOSImageStatus struct {
	fleet.BundleStatus `json:""` //nolint
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
