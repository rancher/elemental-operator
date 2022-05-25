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

type MachineRegistration struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
	Metadata   struct {
		Name string `json:"name" yaml:"name"`
	} `json:"metadata" yaml:"metadata"`
	Spec struct {
		CloudConfig map[string]interface{} `json:"cloudConfig" yaml:"cloudConfig"`
	} `json:"spec" yaml:"spec"`
}

func NewMachineRegistration(name string, cloudConfig map[string]interface{}) *MachineRegistration {
	return &MachineRegistration{
		APIVersion: "elemental.cattle.io/v1",
		Metadata: struct {
			Name string "json:\"name\" yaml:\"name\""
		}{Name: name},
		Kind: "MachineRegistration",
		Spec: struct {
			CloudConfig map[string]interface{} "json:\"cloudConfig\" yaml:\"cloudConfig\""
		}{
			CloudConfig: cloudConfig,
		},
	}
}
