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

type ManagedOSVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedOSVersionSpec   `json:"spec"`
	Status ManagedOSVersionStatus `json:"status"`
}

type ManagedOSVersionSpec struct {
	Version          string                   `json:"version,omitempty"`
	Type             string                   `json:"type,omitempty"`
	MinVersion       string                   `json:"minVersion,omitempty"`
	Metadata         *fleet.GenericMap        `json:"metadata,omitempty"`
	UpgradeContainer *upgradev1.ContainerSpec `json:"upgradeContainer,omitempty"`
}

type ManagedOSVersionStatus struct {
	fleet.BundleStatus `json:""` //nolint
}

// +kubebuilder:object:root=true

// ManagedOSVersionList contains a list of ManagedOSVersions.
type ManagedOSVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedOSVersion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedOSVersion{}, &ManagedOSVersionList{})
}
