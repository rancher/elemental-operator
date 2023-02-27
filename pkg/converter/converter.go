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

package converter

import (
	"encoding/json"
	"fmt"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// LegacyConfig is the struct used to pass registration data by legacy Elemental
// operators and legacy register-clients
type LegacyConfig struct {
	Elemental   elementalv1.Elemental  `yaml:"elemental"`
	CloudConfig map[string]interface{} `yaml:"cloud-config,omitempty"`
}

func CloudConfigToLegacy(cloudConf map[string]runtime.RawExtension) (map[string]interface{}, error) {
	legacyCloudConf := make(map[string]interface{})
	for cloudKey, cloudData := range cloudConf {
		var data interface{}
		if err := json.Unmarshal(cloudData.Raw, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal '%s'['%s']: %w", cloudKey, cloudData.Raw, err)
		}
		legacyCloudConf[cloudKey] = data
	}
	return legacyCloudConf, nil
}

func CloudConfigFromLegacy(legacyConf map[string]interface{}) (map[string]runtime.RawExtension, error) {
	cloudConf := make(map[string]runtime.RawExtension)
	for cloudKey, cloudVal := range legacyConf {
		jsonVal, err := encodeToJSON(cloudVal)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal '%s'['%+v']: %w", cloudKey, cloudVal, err)
		}
		extension := runtime.RawExtension{Raw: jsonVal}
		cloudConf[cloudKey] = extension
	}
	return cloudConf, nil
}

func encodeToJSON(val interface{}) ([]byte, error) {
	if m, ok := val.(map[interface{}]interface{}); ok {
		// Convert nested map to map[string]interface{}
		m2 := make(map[string]interface{})
		for k, v := range m {
			k2, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("key is not a string: %v", k)
			}
			m2[k2] = v
		}
		val = m2
	}
	return json.Marshal(val)
}
