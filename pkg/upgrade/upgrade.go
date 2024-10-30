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

package upgrade

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rancher/elemental-operator/pkg/elementalcli"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/util"
	"github.com/twpayne/go-vfs"
)

const (
	upgradeCloudConfigPath = "/oem/90_operator.yaml"
	correlationIDLabelKey  = "correlationID"
	upgradeRunDir          = "/run/elemental"
)

var (
	upgradeMounts = []string{"/dev", "/run"}
)

type Environment struct {
	Config          elementalcli.UpgradeConfig
	HostDir         string `mapstructure:"host-dir,omitempty"`
	CloudConfigPath string `mapstructure:"cloud-config,omitempty"`
}

type Upgrader interface {
	UpgradeElemental(environment Environment) (bool, error)
}

func NewUpgrader(fs vfs.FS) Upgrader {
	return &upgrader{
		fs:     fs,
		runner: elementalcli.NewRunner(),
		cmd:    util.NewCommandRunner(),
	}
}

var _ Upgrader = (*upgrader)(nil)

type upgrader struct {
	fs     vfs.FS
	runner elementalcli.Runner
	cmd    util.CommandRunner
}

func (u *upgrader) UpgradeElemental(environment Environment) (bool, error) {
	log.Infof("Applying upgrade: %s", environment.Config.CorrelationID)

	runDir := filepath.Join(environment.HostDir, upgradeRunDir)
	if err := vfs.MkdirAll(u.fs, runDir, os.ModePerm); err != nil {
		return false, fmt.Errorf("creating directory '%s': %w", runDir, err)
	}

	if err := u.mountDirs(upgradeMounts, environment.HostDir); err != nil {
		return false, fmt.Errorf("mounting host directories: %w", err)
	}
	elementalState, err := u.runner.GetState()
	if err != nil {
		return false, fmt.Errorf("reading installation state: %w", err)
	}

	if u.isCorrelationIDFound(elementalState, environment.Config.CorrelationID) {
		log.Infof("Upgrade '%s' successfully applied", environment.Config.CorrelationID)
		return false, nil
	}

	if err := u.applyCloudConfig(environment.HostDir, environment.CloudConfigPath); err != nil {
		return false, fmt.Errorf("applying upgrade cloud config: %w", err)
	}

	// Prepare Snapshot labels
	if environment.Config.SnapshotLabels == nil {
		environment.Config.SnapshotLabels = map[string]string{}
	}
	environment.Config.SnapshotLabels[correlationIDLabelKey] = environment.Config.CorrelationID

	log.Infof("Applying upgrade %s", environment.Config.CorrelationID)
	if err := u.runner.Upgrade(environment.Config); err != nil {
		return false, fmt.Errorf("applying upgrade '%s': %w", environment.Config.CorrelationID, err)
	}
	return true, nil
}

func (u *upgrader) applyCloudConfig(hostDir string, cloudConfigPath string) error {
	hostCloudConfigPath := filepath.Join(hostDir, upgradeCloudConfigPath)

	cloudConfigBytes, err := u.fs.ReadFile(cloudConfigPath)
	if os.IsNotExist(err) {
		log.Infof("Upgrade cloud config '%s' is missing. Removing previously applied config in '%s', if any.", cloudConfigPath, hostCloudConfigPath)
		if err := u.fs.Remove(hostCloudConfigPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing file '%s': %w", hostCloudConfigPath, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading file '%s': %w", cloudConfigPath, err)
	}

	hostCloudConfigBytes, err := u.fs.ReadFile(hostCloudConfigPath)
	if os.IsNotExist(err) || err == nil {
		if !bytes.Equal(hostCloudConfigBytes, cloudConfigBytes) {
			log.Infof("Applying upgrade cloud config to: %s", hostCloudConfigPath)
			if err := u.fs.WriteFile(hostCloudConfigPath, cloudConfigBytes, os.ModePerm); err != nil {
				return fmt.Errorf("writing file '%s': %w", hostCloudConfigPath, err)
			}
		}
	} else {
		return fmt.Errorf("reading file '%s': %w", hostCloudConfigPath, err)
	}

	return nil
}

func (u *upgrader) isCorrelationIDFound(elementalState elementalcli.State, correlationID string) bool {
	// This is normally not supposed to happen, as we expect at least the first snapshot to be present after install.
	// However we can still try to upgrade in this case, hoping the upgrade snapshot will be created after that.
	if elementalState.StatePartition.Snapshots == nil {
		log.Info("Could not find correlationID in empty snapshots list")
		return false
	}

	correlationIDFound := false
	correlationIDFoundInActiveSnapshot := false
	for _, snapshot := range elementalState.StatePartition.Snapshots {
		if snapshot.Labels[correlationIDLabelKey] == correlationID {
			correlationIDFound = true
			correlationIDFoundInActiveSnapshot = snapshot.Active
			break
		}
	}

	// If the upgrade was already applied, but somehow the system was reverted to a different snapshot,
	// do not apply the upgrade again. This will prevent a cascade loop effect, for example when the
	// revert is automatically applied by the boot assessment mechanism.
	if correlationIDFound && !correlationIDFoundInActiveSnapshot {
		log.Infof("CorrelationID %s found on a passive snapshot. Not upgrading again.", correlationID)
		return true
	}

	// Found on the active snapshot. All good, nothing to do.
	if correlationIDFound && correlationIDFoundInActiveSnapshot {
		return true
	}

	log.Infof("Could not find snapshot with correlationID %s", correlationID)
	return false
}

func (u *upgrader) mountDirs(mounts []string, hostDir string) error {
	if hostDir == "/" {
		return nil
	}

	for _, mount := range mounts {
		hostMount := filepath.Join(hostDir, mount)
		args := []string{"--rbind", hostMount, mount}
		log.Debugf("running: mount %s", strings.Join(args, " "))
		if err := u.cmd.Run("mount", args...); err != nil {
			return fmt.Errorf("mounting '%s': %w", hostMount, err)
		}
	}

	return nil
}
