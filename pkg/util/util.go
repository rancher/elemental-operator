/*
Copyright © 2022 - 2023 SUSE LLC

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

package util

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/rancher/elemental-operator/pkg/log"
)

func RemoveInvalidConditions(conditions []metav1.Condition) []metav1.Condition {
	newConditions := []metav1.Condition{}
	for _, cond := range conditions {
		if cond.Type == "" || cond.Status == "" || cond.LastTransitionTime.IsZero() || cond.Reason == "" {
			continue
		}
		newConditions = append(newConditions, cond)
	}
	return newConditions
}

func MarshalCloudConfig(cloudConfig map[string]runtime.RawExtension) ([]byte, error) {
	if len(cloudConfig) == 0 {
		log.Warningf("no cloud-config data to decode")
		return []byte{}, nil
	}

	var err error
	bytes := []byte("#cloud-config\n")

	for k, v := range cloudConfig {
		var jsonData []byte
		if jsonData, err = v.MarshalJSON(); err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}

		var structData interface{}
		if err := json.Unmarshal(jsonData, &structData); err != nil {
			log.Debugf("failed to decode %s (%s): %s", k, string(jsonData), err.Error())
			return nil, fmt.Errorf("%s: %w", k, err)
		}

		var yamlData []byte
		if yamlData, err = yaml.Marshal(structData); err != nil {
			return nil, err
		}

		bytes = append(bytes, append([]byte(fmt.Sprintf("%s:\n", k)), yamlData...)...)
	}

	return bytes, nil
}
