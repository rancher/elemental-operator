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
	"reflect"

	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/twpayne/go-vfs"

	"github.com/rancher/elemental-operator/pkg/elementalcli"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/upgrade"
	"github.com/rancher/elemental-operator/pkg/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	ErrRebooting            = errors.New("Machine needs reboot after upgrade")
	ErrAlreadyShuttingDown  = errors.New("System is already shutting down")
	ErrMissingCorrelationID = errors.New("Missing upgrade correlation ID")
)

var decodeHook = viper.DecodeHook(
	mapstructure.ComposeDecodeHookFunc(
		KeyValuePairHook(),
	),
)

func KeyValuePairHook() mapstructure.DecodeHookFuncType {
	return func(from reflect.Type, to reflect.Type, data interface{}) (interface{}, error) {
		if from.Kind() != reflect.String {
			return data, nil
		}

		if to != reflect.TypeOf(elementalcli.KeyValuePair{}) {
			return data, nil
		}

		return elementalcli.KeyValuePairFromData(data)
	}
}

func newUpgradeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrades the machine",
		RunE: func(_ *cobra.Command, _ []string) error {
			upgradeConfig := elementalcli.UpgradeConfig{Bootloader: true}
			if err := viper.Unmarshal(&upgradeConfig, decodeHook); err != nil {
				return fmt.Errorf("unmarshalling config: %w", err)
			}
			environment := upgrade.Environment{
				Config: upgradeConfig,
			}
			if err := viper.Unmarshal(&environment); err != nil {
				return fmt.Errorf("unmarshalling context: %w", err)
			}

			if upgradeConfig.Debug {
				log.EnableDebugLogging()
			}

			nsEnter := util.NewNsEnter()
			upgrader := upgrade.NewUpgrader(vfs.OSFS)
			return upgradeElemental(nsEnter, upgrader, environment)
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

	cmd.Flags().StringToString("snapshot-labels", map[string]string{}, "Labels to apply to the upgrade snapshot")
	_ = viper.BindPFlag("snapshot-labels", cmd.Flags().Lookup("snapshot-labels"))

	return cmd
}

func upgradeElemental(nsEnter util.NsEnter, upgrader upgrade.Upgrader, environment upgrade.Environment) error {
	// For sanity, this needs to be verified or the upgrade process may end up in an infinite loop
	if len(environment.Config.CorrelationID) == 0 {
		return ErrMissingCorrelationID
	}

	// If the system is shutting down, return an error so we can try again on next reboot.
	alreadyShuttingDown, err := nsEnter.IsSystemShuttingDown(environment.HostDir)
	if err != nil {
		return fmt.Errorf("determining if system is running: %w", err)
	}
	if alreadyShuttingDown {
		return ErrAlreadyShuttingDown
	}

	// If system is not shutting down we can proceed.
	needsReboot, err := upgrader.UpgradeElemental(environment)
	// If the upgrade could not be applied or verified,
	// then this command will fail but the machine will not reboot.
	if err != nil {
		return fmt.Errorf("upgrading machine: %w", err)
	}
	// If the machine needs a reboot after an upgrade has been applied,
	// so that consumers can try again after reboot to validate the upgrade has been applied successfully.
	if needsReboot {
		log.Infof("Rebooting machine after %s upgrade", environment.Config.CorrelationID)
		nsEnter.Reboot(environment.HostDir)
		return ErrRebooting
	}
	// Upgrade has been applied successfully, nothing to do.
	log.Infof("Upgrade %s applied successfully", environment.Config.CorrelationID)
	return nil
}
