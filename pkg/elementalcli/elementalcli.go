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

package elementalcli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rancher/wrangler/pkg/data/convert"
	"github.com/sirupsen/logrus"
)

const installMediaConfigDir = "/run/initramfs/live/elemental"

func Run(conf map[string]interface{}) error {
	ev := mapToEnv("ELEMENTAL_INSTALL_", conf)

	installerOpts := []string{"elemental"}
	// There are no env var bindings in elemental-cli for elemental root options
	// so root flags should be passed within the command line
	debug, ok := conf["debug"].(bool)
	if ok && debug {
		installerOpts = append(installerOpts, "--debug")
	}
	configDir, ok := conf["config-dir"].(string)
	if ok && configDir != "" {
		installerOpts = append(installerOpts, "--config-dir", configDir)
	} else {
		logrus.Infof("Attempt to load elemental client config from default path: %s", installMediaConfigDir)
		installerOpts = append(installerOpts, "--config-dir", installMediaConfigDir)
	}
	installerOpts = append(installerOpts, "install")

	cmd := exec.Command("elemental")
	cmd.Env = append(os.Environ(), ev...)
	cmd.Stdout = os.Stdout
	cmd.Args = installerOpts
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// it's a mapping of how config env option should be transliterated to the elemental CLI
var defaultOverrides = map[string]string{
	"ELEMENTAL_INSTALL_CONFIG_URLS": "ELEMENTAL_INSTALL_CLOUD_INIT",
	"ELEMENTAL_INSTALL_POWEROFF":    "ELEMENTAL_POWEROFF",
	"ELEMENTAL_INSTALL_REBOOT":      "ELEMENTAL_REBOOT",
	"ELEMENTAL_INSTALL_EJECT_CD":    "ELEMENTAL_EJECT_CD",
	"ELEMENTAL_INSTALL_DEVICE":      "ELEMENTAL_INSTALL_TARGET",
	"ELEMENTAL_INSTALL_SYSTEM_URI":  "ELEMENTAL_INSTALL_SYSTEM",
	"ELEMENTAL_INSTALL_DEBUG":       "ELEMENTAL_DEBUG",
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

		if data, ok := v.(map[string]interface{}); ok {
			subResult := mapToEnv(keyName+"_", data)
			result = append(result, subResult...)
		} else if slice, ok := v.([]interface{}); ok {
			// Convert slices into comma separated values, this is
			// what viper/cobra support on elemental-cli side
			ev := fmt.Sprintf("%s=", keyName)
			for i, s := range slice {
				if i < len(slice)-1 {
					ev += fmt.Sprintf("%v,", s)
				} else {
					ev += fmt.Sprintf("%v", s)
				}
			}
			result = append(result, ev)
		} else {
			result = append(result, fmt.Sprintf("%s=%v", keyName, v))
			logrus.Debugf("%s=%v\n", keyName, v)
		}
	}
	return result
}
