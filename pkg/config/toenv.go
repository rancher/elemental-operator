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

package config

import (
	"fmt"
	"strings"

	"github.com/rancher/wrangler/pkg/data/convert"
	"github.com/sirupsen/logrus"
)

// ToEnv converts the config into a slice env.
func ToEnv(inst Install) ([]string, error) {
	// Pass only the elemental values, as those are the ones used by the installer/cli
	// Ignore the Data as that is cloud-config stuff
	data, err := convert.EncodeToMap(&inst)
	if err != nil {
		return nil, err
	}

	return mapToEnv("ELEMENTAL_INSTALL_", data), nil
}

// it's a mapping of how config env option should be transliterated to the elemental CLI
var defaultOverrides = map[string]string{
	"ELEMENTAL_INSTALL_CONFIG_URL": "ELEMENTAL_INSTALL_CLOUD_INIT",
	"ELEMENTAL_INSTALL_POWEROFF":   "ELEMENTAL_POWEROFF",
	"ELEMENTAL_INSTALL_REBOOT":     "ELEMENTAL_REBOOT",
	"ELEMENTAL_INSTALL_EJECT_CD":   "ELEMENTAL_EJECT_CD",
	"ELEMENTAL_INSTALL_DEVICE":     "ELEMENTAL_INSTALL_TARGET",
	"ELEMENTAL_INSTALL_SYSTEM_URI": "ELEMENTAL_INSTALL_SYSTEM",
	"ELEMENTAL_INSTALL_DEBUG":      "ELEMENTAL_DEBUG",
	"ELEMENTAL_INSTALL_PASSWORD":   "SKIP",
	"ELEMENTAL_INSTALL_SSH_KEYS":   "SKIP",
}

func envOverrides(keyName string) string {
	for k, v := range defaultOverrides {
		keyName = strings.ReplaceAll(keyName, k, v)
	}
	return keyName
}

func mapToEnv(prefix string, data map[string]interface{}) []string {
	var result []string

	logrus.Debugln("Computed environment variables:")

	for k, v := range data {
		keyName := strings.ToUpper(prefix + convert.ToYAMLKey(k))
		keyName = strings.ReplaceAll(keyName, "-", "_")
		// Apply overrides needed to convert between configs types
		keyName = envOverrides(keyName)
		if keyName == "SKIP" {
			continue
		}

		if data, ok := v.(map[string]interface{}); ok {
			subResult := mapToEnv(keyName+"_", data)
			result = append(result, subResult...)
		} else {
			result = append(result, fmt.Sprintf("%s=%v", keyName, v))
			logrus.Debugf("%s=%v\n", keyName, v)
		}
	}
	return result
}
