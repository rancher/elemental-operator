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

type ManagedOSImage struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
	Metadata   struct {
		Name string `json:"name" yaml:"name"`
	} `json:"metadata" yaml:"metadata"`
	Spec struct {
		OSImage              string                   `json:"osImage" yaml:"osImage"`
		ManagedOSVersionName string                   `json:"managedOSVersionName" yaml:"managedOSVersionName"`
		ClusterTargets       []map[string]interface{} `json:"clusterTargets" yaml:"clusterTargets"`
	}
}

func NewManagedOSImage(name string, clusterTargets []map[string]interface{}, mosImage string, mosVersionName string) *ManagedOSImage {
	return &ManagedOSImage{
		APIVersion: "rancheros.cattle.io/v1",
		Metadata: struct {
			Name string "json:\"name\" yaml:\"name\""
		}{Name: name},
		Kind: "ManagedOSImage",
		Spec: struct {
			OSImage              string                   "json:\"osImage\" yaml:\"osImage\""
			ManagedOSVersionName string                   "json:\"managedOSVersionName\" yaml:\"managedOSVersionName\""
			ClusterTargets       []map[string]interface{} "json:\"clusterTargets\" yaml:\"clusterTargets\""
		}{
			OSImage:              mosImage,
			ManagedOSVersionName: mosVersionName,
			ClusterTargets:       clusterTargets,
		},
	}
}
