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

package services

import (
	"fmt"

	provv1 "github.com/rancher-sandbox/rancheros-operator/pkg/apis/rancheros.cattle.io/v1"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type JSONSyncer struct {
	URL string `json:"url"`
}

func (*JSONSyncer) sync() ([]provv1.ManagedOSVersion, error) {
	fmt.Println("Synching")
	return []provv1.ManagedOSVersion{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "v1"},
			Spec: provv1.ManagedOSVersionSpec{
				Version:    "v1",
				Type:       "container",
				MinVersion: "0.0.0",
				Metadata: &fleet.GenericMap{
					Data: map[string]interface{}{
						"upgradeImage": "registry.com/repository/image:v1",
					},
				},
			},
		}, {
			ObjectMeta: metav1.ObjectMeta{Name: "v2"},
			Spec: provv1.ManagedOSVersionSpec{
				Version:    "v2",
				Type:       "container",
				MinVersion: "0.0.0",
				Metadata: &fleet.GenericMap{
					Data: map[string]interface{}{
						"upgradeImage": "registry.com/repository/image:v2",
					},
				},
			},
		},
	}, nil
}
