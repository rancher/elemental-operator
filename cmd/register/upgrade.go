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

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/rancher/elemental-operator/pkg/elementalcli"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	ErrRebooting           = errors.New("Machine needs reboot after upgrade")
	ErrTimedOut            = errors.New("Upgrade timed out")
	ErrAlreadyShuttingDown = errors.New("System is already shutting down")
	mounts                 = []string{"/dev", "/run"}
)

const (
	lockPath               = "/run/elemental/upgrade.lock"
	lockTimeout            = 10 * time.Minute
	upgradeCloudConfigPath = "/oem/90_operator.yaml"
	correlationIDLabelKey  = "correlationID"
)

func newUpgradeCommand() *cobra.Command {
	var hostDir string
	var cloudConfigPath string
	var recovery bool
	var recoveryOnly bool
	var force bool
	var debug bool
	var system string
	var correlationID string

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrades the machine",
		RunE: func(_ *cobra.Command, _ []string) error {
			upgradeConfig := elementalcli.UpgradeConfig{
				Debug:        debug,
				Recovery:     recovery,
				RecoveryOnly: recoveryOnly,
				System:       system,
				Bootloader:   true,
			}

			needsReboot, err := upgrade(upgradeConfig, hostDir, cloudConfigPath, correlationID, force)
			// If the upgrade could not be applied or verified,
			// then this command will fail but the machine will not reboot.
			if err != nil {
				return fmt.Errorf("upgrading machine: %w", err)
			}
			// If the machine needs a reboot after an upgrade has been applied,
			// so that consumers can try again after reboot to validate the upgrade has been applied successfully.
			if needsReboot {
				reboot()
				return ErrRebooting
			}
			// Upgrade has been applied successfully, nothing to do.
			return nil
		},
	}

	viper.AutomaticEnv()
	cmd.Flags().StringVar(&hostDir, "host-dir", "/host", "The machine root directory where to apply the upgrade")
	cmd.Flags().StringVar(&cloudConfigPath, "cloud-config", "/run/data/cloud-config", "The path of a cloud-config file to install on the machine during upgrade")
	cmd.Flags().StringVar(&system, "system", "dir:/", "The system image uri or filesystem location to upgrade to")
	cmd.Flags().StringVar(&correlationID, "correlation-id", "", "A correlationID to label the upgrade snapshot with")
	cmd.Flags().BoolVar(&recovery, "recovery", false, "Upgrades the recovery partition together with the system")
	cmd.Flags().BoolVar(&recoveryOnly, "recovery-only", false, "Upgrades the recovery partition only")
	cmd.Flags().BoolVar(&force, "force", false, "Force the application of the upgrade, even with an already installed correlation-id")
	cmd.Flags().BoolVar(&debug, "debug", true, "Prints debug logs when performing upgrade")
	return cmd
}

func upgrade(config elementalcli.UpgradeConfig, hostDir string, cloudConfigPath string, correlationID string, force bool) (bool, error) {
	log.Infof("Applying upgrade: %s", correlationID)

	hostLockPath := filepath.Join(hostDir, lockPath)
	runner := elementalcli.NewRunner()

	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()

	fileLock := flock.New(hostLockPath)

	for {
		time.Sleep(1 * time.Second)
		select {
		case <-ctx.Done():
			log.Errorf("Upgrade timed out")
			return false, ErrTimedOut
		default:
			lockAcquired, err := fileLock.TryLock()
			if err != nil {
				return false, fmt.Errorf("trying to lock file '%s': %w", hostLockPath, err)
			}

			if lockAcquired {
				defer unlock(fileLock, hostLockPath)

				shuttingDown, err := isSystemShuttingDown()
				if err != nil {
					return false, fmt.Errorf("determining if system is shutting down: %w", err)
				}
				if shuttingDown {
					return false, ErrAlreadyShuttingDown
				}

				if err := applyCloudConfig(hostDir, cloudConfigPath); err != nil {
					return false, fmt.Errorf("applying upgrade cloud config: %w", err)
				}

				elementalState, err := runner.GetState()
				if err != nil {
					return false, fmt.Errorf("reading installation state: %w", err)
				}

				if !isCorrelationIDFound(elementalState, correlationID) || force {
					log.Infof("Applying upgrade %s", correlationID)
					if err := mountDirs(mounts, hostDir); err != nil {
						return false, fmt.Errorf("mounting host directories: %w", err)
					}
					if err := runner.Upgrade(config); err != nil {
						return false, fmt.Errorf("applying upgrade '%s': %w", correlationID, err)
					}
				} else {
					log.Infof("Upgrade '%s' successfully applied", correlationID)
					return false, nil
				}
			}
		}
	}
}

func unlock(fileLock *flock.Flock, lockPath string) {
	if err := fileLock.Unlock(); err != nil {
		log.Errorf("Cloud not unlock file '%s': %s", lockPath, err.Error())
	}
}

func applyCloudConfig(hostDir string, cloudConfigPath string) error {
	hostCloudConfigPath := filepath.Join(hostDir, upgradeCloudConfigPath)

	cloudConfigBytes, err := os.ReadFile(cloudConfigPath)
	if os.IsNotExist(err) {
		log.Infof("Upgrade cloud config '%s' is missing. Removing previously applied config in '%s', if any.", cloudConfigPath, hostCloudConfigPath)
		if err := os.Remove(hostCloudConfigPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing file '%s': %w", hostCloudConfigPath, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading file '%s': %w", cloudConfigPath, err)
	}

	hostCloudConfigBytes, err := os.ReadFile(hostCloudConfigPath)
	if os.IsNotExist(err) || err == nil {
		if !bytes.Equal(hostCloudConfigBytes, cloudConfigBytes) {
			log.Infof("Applying upgrade cloud config to: %s", hostCloudConfigPath)
			if err := os.WriteFile(hostCloudConfigPath, cloudConfigBytes, os.ModePerm); err != nil {
				return fmt.Errorf("writing file '%s': %w", hostCloudConfigPath, err)
			}
		}
	} else {
		return fmt.Errorf("reading file '%s': %w", hostCloudConfigPath, err)
	}

	return nil
}

func isCorrelationIDFound(elementalState elementalcli.State, correlationID string) bool {
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

func isSystemShuttingDown() (bool, error) {
	cmd := exec.Command("nsenter")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Args = []string{"-i", "-m", "-t", "1", "--", "systemctl is-system-running"}
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("running: systemctl is-system-running: %w", err)
	}
	if string(output) == "stopping" {
		return true, nil
	}
	return false, nil
}

func reboot() {
	cmd := exec.Command("nsenter")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Args = []string{"-i", "-m", "-t", "1", "--", "reboot"}
	if err := cmd.Run(); err != nil {
		log.Errorf("Could not reboot: %s", err)
	}
}

func mountDirs(mounts []string, hostDir string) error {
	if hostDir == "/" {
		return nil
	}

	for _, mount := range mounts {
		hostMount := filepath.Join(hostDir, mount)
		cmd := exec.Command("mount")
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr
		cmd.Args = []string{"--rbind", hostMount, mount}
		log.Debugf("running: mount %s", strings.Join(cmd.Args, " "))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("mounting '%s': %w", hostMount, err)
		}
	}

	return nil
}
