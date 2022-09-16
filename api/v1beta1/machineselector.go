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
	ProviderID string               `json:"providerID,omitempty"`
	Selector   metav1.LabelSelector `json:"selector,omitempty"`
}

type MachineInventorySelectorStatus struct {
	// Conditions describe the state of the machine selector object.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	Ready bool `json:"ready,omitempty"`
	// Addresses machine addresses
	// +optional
	Addresses clusterv1.MachineAddresses `json:"addresses,omitempty"`
	// BootstrapPlanChecksum bootstrap plan checksum
	// +optional
	BootstrapPlanChecksum string `json:"bootstrapPlanChecksum,omitempty"`
	// MachineInventoryRef reference to the machine inventory that belongs to the selector
	// +optional
	MachineInventoryRef *corev1.ObjectReference `json:"machineInventoryRef,omitempty"` // TODO: use LocalObjectReference
}

// +kubebuilder:object:root=true

// MachineInventorySelectorList contains a list of MachineInventorySelector.
type MachineInventorySelectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineInventorySelector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MachineInventorySelector{}, &MachineInventorySelectorList{})
}
