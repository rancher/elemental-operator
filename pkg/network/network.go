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
	"path/filepath"
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/util"
	"github.com/rancher/yip/pkg/schema"
	"github.com/twpayne/go-vfs"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	nmcDesiredStatesDir  = "/tmp/desired-states"
	nmcNewtorkConfigDir  = "/tmp/network-config"
	nmcAllConfigName     = "_all.yaml"
	systemConnectionsDir = "/etc/NetworkManager/system-connections"
	configApplicator     = "/oem/99-network-config-applicator.yaml"
)

var (
	ErrEmptyConfig = errors.New("Network config is empty")
)

type Configurator interface {
	GetNetworkConfigApplicator(networkConfig elementalv1.NetworkConfig) (schema.YipConfig, error)
	ResetNetworkConfig() error
}

var _ Configurator = (*nmcConfigurator)(nil)

func NewConfigurator(fs vfs.FS) Configurator {
	return &nmcConfigurator{
		fs:     fs,
		runner: &util.ExecRunner{},
	}
}

type nmcConfigurator struct {
	fs     vfs.FS
	runner util.CommandRunner
}

func (n *nmcConfigurator) GetNetworkConfigApplicator(networkConfig elementalv1.NetworkConfig) (schema.YipConfig, error) {
	configApplicator := schema.YipConfig{}

	if len(networkConfig.Config) == 0 {
		log.Warning("no network config data to decode")
		return configApplicator, ErrEmptyConfig
	}

	// This creates a parent "root" key to facilitate parsing the schemaless map
	mapSlice := k8syaml.JSONObjectToYAMLObject(map[string]interface{}{"root": networkConfig.Config})
	if len(mapSlice) <= 0 {
		return configApplicator, errors.New("Could not convert json cloudConfig object to yaml")
	}

	// Just marshal the value of the "root" key
	yamlData, err := k8syaml.Marshal(mapSlice[0].Value)
	if err != nil {
		return configApplicator, fmt.Errorf("marshalling yaml: %w", err)
	}

	yamlStringData := string(yamlData)

	// Go through the nmc yaml config and replace template placeholders "{my-ip-name}" with actual IP.
	for name, ipAddress := range networkConfig.IPAddresses {
		yamlStringData = strings.ReplaceAll(yamlStringData, fmt.Sprintf("{%s}", name), ipAddress)
	}

	// Dump the digested config somewhere
	if err := vfs.MkdirAll(n.fs, nmcDesiredStatesDir, 0700); err != nil {
		return configApplicator, fmt.Errorf("creating dir '%s': %w", nmcDesiredStatesDir, err)
	}
	nmcAllConfigPath := filepath.Join(nmcDesiredStatesDir, nmcAllConfigName)
	if err := n.fs.WriteFile(nmcAllConfigPath, []byte(yamlStringData), 0600); err != nil {
		return configApplicator, fmt.Errorf("writing file '%s': %w", nmcAllConfigPath, err)
	}

	// Generate configurations
	if err := vfs.MkdirAll(n.fs, nmcNewtorkConfigDir, 0700); err != nil {
		return configApplicator, fmt.Errorf("creating dir '%s': %w", nmcNewtorkConfigDir, err)
	}
	if err := n.runner.Run("nmc", "generate", "--config-dir", nmcDesiredStatesDir, "--output-dir", nmcNewtorkConfigDir); err != nil {
		return configApplicator, fmt.Errorf("running: nmc generate: %w", err)
	}

	// Apply configurations
	if err := n.runner.Run("nmc", "apply", "--config-dir", nmcNewtorkConfigDir); err != nil {
		return configApplicator, fmt.Errorf("running: nmc apply: %w", err)
	}

	// Now fetch all /etc/NetworkManager/system-connections/*.nmconnection files.
	// Each file is added to the configApplicator.
	files, err := n.fs.ReadDir(systemConnectionsDir)
	if err != nil {
		return configApplicator, fmt.Errorf("reading directory '%s': %w", systemConnectionsDir, err)
	}

	yipFiles := []schema.File{}
	for _, file := range files {
		fileName := file.Name()
		if strings.HasSuffix(fileName, ".nmconnection") {
			bytes, err := n.fs.ReadFile(filepath.Join(systemConnectionsDir, fileName))
			if err != nil {
				return configApplicator, fmt.Errorf("reading file '%s': %w", fileName, err)
			}
			yipFiles = append(yipFiles, schema.File{
				Path:        filepath.Join(systemConnectionsDir, fileName),
				Permissions: 0600,
				Content:     string(bytes),
			})
		}
	}

	// Wrap up the yip config
	configApplicator.Name = "Apply network config"
	configApplicator.Stages = map[string][]schema.Stage{
		"initramfs": {
			schema.Stage{
				If:    "[ ! -f /run/elemental/recovery_mode ]",
				Files: yipFiles,
			},
		},
	}

	return configApplicator, nil
}

// ResetNetworkConfig is invoked during reset trigger.
// It is important that once the elemental-system-agent marks the reset-trigger plan as completed,
// the assigned IPs are no longer used.
// This to prevent any race condition in which the remote MachineInventory is deleted,
// the associated IPAddressClaims are also deleted, but the machine is still using the IPAddresses from those claims.
//
// Also note that this method is called twice, since the first time we restart the elemental-system-agent.
// The reason for that restart is that after restarting the NetworkManager, the elemental-system-agent process hangs
// waiting for the (old) connection timeout. This can lead to the scheduled shutdown to trigger (and reset from recovery start),
// before the elemental-system-agent has time to recover from the connection change and confirm the application of the reset-trigger plan.
// Potentially this can lead to an infinite reset loop.
func (n *nmcConfigurator) ResetNetworkConfig() error {
	// If there are no /etc/NetworkManager/system-connections/*.nmconnection files,
	// then this is the second time we invoke this method, or we never configured any
	// network config so we have nothing to do.
	connectionFiles, err := n.fs.ReadDir(systemConnectionsDir)
	if err != nil {
		return fmt.Errorf("reading files in dir '%s': %w", systemConnectionsDir, err)
	}
	needsReset := false
	for _, file := range connectionFiles {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".nmconnection") {
			needsReset = true
			break
		}
	}
	if !needsReset {
		log.Debug("Network is already reset, nothing to do")
		return nil
	}

	// Delete all .nmconnection files. This will also delete any "static" connection that the user defined in the base image for example.
	// Which means maybe that is not supported anymore, or if we want to support it we should make sure we only delete the ones created by elemental,
	// for example prefixing all files with "elemental-" or just parsing the network config again at this stage to determine the file names.
	log.Debug("Deleting all .nmconnection configs")
	if err := n.runner.Run("find", systemConnectionsDir, "-name", "*.nmconnection", "-type", "f", "-delete"); err != nil {
		return fmt.Errorf("deleting all %s/*.nmconnection: %w", systemConnectionsDir, err)
	}

	//TODO: Here is where we can create "first boot" connections, for example by parsing the network config from the remote registration.
	//      In this way we can always reset to a deterministic network configuration, rather than simply delete all connections and revert on default (dhcp)

	// We need to invoke nmcli connection reload to tell NetworkManager to reload connections from disk.
	// NetworkManager won't reload them alone with a simple restart.
	log.Debug("Reloading connections")
	if err := n.runner.Run("nmcli", "connection", "reload"); err != nil {
		return fmt.Errorf("running: nmcli connection reload: %w", err)
	}

	// Restart NetworkManager to restart connections.
	log.Debug("Restarting NetworkManager")
	if err := n.runner.Run("systemctl", "restart", "NetworkManager.service"); err != nil {
		return fmt.Errorf("running command: systemctl restart NetworkManager.service: %w", err)
	}

	// Not entirely necessary, but this mitigates the risk of continuing with any potential elemental-system-agent
	// plan confirmation while the network is offline.
	log.Debug("Waiting NetworkManager online")
	if err := n.runner.Run("systemctl", "start", "NetworkManager-wait-online.service"); err != nil {
		return fmt.Errorf("running command: systemctl start NetworkManager-wait-online.service: %w", err)
	}

	// Restarts the elemental-system-agent to start a new connection using the new config.
	// This will make the plan be executed a second time.
	log.Debug("Restarting elemental-system-agent")
	if err := n.runner.Run("systemctl", "restart", "elemental-system-agent.service"); err != nil {
		return fmt.Errorf("running command: systemctl restart elemental-system-agent.service: %w", err)
	}
	return nil
}
