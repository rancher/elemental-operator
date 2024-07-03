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

package network

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/yip/pkg/schema"
	"github.com/twpayne/go-vfs"
	"sigs.k8s.io/yaml"
)

const (
	firstBootConfigTempPath  = "/tmp/first-boot-network-config.yaml"
	firstBootConfigFinalPath = "/oem/network/first-boot-network-config.yaml"
	appliedConfig            = "/oem/network/applied-config.yaml"
	ConfigApplicator         = "/oem/99-network-config-applicator.yaml"
)

type Configurator interface {
	// GetFirstBootConfig is invoked at first boot from the installation media.
	// The returned YipConfig will be passed to the elemental cli during the install phase,
	// to be installed in the system.
	GetFirstBootConfig() (schema.YipConfig, error)
	// RestoreFirstBootConfig is invoked during the trigger reset process.
	// This assumes the FirstBootConfig can still contact the Rancher endpoint to confirm reset.
	RestoreFirstBootConfig() error
	// ApplyConfig should apply a given config to the system.
	ApplyConfig(elementalv1.NetworkConfig) error
}

var _ Configurator = (*nmstateConfigurator)(nil)

func NewConfigurator(fs vfs.FS) Configurator {
	return &nmstateConfigurator{fs: fs}
}

type nmstateConfigurator struct {
	fs vfs.FS
}

func (n *nmstateConfigurator) GetFirstBootConfig() (schema.YipConfig, error) {
	config := schema.YipConfig{}

	// 1. Dump the current config into a tmp file
	cmd := exec.Command("nmstatectl")
	cmd.Args = []string{"show", ">", firstBootConfigTempPath}
	if err := cmd.Run(); err != nil {
		return config, fmt.Errorf("running nmstatectl show: %w", err)
	}

	// 2. Read the file
	bytes, err := n.fs.ReadFile(firstBootConfigTempPath)
	if err != nil {
		return config, fmt.Errorf("reading file '%s': %w", firstBootConfigTempPath, err)
	}

	// 3. Create installable yip config to persist the current config
	config.Name = "Persist First Boot Network configuration"
	config.Stages = map[string][]schema.Stage{
		"initramfs": {
			schema.Stage{
				If: fmt.Sprintf("[ ! -f %s ]", firstBootConfigFinalPath),
				Directories: []schema.Directory{
					{
						Path:        filepath.Dir(firstBootConfigFinalPath),
						Permissions: 0700,
					},
				}, Files: []schema.File{
					{
						Path:        firstBootConfigFinalPath,
						Content:     string(bytes),
						Permissions: 0600,
					},
				},
			},
		},
	}

	return config, nil
}

func (n *nmstateConfigurator) RestoreFirstBootConfig() error {
	// Delete config applicator to prevent re-applying the config again
	if err := n.fs.Remove(ConfigApplicator); err != nil {
		return fmt.Errorf("deleting file '%s': %w", ConfigApplicator, err)
	}
	// Also delete the previously applied config, just in case
	if err := n.fs.Remove(appliedConfig); err != nil {
		return fmt.Errorf("deleting file '%s': %w", appliedConfig, err)
	}

	cmd := exec.Command("nmstatectl")
	cmd.Args = []string{"apply", firstBootConfigFinalPath}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running nmstatectl apply %s: %w", firstBootConfigFinalPath, err)
	}
	return nil
}

func (n *nmstateConfigurator) ApplyConfig(config elementalv1.NetworkConfig) error {
	if len(config.Config) == 0 {
		log.Warning("no network config data to decode")
		return nil
	}

	// This creates a parent "root" key to facilitate parsing the schemaless map
	mapSlice := yaml.JSONObjectToYAMLObject(map[string]interface{}{"root": config.Config})
	if len(mapSlice) <= 0 {
		return errors.New("Could not convert json cloudConfig object to yaml")
	}

	// Just marshal the value of the "root" key
	yamlData, err := yaml.Marshal(mapSlice[0].Value)
	if err != nil {
		return fmt.Errorf("marshalling yaml: %w", err)
	}

	yamlStringData := string(yamlData)

	// Go through the nmstate yaml config and replace template placeholders "{my-ip-name}" with actual IP.
	for name, ipAddress := range config.IPAddresses {
		yamlStringData = strings.ReplaceAll(yamlStringData, fmt.Sprintf("{%s}", name), ipAddress)
	}

	// Dump the digested config somewhere
	if err := vfs.MkdirAll(n.fs, filepath.Dir(appliedConfig), 0700); err != nil {
		return fmt.Errorf("creating directory for file '%s': %w", appliedConfig, err)
	}
	if err := n.fs.WriteFile(appliedConfig, []byte(yamlStringData), 0600); err != nil {
		return fmt.Errorf("writing file '%s': %w", appliedConfig, err)
	}

	// Try to apply it
	cmd := exec.Command("nmstatectl", "apply", appliedConfig)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running nmstatectl apply %s: %w", appliedConfig, err)
	}

	// Finally, "persist" the application so that it will stick in recovery mode as well
	networkConfigApplicator := schema.YipConfig{}
	networkConfigApplicator.Name = "Apply network config"
	networkConfigApplicator.Stages = map[string][]schema.Stage{
		"initramfs": {
			schema.Stage{
				If: fmt.Sprintf("[ ! -f %s ]", appliedConfig),
				Directories: []schema.Directory{
					{
						Path:        filepath.Dir(appliedConfig),
						Permissions: 0700,
					},
				}, Files: []schema.File{
					{
						Path:        appliedConfig,
						Content:     yamlStringData,
						Permissions: 0600,
					},
				},
			},
		},
		"network.after": {
			schema.Stage{
				Commands: []string{fmt.Sprintf("nmstatectl apply %s", appliedConfig)},
			},
		},
	}
	networkConfigApplicatorBytes, err := yaml.Marshal(networkConfigApplicator)
	if err != nil {
		return fmt.Errorf("marshalling network config applicator: %w", err)
	}
	if err := n.fs.WriteFile(ConfigApplicator, networkConfigApplicatorBytes, 0600); err != nil {
		return fmt.Errorf("writing file '%s': %w", ConfigApplicator, err)
	}

	return nil
}
