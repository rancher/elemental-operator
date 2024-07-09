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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/yip/pkg/schema"
)

const (
	firstBootConfigTempPath = "/tmp/first-boot-network-config.yaml"
	firstBootConfigPath     = "/oem/network/first-boot-network-config.yaml"
	systemConnectionsDir    = "/etc/NetworkManager/system-connections"
	configApplicator        = "/oem/99-network-config-applicator.yaml"
)

type Configurator interface {
	GetNetworkConfigApplicator(networkConfig elementalv1.NetworkConfig) schema.YipConfig
	ResetNetworkConfig() error
}

var _ Configurator = (*networkManagerConfigurator)(nil)

func NewConfigurator() Configurator {
	return &networkManagerConfigurator{}
}

type networkManagerConfigurator struct{}

func (n *networkManagerConfigurator) GetNetworkConfigApplicator(networkConfig elementalv1.NetworkConfig) schema.YipConfig {
	// Replace the "{my-ip-name}" placeholders with real IPAddresses
	for connectionName, connectionConfig := range networkConfig.Connections {
		for ipName, ipAddress := range networkConfig.IPAddresses {
			networkConfig.Connections[connectionName] = strings.ReplaceAll(connectionConfig, fmt.Sprintf("{%s}", ipName), ipAddress)
		}
	}

	// Add files to the yip config
	files := []schema.File{}
	for connectionName, connectionConfig := range networkConfig.Connections {
		files = append(files, schema.File{
			Path:        filepath.Join(systemConnectionsDir, fmt.Sprintf("%s.nmconnection", connectionName)),
			Permissions: 0600,
			Content:     connectionConfig,
		})
	}

	config := schema.YipConfig{}
	config.Name = "Apply network config"
	config.Stages = map[string][]schema.Stage{
		"initramfs": {
			schema.Stage{
				If: "[ -f /run/elemental/active_mode ]",
				Directories: []schema.Directory{
					{
						Path:        "systemConnectionsDir",
						Permissions: 0700,
					},
				},
				Files: files,
			},
		},
	}

	return config
}

func (n *networkManagerConfigurator) ResetNetworkConfig() error {
	cmd := exec.Command("find", systemConnectionsDir, "-name", "\"*.nmconnection\"", "-type", "f", "-delete")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("deleting all %s/*.nmconnection: %w", systemConnectionsDir, err)
	}

	cmd = exec.Command("nmcli", "connection", "reload")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running: nmcli connection reload: %w", err)
	}

	cmd = exec.Command("systemctl", "restart", "NetworkManager.service")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running command: systemctl restart NetworkManager.service: %w", err)
	}
	return nil
}
