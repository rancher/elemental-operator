package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	MachineInventoryFinalizer                   = "machineinventory.elemental.cattle.io"
	PlanSecretType            corev1.SecretType = "elemental.cattle.io/plan"
)

type MachineInventorySpec struct {
	// TPMHash the hash of the TPM EK public key. This is used if you are
	// using TPM2 to identifiy nodes.  You can obtain the TPM by
	// running `rancherd get-tpm-hash` on the node. Or nodes can
	// report their TPM hash by using the MachineRegister
	// +optional
	TPMHash string `json:"tpmHash,omitempty"`
}

type MachineInventoryStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// PlanStatus reflect the status of the plan owned by the machine inventory object.
	// +optional
	Plan *PlanStatus `json:"plan,omitempty"`
}

type PlanState string

const (
	PlanApplied PlanState = "Applied"
	PlanFailed  PlanState = "Failed"
)

type PlanStatus struct {
	// PlanSecretRef a reference to the created plan secret
	// +optional
	PlanSecretRef *corev1.ObjectReference `json:"secretRef,omitempty"`
	// Checksum checksum of the created plan
	// +optional
	Checksum string `json:"checksum,omitempty"`
	// +optional
	State PlanState `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type MachineInventory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineInventorySpec   `json:"spec,omitempty"`
	Status MachineInventoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MachineInventoryList contains a list of MachineInventory.
type MachineInventoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineInventory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MachineInventory{}, &MachineInventoryList{})
}