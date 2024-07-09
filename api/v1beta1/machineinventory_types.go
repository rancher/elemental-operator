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
)

const (
	MachineInventoryFinalizer                               = "machineinventory.elemental.cattle.io"
	PlanSecretType                        corev1.SecretType = "elemental.cattle.io/plan"
	PlanTypeAnnotation                                      = "elemental.cattle.io/plan.type"
	PlanTypeEmpty                                           = "empty"
	PlanTypeBootstrap                                       = "bootstrap"
	PlanTypeReset                                           = "reset"
	MachineInventoryResettableAnnotation                    = "elemental.cattle.io/resettable"
	MachineInventoryOSUnmanagedAnnotation                   = "elemental.cattle.io/os.unmanaged"
)

type MachineInventorySpec struct {
	// TPMHash the hash of the TPM EK public key. This is used if you are
	// using TPM2 to identifiy nodes.  You can obtain the TPM by
	// running `rancherd get-tpm-hash` on the node. Or nodes can
	// report their TPM hash by using the MachineRegister.
	// +optional
	TPMHash string `json:"tpmHash,omitempty"`
	// MachineHash the hash of the identifier used by the host to identify
	// to the operator. This is used when the host authenticates without TPM.
	// Both the authentication method and the identifier used to derive the hash
	// depend upon the MachineRegistration spec.config.elemental.registration.auth value.
	// +optional
	MachineHash string `json:"machineHash,omitempty"`
	// IPAddressRef the reference to the IPAddress that should be applied to the
	// machine at installation time.
	// +optional
	IPAddressClaimRef *corev1.ObjectReference `json:"ipAddressClaimRef,omitempty"`
}

type MachineInventoryStatus struct {
	// Conditions describe the state of the machine inventory object.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// PlanStatus reflect the status of the plan owned by the machine inventory object.
	// +optional
	Plan *PlanStatus `json:"plan,omitempty"`
	// IPAddressRef contains the reference to the IPAddress generated from the IPAddressClaim in IPAddressClaimRef
	// +optional
	IPAddressRef *corev1.ObjectReference `json:"ipAddressRef,omitempty"`
}

type PlanState string

const (
	PlanApplied PlanState = "Applied"
	PlanFailed  PlanState = "Failed"
)

type PlanStatus struct {
	// PlanSecretRef a reference to the created plan secret.
	// +optional
	PlanSecretRef *corev1.ObjectReference `json:"secretRef,omitempty"`
	// Checksum checksum of the created plan.
	// +optional
	Checksum string `json:"checksum,omitempty"`
	// State reflect state of the plan that belongs to the machine inventory.
	// +kubebuilder:validation:Enum=Applied;Failed
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

// MachineInventoryList contains a list of MachineInventories.
type MachineInventoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineInventory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MachineInventory{}, &MachineInventoryList{})
}
