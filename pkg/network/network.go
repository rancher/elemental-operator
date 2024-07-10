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
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/yip/pkg/schema"
	"github.com/twpayne/go-vfs"
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

func NewConfigurator(fs vfs.FS) Configurator {
	return &networkManagerConfigurator{
		fs: fs,
	}
}

type networkManagerConfigurator struct {
	fs vfs.FS
}

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
				If:    "[ -f /run/elemental/active_mode ]",
				Files: files,
			},
		},
	}

	return config
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
func (n *networkManagerConfigurator) ResetNetworkConfig() error {
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
	cmd := exec.Command("find", systemConnectionsDir, "-name", "*.nmconnection", "-type", "f", "-delete")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("deleting all %s/*.nmconnection: %w", systemConnectionsDir, err)
	}

	//TODO: Here is where we can create "first boot" connections, for example by parsing the network config from the remote registration.
	//      In this way we can always reset to a deterministic network configuration, rather than simply delete all connections and revert on default (dhcp)

	// We need to invoke nmcli connection reload to tell NetworkManager to reload connections from disk.
	// NetworkManager won't reload them alone with a simple restart.
	log.Debug("Reloading connections")
	cmd = exec.Command("nmcli", "connection", "reload")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running: nmcli connection reload: %w", err)
	}

	// Restart NetworkManager to restart connections.
	log.Debug("Restarting NetworkManager")
	cmd = exec.Command("systemctl", "restart", "NetworkManager.service")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running command: systemctl restart NetworkManager.service: %w", err)
	}

	// Not entirely necessary, but this mitigates the risk of continuing with any potential elemental-system-agent
	// plan confirmation while the network is offline.
	log.Debug("Waiting NetworkManager online")
	cmd = exec.Command("systemctl", "start", "NetworkManager-wait-online.service")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running command: systemctl start NetworkManager-wait-online.service: %w", err)
	}

	// Restarts the elemental-system-agent to start a new connection using the new config.
	// This will make the plan be executed a second time.
	log.Debug("Restarting elemental-system-agent")
	cmd = exec.Command("systemctl", "restart", "elemental-system-agent.service")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running command: systemctl restart elemental-system-agent.service: %w", err)
	}
	return nil
}
