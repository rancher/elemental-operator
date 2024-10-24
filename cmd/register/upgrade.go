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
	"errors"
	"fmt"
	"os"

	"strings"

	"github.com/rancher/elemental-operator/pkg/elementalcli"
	"github.com/rancher/elemental-operator/pkg/install"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/twpayne/go-vfs"

	"k8s.io/utils/exec"
	"k8s.io/utils/nsenter"
)

var (
	ErrRebooting            = errors.New("Machine needs reboot after upgrade")
	ErrAlreadyShuttingDown  = errors.New("System is already shutting down")
	ErrMissingCorrelationID = errors.New("Missing upgrade correlation ID")
)

func newUpgradeCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrades the machine",
		RunE: func(_ *cobra.Command, _ []string) error {
			upgradeConfig := elementalcli.UpgradeConfig{
				Debug:        viper.GetBool("debug"),
				Recovery:     viper.GetBool("recovery"),
				RecoveryOnly: viper.GetBool("recovery-only"),
				System:       viper.GetString("system"),
				Bootloader:   true,
			}
			upgradeContext := install.UpgradeContext{
				Config:          upgradeConfig,
				HostDir:         viper.GetString("host-dir"),
				CloudConfigPath: viper.GetString("cloud-config"),
				CorrelationID:   viper.GetString("correlation-id"),
			}

			if upgradeConfig.Debug {
				log.EnableDebugLogging()
			}

			// For sanity, this needs to be verified or the upgrade process may end up in an infinite loop
			if len(upgradeContext.CorrelationID) == 0 {
				return ErrMissingCorrelationID
			}

			// If the system is shutting down, return an error so we can try again on next reboot.
			alreadyShuttingDown, err := isSystemShuttingDown(upgradeContext.HostDir)
			if err != nil {
				return fmt.Errorf("determining if system is running: %w", err)
			}
			if alreadyShuttingDown {
				return ErrAlreadyShuttingDown
			}

			// If system is not shutting down we can proceed.
			installer := install.NewInstaller(vfs.OSFS, nil, nil)

			needsReboot, err := installer.UpgradeElemental(upgradeContext)
			// If the upgrade could not be applied or verified,
			// then this command will fail but the machine will not reboot.
			if err != nil {
				return fmt.Errorf("upgrading machine: %w", err)
			}
			// If the machine needs a reboot after an upgrade has been applied,
			// so that consumers can try again after reboot to validate the upgrade has been applied successfully.
			if needsReboot {
				log.Infof("Rebooting machine after %s upgrade", upgradeContext.CorrelationID)
				reboot(upgradeContext.HostDir)
				return ErrRebooting
			}
			// Upgrade has been applied successfully, nothing to do.
			log.Infof("Upgrade %s applied successfully", upgradeContext.CorrelationID)
			return nil
		},
	}

	viper.AutomaticEnv()
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetEnvPrefix("ELEMENTAL_REGISTER_UPGRADE")

	cmd.Flags().String("host-dir", "/host", "The machine root directory where to apply the upgrade")
	_ = viper.BindPFlag("host-dir", cmd.Flags().Lookup("host-dir"))

	cmd.Flags().String("cloud-config", "/run/data/cloud-config", "The path of a cloud-config file to install on the machine during upgrade")
	_ = viper.BindPFlag("cloud-config", cmd.Flags().Lookup("cloud-config"))

	cmd.Flags().String("system", "dir:/", "The system image uri or filesystem location to upgrade to")
	_ = viper.BindPFlag("system", cmd.Flags().Lookup("system"))

	cmd.Flags().String("correlation-id", "", "A correlationID to label the upgrade snapshot with")
	_ = viper.BindPFlag("correlation-id", cmd.Flags().Lookup("correlation-id"))

	cmd.Flags().Bool("recovery", false, "Upgrades the recovery partition together with the system")
	_ = viper.BindPFlag("recovery", cmd.Flags().Lookup("recovery"))

	cmd.Flags().Bool("recovery-only", false, "Upgrades the recovery partition only")
	_ = viper.BindPFlag("recovery-only", cmd.Flags().Lookup("recovery-only"))

	cmd.Flags().Bool("debug", true, "Prints debug logs when performing upgrade")
	_ = viper.BindPFlag("debug", cmd.Flags().Lookup("debug"))

	return cmd
}

func isSystemShuttingDown(hostDir string) (bool, error) {
	ex := exec.New()
	nsEnter, err := nsenter.NewNsenter(hostDir, ex)
	if err != nil {
		return false, fmt.Errorf("initializing nsenter: %w", err)
	}
	cmd := nsEnter.Command("systemctl", "is-system-running")
	cmd.SetStdin(os.Stdin)
	cmd.SetStderr(os.Stderr)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("running: systemctl is-system-running: %w", err)
	}
	if string(output) == "stopping" {
		return true, nil
	}
	return false, nil
}

func reboot(hostDir string) {
	ex := exec.New()
	nsEnter, err := nsenter.NewNsenter(hostDir, ex)
	if err != nil {
		log.Errorf("Coult not initialize nsenter: %s", err.Error())
	}
	cmd := nsEnter.Command("reboot")
	cmd.SetStdin(os.Stdin)
	cmd.SetStderr(os.Stderr)
	cmd.SetStdout(os.Stdout)
	if err := cmd.Run(); err != nil {
		log.Errorf("Could not reboot: %s", err)
	}
}
