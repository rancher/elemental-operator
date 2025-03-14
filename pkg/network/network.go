/*
Copyright © 2022 - 2025 SUSE LLC

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
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/util"
	"github.com/rancher/yip/pkg/schema"
	"github.com/twpayne/go-vfs"
)

const (
	ConfiguratorNone          = "none"
	ConfiguratorNmc           = "nmc"
	ConfiguratorNmstate       = "nmstate"
	ConfiguratorNmconnections = "nmconnections"
)

const (
	// common
	systemConnectionsDir = "/etc/NetworkManager/system-connections"
	configApplicator     = "/oem/elemental-network.yaml"
	// nmc intermediate
	nmcDesiredStatesDir = "/tmp/declarative-networking/nmc/desired-states"
	nmcNewtorkConfigDir = "/tmp/declarative-networking/nmc/network-config"
	nmcAllConfigName    = "_all.yaml"
	// nmstate intermediate
	nmstateTempPath = "/tmp/declarative-networking/elemental-nmstate.yaml"
	// yip Applicator config
	applicatorName  = "Apply network config"
	applicatorStage = "initramfs"
)

var (
	ErrEmptyConfig         = errors.New("Network config is empty")
	ErrUnknownConfigurator = errors.New("Unknown network configurator type")
)

type Configurator interface {
	GetNetworkConfigApplicator(networkConfig elementalv1.NetworkConfig) (schema.YipConfig, error)
	ResetNetworkConfig() error
}

var _ Configurator = (*configurator)(nil)

type configurator struct {
	fs     vfs.FS
	runner util.CommandRunner
}

func NewConfigurator(fs vfs.FS) Configurator {
	return &configurator{
		fs:     fs,
		runner: &util.ExecRunner{},
	}
}

func (c *configurator) GetNetworkConfigApplicator(networkConfig elementalv1.NetworkConfig) (schema.YipConfig, error) {
	switch networkConfig.Configurator {
	case "":
		return schema.YipConfig{}, nil
	case ConfiguratorNone:
		return schema.YipConfig{}, nil
	case ConfiguratorNmc:
		nc := nmcConfigurator{fs: c.fs, runner: c.runner}
		return nc.GetNetworkConfigApplicator(networkConfig)
	case ConfiguratorNmstate:
		nc := nmstateConfigurator{fs: c.fs, runner: c.runner}
		return nc.GetNetworkConfigApplicator(networkConfig)
	case ConfiguratorNmconnections:
		nc := networkManagerConfigurator{fs: c.fs, runner: c.runner}
		return nc.GetNetworkConfigApplicator(networkConfig)
	default:
		return schema.YipConfig{}, fmt.Errorf("using configurator '%s': %w", networkConfig.Configurator, ErrUnknownConfigurator)
	}
}

// ResetNetworkConfig is a common reset procedure that works for all NetworkManager based installations.
//
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
func (c *configurator) ResetNetworkConfig() error {
	// If there are no /etc/NetworkManager/system-connections/*.nmconnection files,
	// then this is the second time we invoke this method, or we never configured any
	// network config so we have nothing to do.
	connectionFiles, err := c.fs.ReadDir(systemConnectionsDir)
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

	// Delete the yip Network Applicator file, so that it's not going to be re-evaluated when rebooting in Recovery for reset
	if err := c.fs.Remove(configApplicator); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting file '%s': %w", configApplicator, err)
	}

	// Delete all .nmconnection files. This will also delete any "static" connection that the user defined in the base image for example.
	// Note that ResetNetworkConfig() is only called when the network config is Elemental driven, hence we not expect any other custom connection defined.
	log.Debug("Deleting all .nmconnection configs")
	if err := c.runner.Run("find", systemConnectionsDir, "-name", "*.nmconnection", "-type", "f", "-delete"); err != nil {
		return fmt.Errorf("deleting all %s/*.nmconnection: %w", systemConnectionsDir, err)
	}

	// We need to invoke nmcli connection reload to tell NetworkManager to reload connections from disk.
	// NetworkManager won't reload them alone with a simple restart.
	log.Debug("Reloading connections")
	if err := c.runner.Run("nmcli", "connection", "reload"); err != nil {
		return fmt.Errorf("running: nmcli connection reload: %w", err)
	}

	// Restart NetworkManager to restart connections.
	log.Debug("Restarting NetworkManager")
	if err := c.runner.Run("systemctl", "restart", "NetworkManager.service"); err != nil {
		return fmt.Errorf("running command: systemctl restart NetworkManager.service: %w", err)
	}

	// Not entirely necessary, but this mitigates the risk of continuing with any potential elemental-system-agent
	// plan confirmation while the network is offline.
	log.Debug("Waiting NetworkManager online")
	if err := c.runner.Run("systemctl", "start", "NetworkManager-wait-online.service"); err != nil {
		return fmt.Errorf("running command: systemctl start NetworkManager-wait-online.service: %w", err)
	}

	// Restarts the elemental-system-agent to start a new connection using the new config.
	// This will make the plan be executed a second time.
	log.Debug("Restarting elemental-system-agent")
	if err := c.runner.Run("systemctl", "restart", "elemental-system-agent.service"); err != nil {
		return fmt.Errorf("running command: systemctl restart elemental-system-agent.service: %w", err)
	}
	return nil
}
