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

package catalog

import (
	"github.com/rancher/elemental-operator/pkg/config"
)

type MachineRegistrationSpec struct {
	MachineName                 string            `yaml:"machineName,omitempty" json:"machineName,omitempty"`
	MachineInventoryLabels      map[string]string `yaml:"machineInventoryLabels,omitempty" json:"machineInventoryLabels,omitempty"`
	MachineInventoryAnnotations map[string]string `yaml:"machineInventoryAnnotations,omitempty" json:"machineInventoryAnnotations,omitempty"`
	Install                     *config.Install   `yaml:"install,omitempty" json:"install,omitempty"`
}

type MachineRegistration struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
	Metadata   struct {
		Name string `json:"name" yaml:"name"`
	} `json:"metadata" yaml:"metadata"`
	Spec MachineRegistrationSpec `json:"spec" yaml:"spec"`
}

func NewMachineRegistration(name string, spec MachineRegistrationSpec) *MachineRegistration {
	return &MachineRegistration{
		APIVersion: "elemental.cattle.io/v1beta1",
		Metadata: struct {
			Name string "json:\"name\" yaml:\"name\""
		}{Name: name},
		Kind: "MachineRegistration",
		Spec: spec,
	}
}
