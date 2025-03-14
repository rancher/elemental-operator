/*
Copyright © 2022 - 2025 SUSE LLC

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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MachineRegistrationFinalizer = "machineregistration.elemental.cattle.io"
)

type MachineRegistrationSpec struct {
	// +optional
	MachineName string `json:"machineName,omitempty"`
	// MachineInventoryLabels label to be added to the created MachineInventory object.
	// +optional
	MachineInventoryLabels map[string]string `json:"machineInventoryLabels,omitempty"`
	// MachineInventoryAnnotations annotations to be added to the created MachineInventory object.
	// +optional
	MachineInventoryAnnotations map[string]string `json:"machineInventoryAnnotations,omitempty"`
	// Config the cloud config that will be used to provision the node.
	// +optional
	Config Config `json:"config,omitempty"`
}

type MachineRegistrationStatus struct {
	// Conditions describe the state of the machine registration object.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// RegistrationURL is the URL for registering a new machine.
	// +optional
	RegistrationURL string `json:"registrationURL,omitempty"`
	// RegistrationToken a token for registering a machine.
	// +optional
	RegistrationToken string `json:"registrationToken,omitempty"`
	// ServiceAccountRef a reference to the service account created by the machine registration.
	// +optional
	ServiceAccountRef *corev1.ObjectReference `json:"serviceAccountRef,omitempty"` // TODO: use LocalObjectReference
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type MachineRegistration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineRegistrationSpec   `json:"spec,omitempty"`
	Status MachineRegistrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MachineRegistrationList contains a list of MachineRegistrations.
type MachineRegistrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineRegistration `json:"items"`
}

// GetClientRegistrationConfig returns the configuration required by the elemental-register
// to register itself against this MachineRegistration instance.
func (m MachineRegistration) GetClientRegistrationConfig(cacert string) (*Config, error) {
	mRegistration := m.Spec.Config.Elemental.Registration

	if !meta.IsStatusConditionTrue(m.Status.Conditions, ReadyCondition) {
		return nil, fmt.Errorf("machine registration is not ready")
	}

	if m.Status.RegistrationURL == "" {
		return nil, fmt.Errorf("registration URL is not set")
	}

	return &Config{
		Elemental: Elemental{
			Registration: Registration{
				URL:             m.Status.RegistrationURL,
				CACert:          cacert,
				EmulateTPM:      mRegistration.EmulateTPM,
				EmulatedTPMSeed: mRegistration.EmulatedTPMSeed,
				NoSMBIOS:        mRegistration.NoSMBIOS,
				Auth:            mRegistration.Auth,
			},
		},
	}, nil
}

func init() {
	SchemeBuilder.Register(&MachineRegistration{}, &MachineRegistrationList{})
}
