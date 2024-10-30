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
	"gopkg.in/yaml.v3"
)

const TempCloudInitDir = "/tmp/elemental/cloud-init"

type UpgradeConfig struct {
	Debug          bool         `mapstructure:"debug,omitempty"`
	Recovery       bool         `mapstructure:"recovery,omitempty"`
	RecoveryOnly   bool         `mapstructure:"recovery-only,omitempty"`
	System         string       `mapstructure:"system,omitempty"`
	Bootloader     bool         `mapstructure:"bootloader,omitempty"`
	CorrelationID  string       `mapstructure:"correlation-id,omitempty"`
	SnapshotLabels KeyValuePair `mapstructure:"snapshot-labels,omitempty"`
}

type KeyValuePair map[string]string

// KeyValuePairFromData decoded a KeyValuePair object from comma separated strings.
// The expected format is 'myLabel1=foo,myLabel2=bar'.
func KeyValuePairFromData(data interface{}) (KeyValuePair, error) {
	result := map[string]string{}

	str, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("can't unmarshal %+v to a KeyValuePair type", data)
	}

	labels := strings.Split(str, ",")
	for _, label := range labels {
		keyValuePair := strings.Split(label, "=")
		if len(keyValuePair) != 2 {
			return nil, fmt.Errorf("can't unmarshal key/value pair '%s'", label)
		}
		result[keyValuePair[0]] = keyValuePair[1]
	}

	return result, nil
}

type State struct {
	StatePartition PartitionState `yaml:"state,omitempty"`
}

type PartitionState struct {
	Snapshots map[int]*Snapshot `yaml:"snapshots,omitempty"`
}

type Snapshot struct {
	Active bool              `yaml:"active,omitempty"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

type Runner interface {
	Install(elementalv1.Install) error
	Reset(elementalv1.Reset) error
	Upgrade(UpgradeConfig) error
	GetState() (State, error)
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

func (r *runner) Upgrade(conf UpgradeConfig) error {
	installerOpts := []string{"elemental"}
	// There are no env var bindings in elemental-cli for elemental root options
	// so root flags should be passed within the command line
	if conf.Debug {
		installerOpts = append(installerOpts, "--debug")
	}

	// Actual subcommand
	if conf.RecoveryOnly {
		installerOpts = append(installerOpts, "upgrade-recovery")
	} else {
		installerOpts = append(installerOpts, "upgrade")
	}

	if conf.Bootloader {
		installerOpts = append(installerOpts, "--bootloader")
	}

	cmd := exec.Command("elemental")
	environmentVariables := mapToUpgradeEnv(conf)
	cmd.Env = append(os.Environ(), environmentVariables...)
	cmd.Stdout = os.Stdout
	cmd.Args = installerOpts
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	log.Debugf("running: %s\n with ENV:\n%s", strings.Join(installerOpts, " "), strings.Join(environmentVariables, "\n"))
	return cmd.Run()
}

func (r *runner) GetState() (State, error) {
	state := State{}

	log.Debug("Getting elemental state")
	installerOpts := []string{"elemental", "state"}
	cmd := exec.Command("elemental")
	cmd.Args = installerOpts
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	log.Debugf("running: %s", strings.Join(installerOpts, " "))

	var commandOutput []byte
	var err error
	if commandOutput, err = cmd.Output(); err != nil {
		return state, fmt.Errorf("running elemental state: %w", err)
	}
	if err := yaml.Unmarshal(commandOutput, &state); err != nil {
		return state, fmt.Errorf("unmarshalling elemental state: %w", err)
	}

	return state, nil
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
	variables = append(variables, formatEV("ELEMENTAL_SNAPSHOTTER_MAX_SNAPS", fmt.Sprintf("%d", conf.Snapshotter.MaxSnaps)))
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

func mapToUpgradeEnv(conf UpgradeConfig) []string {
	var variables []string
	// See GetUpgradeKeyEnvMap() in https://github.com/rancher/elemental-toolkit/blob/main/pkg/constants/constants.go
	variables = append(variables, formatEV("ELEMENTAL_UPGRADE_RECOVERY", strconv.FormatBool(conf.Recovery)))
	if conf.RecoveryOnly {
		variables = append(variables, formatEV("ELEMENTAL_UPGRADE_RECOVERY_SYSTEM", conf.System))
	} else {
		variables = append(variables, formatEV("ELEMENTAL_UPGRADE_SYSTEM", conf.System))
	}
	if len(conf.SnapshotLabels) > 0 {
		variables = append(variables, formatEV("ELEMENTAL_UPGRADE_SNAPSHOT_LABELS", FormatSnapshotLabels(conf.SnapshotLabels)))
	}
	return variables
}

func formatEV(key string, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}

// --snapshot-labels foo=bar,version=v1.2.3
func FormatSnapshotLabels(labels map[string]string) string {
	formattedLabels := []string{}
	for key, value := range labels {
		if value == "" {
			continue
		}
		formattedLabels = append(formattedLabels, fmt.Sprintf("%s=%s", key, value))
	}
	return strings.Join(formattedLabels, ",")
}
