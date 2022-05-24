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
	"github.com/rancher/wrangler/pkg/condition"
	"github.com/rancher/wrangler/pkg/genericcondition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	PlanReadyCondition   = condition.Cond("PlanReady")
	ReadyCondition       = condition.Cond("Ready")
	InitializedCondition = condition.Cond("Initialized")

	PlanSecretType corev1.SecretType = "elemental.cattle.io/plan"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MachineInventory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineInventorySpec   `json:"spec"`
	Status MachineInventoryStatus `json:"status"`
}

type MachineInventorySpec struct {
	TPMHash string `json:"tpmHash,omitempty"`
}

type PlanStatus struct {
	SecretRef       *corev1.ObjectReference `json:"secretRef,omitempty"`
	Checksum        string                  `json:"checksum,omitempty"`
	AppliedChecksum string                  `json:"appliedChecksum,omitempty"`
	FailedChecksum  string                  `json:"failedChecksum,omitempty"`
}

type MachineInventoryStatus struct {
	Conditions []genericcondition.GenericCondition `json:"conditions,omitempty"`
	Plan       *PlanStatus                         `json:"plan,omitempty"`
}
