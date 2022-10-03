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
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	MachineInventorySelectorReadyReason = "MachineInventorySelectorReady"
	BootstrapReadyReason                = "BootstrapReady"
	WaitingForMachineInventoryReason    = "WaitingForMachineInventory"
	WaitingForBootstrapReason           = "WaitingForBootstrapReason"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MachineInventorySelector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MachineInventorySelectorSpec   `json:"spec"`
	Status            MachineInventorySelectorStatus `json:"status"`
}

type MachineInventorySelectorSpec struct {
	ProviderID string               `json:"providerID"`
	Selector   metav1.LabelSelector `json:"selector,omitempty"`
}

type MachineInventorySelectorStatus struct {
	Conditions            []genericcondition.GenericCondition `json:"conditions,omitempty"`
	Ready                 bool                                `json:"ready,omitempty"`
	Addresses             capi.MachineAddresses               `json:"addresses,omitempty"`
	BootstrapPlanChecksum string                              `json:"bootstrapPlanChecksum,omitempty"`
	MachineInventoryRef   *corev1.ObjectReference             `json:"machineInventoryRef,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MachineInventorySelectorTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MachineInventorySelectorTemplateSpec `json:"spec"`
}

type MachineInventorySelectorTemplateSpec struct {
	Template MachineInventorySelector `json:"template"`
}

var (
	InventoryReadyCondition = condition.Cond("InventoryReady")
	BootstrapReadyCondition = condition.Cond("BootstrapReady")
)
