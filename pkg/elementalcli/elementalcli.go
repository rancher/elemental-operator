/*
Copyright © 2022 - 2024 SUSE LLC

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
	"strconv"
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
)

const TempCloudInitDir = "/tmp/elemental/cloud-init"

type Runner interface {
	Install(elementalv1.Install) error
	Reset(elementalv1.Reset) error
}

func NewRunner() Runner {
	return &runner{}
}

var _ Runner = (*runner)(nil)

type runner struct{}

func (r *runner) Install(conf elementalv1.Install) error {
	installerOpts := []string{"elemental"}
	// There are no env var bindings in elemental-cli for elemental root options
	// so root flags should be passed within the command line
	if conf.Debug {
		installerOpts = append(installerOpts, "--debug")
	}

	if conf.ConfigDir != "" {
		installerOpts = append(installerOpts, "--config-dir", conf.ConfigDir)
	}
	installerOpts = append(installerOpts, "install")

	cmd := exec.Command("elemental")
	environmentVariables := mapToInstallEnv(conf)
	cmd.Env = append(os.Environ(), environmentVariables...)
	cmd.Stdout = os.Stdout
	cmd.Args = installerOpts
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	log.Debugf("running: %s\n with ENV:\n%s", strings.Join(installerOpts, " "), strings.Join(environmentVariables, "\n"))
	return cmd.Run()
}

func (r *runner) Reset(conf elementalv1.Reset) error {
	installerOpts := []string{"elemental"}
	// There are no env var bindings in elemental-cli for elemental root options
	// so root flags should be passed within the command line
	if conf.Debug {
		installerOpts = append(installerOpts, "--debug")
	}
	installerOpts = append(installerOpts, "reset")

	cmd := exec.Command("elemental")
	environmentVariables := mapToResetEnv(conf)
	cmd.Env = append(os.Environ(), environmentVariables...)
	cmd.Stdout = os.Stdout
	cmd.Args = installerOpts
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	log.Debugf("running: %s\n with ENV:\n%s", strings.Join(installerOpts, " "), strings.Join(environmentVariables, "\n"))
	return cmd.Run()
}

func mapToInstallEnv(conf elementalv1.Install) []string {
	var variables []string
	// See GetInstallKeyEnvMap() in https://github.com/rancher/elemental-toolkit/blob/main/pkg/constants/constants.go
	variables = append(variables, formatEV("ELEMENTAL_INSTALL_TARGET", conf.Device))
	variables = append(variables, formatEV("ELEMENTAL_INSTALL_SYSTEM", conf.SystemURI))
	variables = append(variables, formatEV("ELEMENTAL_INSTALL_FIRMWARE", conf.Firmware))
	variables = append(variables, formatEV("ELEMENTAL_INSTALL_ISO", conf.ISO))
	variables = append(variables, formatEV("ELEMENTAL_INSTALL_TTY", conf.TTY))
	variables = append(variables, formatEV("ELEMENTAL_INSTALL_DISABLE_BOOT_ENTRY", strconv.FormatBool(conf.DisableBootEntry)))
	variables = append(variables, formatEV("ELEMENTAL_INSTALL_NO_FORMAT", strconv.FormatBool(conf.NoFormat)))
	// See GetRunKeyEnvMap() in https://github.com/rancher/elemental-toolkit/blob/main/pkg/constants/constants.go
	variables = append(variables, formatEV("ELEMENTAL_POWEROFF", strconv.FormatBool(conf.PowerOff)))
	variables = append(variables, formatEV("ELEMENTAL_REBOOT", strconv.FormatBool(conf.Reboot)))
	variables = append(variables, formatEV("ELEMENTAL_EJECT_CD", strconv.FormatBool(conf.EjectCD)))
	variables = append(variables, formatEV("ELEMENTAL_SNAPSHOTTER_TYPE", conf.Snapshotter.Type))
	variables = append(variables, formatEV("ELEMENTAL_CLOUD_INIT_PATHS", TempCloudInitDir))
	return variables
}

func mapToResetEnv(conf elementalv1.Reset) []string {
	var variables []string
	// See GetResetKeyEnvMap() in https://github.com/rancher/elemental-toolkit/blob/main/pkg/constants/constants.go
	variables = append(variables, formatEV("ELEMENTAL_RESET_SYSTEM", conf.SystemURI))
	variables = append(variables, formatEV("ELEMENTAL_RESET_PERSISTENT", strconv.FormatBool(conf.ResetPersistent)))
	variables = append(variables, formatEV("ELEMENTAL_RESET_OEM", strconv.FormatBool(conf.ResetOEM)))
	variables = append(variables, formatEV("ELEMENTAL_RESET_DISABLE_BOOT_ENTRY", strconv.FormatBool(conf.DisableBootEntry)))
	// See GetRunKeyEnvMap() in https://github.com/rancher/elemental-toolkit/blob/main/pkg/constants/constants.go
	variables = append(variables, formatEV("ELEMENTAL_POWEROFF", strconv.FormatBool(conf.PowerOff)))
	variables = append(variables, formatEV("ELEMENTAL_REBOOT", strconv.FormatBool(conf.Reboot)))
	variables = append(variables, formatEV("ELEMENTAL_CLOUD_INIT_PATHS", TempCloudInitDir))
	return variables
}

func formatEV(key string, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}
