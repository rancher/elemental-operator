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

type ManagedOSVersionSpec struct {
	Type             string                 `json:"type" yaml:"type"`
	Version          string                 `json:"version" yaml:"version"`
	MinVersion       string                 `json:"minVersion" yaml:"minVersion"`
	Metadata         map[string]interface{} `json:"metadata" yaml:"metadata"`
	UpgradeContainer *ContainerSpec         `json:"upgradeContainer" yaml:"upgradeContainer"`
}

type ManagedOSVersion struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
	Metadata   struct {
		Name string `json:"name" yaml:"name"`
	} `json:"metadata" yaml:"metadata"`
	Spec ManagedOSVersionSpec
}

type ContainerSpec struct {
	Image   string   `json:"image,omitempty" yaml:"image,omitempty"`
	Command []string `json:"command,omitempty" yaml:"command,omitempty"`
	Args    []string `json:"args,omitempty" yaml:"args,omitempty"`
}

func NewManagedOSVersion(name string, version string, minVersion string, metadata map[string]interface{}, upgradeC *ContainerSpec) *ManagedOSVersion {
	return &ManagedOSVersion{
		APIVersion: "elemental.cattle.io/v1beta1",
		Metadata: struct {
			Name string "json:\"name\" yaml:\"name\""
		}{Name: name},
		Kind: "ManagedOSVersion",
		Spec: ManagedOSVersionSpec{
			Type:             "container",
			Version:          version,
			MinVersion:       minVersion,
			Metadata:         metadata,
			UpgradeContainer: upgradeC,
		},
	}
}
