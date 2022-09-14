package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true

type MachineInventorySelectorTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MachineInventorySelectorTemplateSpec `json:"spec,omitempty"`
}

type MachineInventorySelectorTemplateSpec struct {
	Template MachineInventorySelector `json:"template"`
}

// +kubebuilder:object:root=true

// MachineInventorySelectorTemplateList contains a list of MachineInventorySelectorTemplate.
type MachineInventorySelectorTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineInventorySelectorTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MachineInventorySelectorTemplate{}, &MachineInventorySelectorTemplateList{})
}
