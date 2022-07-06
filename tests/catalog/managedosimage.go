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
	"time"
)

type DrainSpec struct {
	Timeout                  *time.Duration `json:"timeout,omitempty" yaml:"timeout"`
	GracePeriod              *int32         `json:"gracePeriod,omitempty" yaml:"gracePeriod"`
	DeleteLocalData          *bool          `json:"deleteLocalData,omitempty" yaml:"deleteLocalData"`
	IgnoreDaemonSets         *bool          `json:"ignoreDaemonSets,omitempty" yaml:"ignoreDaemonSets"`
	Force                    bool           `json:"force,omitempty" yaml:"force"`
	DisableEviction          bool           `json:"disableEviction,omitempty" yaml:"disableEviction"`
	SkipWaitForDeleteTimeout int            `json:"skipWaitForDeleteTimeout,omitempty" yaml:"skipWaitForDeleteTimeout"`
}

type ManagedOSImage struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
	Metadata   struct {
		Name string `json:"name" yaml:"name"`
	} `json:"metadata" yaml:"metadata"`

	Spec struct {
		Cordon               *bool                    `json:"cordon,omitempty" yaml:"cordon"`
		Drain                *DrainSpec               `json:"drain,omitempty" yaml:"drain"`
		OSImage              string                   `json:"osImage" yaml:"osImage"`
		ManagedOSVersionName string                   `json:"managedOSVersionName" yaml:"managedOSVersionName"`
		Targets              []map[string]interface{} `json:"clusterTargets" yaml:"clusterTargets"`
	}
}

func NewManagedOSImage(name string, targets []map[string]interface{}, mosImage string, mosVersionName string) *ManagedOSImage {
	cordon := false

	return &ManagedOSImage{
		APIVersion: "elemental.cattle.io/v1beta1",
		Metadata: struct {
			Name string "json:\"name\" yaml:\"name\""
		}{Name: name},
		Kind: "ManagedOSImage",
		Spec: struct {
			Cordon               *bool                    `json:"cordon,omitempty" yaml:"cordon"`
			Drain                *DrainSpec               `json:"drain,omitempty" yaml:"drain"`
			OSImage              string                   `json:"osImage" yaml:"osImage"`
			ManagedOSVersionName string                   `json:"managedOSVersionName" yaml:"managedOSVersionName"`
			Targets              []map[string]interface{} `json:"clusterTargets" yaml:"clusterTargets"`
		}{
			OSImage:              mosImage,
			ManagedOSVersionName: mosVersionName,
			Cordon:               &cordon,
			Targets:              targets,
		},
	}
}

func DrainOSImage(name string, managedOSVersion string, drainSpec *DrainSpec) *ManagedOSImage {
	cordon := false
	return &ManagedOSImage{
		APIVersion: "elemental.cattle.io/v1beta1",
		Metadata: struct {
			Name string "json:\"name\" yaml:\"name\""
		}{Name: name},
		Kind: "ManagedOSImage",
		Spec: struct {
			Cordon               *bool                    `json:"cordon,omitempty" yaml:"cordon"`
			Drain                *DrainSpec               `json:"drain,omitempty" yaml:"drain"`
			OSImage              string                   `json:"osImage" yaml:"osImage"`
			ManagedOSVersionName string                   `json:"managedOSVersionName" yaml:"managedOSVersionName"`
			Targets              []map[string]interface{} `json:"clusterTargets" yaml:"clusterTargets"`
		}{
			Cordon:               &cordon,
			ManagedOSVersionName: managedOSVersion,
			Drain:                drainSpec,
		},
	}
}
