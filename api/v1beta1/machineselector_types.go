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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type MachineInventorySelector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MachineInventorySelectorSpec   `json:"spec,omitempty"`
	Status            MachineInventorySelectorStatus `json:"status,omitempty"`
}

type MachineInventorySelectorSpec struct {
	// ProviderID the identifier for the elemental instance.
	// NOTE: Functionality not implemented yet.
	ProviderID string `json:"providerID,omitempty"`
	// Selector selector to choose elemental machines.
	Selector metav1.LabelSelector `json:"selector,omitempty"`
	// Network is the network template to be applied during MachineInventory adoption.
	// +optional
	Network NetworkTemplate `json:"network,omitempty" yaml:"network"`
}

type MachineInventorySelectorStatus struct {
	// Conditions describe the state of the machine selector object.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	Ready bool `json:"ready,omitempty"`
	// Addresses represent machine addresses.
	// +optional
	Addresses clusterv1.MachineAddresses `json:"addresses,omitempty"`
	// BootstrapPlanChecksum represent bootstrap plan checksum.
	// +optional
	BootstrapPlanChecksum string `json:"bootstrapPlanChecksum,omitempty"`
	// MachineInventoryRef reference to the machine inventory that belongs to the selector.
	// +optional
	MachineInventoryRef *corev1.LocalObjectReference `json:"machineInventoryRef,omitempty"`
}

// +kubebuilder:object:root=true

// MachineInventorySelectorList contains a list of MachineInventorySelectors.
type MachineInventorySelectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineInventorySelector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MachineInventorySelector{}, &MachineInventorySelectorList{})
}
