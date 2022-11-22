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
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

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

func LegacyNewManagedOSVersion(name string, version string, minVersion string, metadata map[string]interface{}, upgradeC *ContainerSpec) *ManagedOSVersion {
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

func NewManagedOSVersion(namespace string, name string, version string, minVersion string, metadata map[string]runtime.RawExtension, upgradeC *upgradev1.ContainerSpec) *elementalv1.ManagedOSVersion {
	return &elementalv1.ManagedOSVersion{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "elemental.cattle.io/v1beta1",
			Kind:       "ManagedOSVersion",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: elementalv1.ManagedOSVersionSpec{
			Type:             "container",
			Version:          version,
			MinVersion:       minVersion,
			Metadata:         metadata,
			UpgradeContainer: upgradeC,
		},
	}
}
