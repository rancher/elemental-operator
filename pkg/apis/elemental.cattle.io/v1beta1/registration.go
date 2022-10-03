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
	"github.com/rancher/elemental-operator/pkg/config"
	"github.com/rancher/wrangler/pkg/genericcondition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MachineRegistrationReadyReason = "MachineRegistrationReady"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MachineRegistration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineRegistrationSpec   `json:"spec"`
	Status MachineRegistrationStatus `json:"status"`
}

type MachineRegistrationSpec struct {
	MachineName                 string            `yaml:"machineName,omitempty" json:"machineName,omitempty"`
	MachineInventoryLabels      map[string]string `yaml:"machineInventoryLabels,omitempty" json:"machineInventoryLabels,omitempty"`
	MachineInventoryAnnotations map[string]string `yaml:"machineInventoryAnnotations,omitempty" json:"machineInventoryAnnotations,omitempty"`
	Config                      *config.Config    `yaml:"config,omitempty" json:"config,omitempty"`
}

type MachineRegistrationStatus struct {
	Conditions        []genericcondition.GenericCondition `json:"conditions,omitempty"`
	RegistrationURL   string                              `json:"registrationURL,omitempty"`
	RegistrationToken string                              `json:"registrationToken,omitempty"`
	ServiceAccountRef *corev1.ObjectReference             `json:"serviceAccountRef,omitempty"`
}
