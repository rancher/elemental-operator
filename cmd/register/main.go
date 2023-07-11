/*
Copyright © 2022 - 2023 SUSE LLC

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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mudler/yip/pkg/schema"
	agent "github.com/rancher/system-agent/pkg/config"
	"github.com/sanity-io/litter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/elementalcli"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/register"
	"github.com/rancher/elemental-operator/pkg/util"
	"github.com/rancher/elemental-operator/pkg/version"
)

const (
	stateInstallFile = "/run/initramfs/cos-state/state.yaml"
	agentStateDir    = "/var/lib/elemental/agent"
	agentConfDir     = "/etc/rancher/elemental/agent"
	afterInstallHook = "/oem/install-hook.yaml"

	// Registration config directories, depending if system is live or not
	regConfExt      = "yaml"
	regConfDir      = "/oem/registration"
	regConfName     = "config"
	liveRegConfDir  = "/run/initramfs/live"
	liveRegConfName = "livecd-cloud-config"

	// This file stores the registration URL and certificate used for the registration
	// this file will be stored into the install system by an after-install hook
	registrationConf = "/run/cos/oem/registration/config.yaml"
)

func main() {
	var cfg elementalv1.Config
	var debug bool

	cmd := &cobra.Command{
		Use:   "elemental-register",
		Short: "Elemental register command",
		Long:  "elemental-register registers a node with the elemental-operator via a config file or flags",
		Run: func(_ *cobra.Command, args []string) {
			if debug {
				log.EnableDebugLogging()
			}
			if viper.GetBool("version") {
				log.Infof("Support version %s, commit %s, commit date %s", version.Version, version.Commit, version.CommitDate)
				return
			}

			log.Infof("Register version %s, commit %s, commit date %s", version.Version, version.Commit, version.CommitDate)

			// Locate config directory and file
			var configDir string
			var configName string
			if len(args) == 0 {
				if !isSystemInstalled() {
					configDir = liveRegConfDir
					configName = liveRegConfName
				} else {
					configDir = regConfDir
					configName = regConfName
				}
			} else {
				configDir = args[0] //Take the first argument only, ignore the rest
				configName = regConfName
			}

			// Merge configuration from file
			if err := mergeConfigFromFile(configDir, configName); err != nil {
				log.Fatalf("Could not read configuration in directory '%s': %s", configDir, err)
			}

			if err := viper.Unmarshal(&cfg); err != nil {
				log.Fatalf("failed to parse configuration: ", err)
			}

			log.Debugf("input config:\n%s", litter.Sdump(cfg))

			run(configDir, cfg)
		},
	}

	// Registration
	cmd.Flags().StringVar(&cfg.Elemental.Registration.URL, "registration-url", "", "Registration url to get the machine config from")
	cmd.Flags().StringVar(&cfg.Elemental.Registration.CACert, "registration-ca-cert", "", "File with the custom CA certificate to use against he registration url")
	cmd.Flags().BoolVar(&cfg.Elemental.Registration.EmulateTPM, "emulate-tpm", false, "Emulate /dev/tpm")
	cmd.Flags().Int64Var(&cfg.Elemental.Registration.EmulatedTPMSeed, "emulated-tpm-seed", 1, "Seed for /dev/tpm emulation")
	cmd.Flags().BoolVar(&cfg.Elemental.Registration.NoSMBIOS, "no-smbios", false, "Disable the use of dmidecode to get SMBIOS")
	cmd.Flags().StringVar(&cfg.Elemental.Registration.Auth, "auth", "tpm", "Registration authentication method")
	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")
	cmd.PersistentFlags().BoolP("version", "v", false, "print version and exit")
	_ = viper.BindPFlag("version", cmd.PersistentFlags().Lookup("version"))

	if err := cmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

func mergeConfigFromFile(path string, name string) error {
	log.Debugf("Using configuration in directory: %s\n", path)
	viper.AddConfigPath(path)
	viper.SetConfigName(name)
	viper.SetConfigType(regConfExt)
	return viper.MergeInConfig()
}

func run(configDir string, config elementalv1.Config) {
	// Validate Registration config
	registration := config.Elemental.Registration

	if registration.URL == "" {
		log.Fatal("Registration URL is empty")
	}

	caCert, err := getRegistrationCA(registration)
	if err != nil {
		log.Fatalf("Could not load registration CA certificate: %s", err)
	}

	client := register.NewClient(configDir)

	data, err := client.Register(registration, caCert)
	if err != nil {
		log.Fatalf("failed to register machine inventory: %w", err)
	}

	log.Debugf("Fetched configuration from manager cluster:\n%s\n\n", string(data))

	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Errorf("failed to parse registration configuration: %w", err)
	}

	if !isSystemInstalled() {
		if err := installElemental(config); err != nil {
			log.Fatalf("elemental installation failed: %w", err)
		}

		log.Info("elemental installation completed, please reboot")
	}
}

func getRegistrationCA(registration elementalv1.Registration) ([]byte, error) {
	/* Here we can have a file path or the cert data itself */
	if _, err := os.Stat(registration.CACert); err == nil {
		log.Info("CACert passed as a file")
		return os.ReadFile(registration.CACert)
	}
	if registration.CACert == "" {
		log.Warning("CACert is empty")
	}
	return []byte(registration.CACert), nil
}

func installElemental(config elementalv1.Config) error {
	cloudInitURLs := config.Elemental.Install.ConfigURLs
	if cloudInitURLs == nil {
		cloudInitURLs = []string{}
	}

	agentConfPath, err := writeSystemAgentConfig(config.Elemental)
	if err != nil {
		return fmt.Errorf("failed to write system agent configuration: %w", err)
	}
	cloudInitURLs = append(cloudInitURLs, agentConfPath)

	if len(config.CloudConfig) > 0 {
		cloudInitPath, err := writeCloudInit(config.CloudConfig)
		if err != nil {
			return fmt.Errorf("failed to write custom cloud-init file: %w", err)
		}
		cloudInitURLs = append(cloudInitURLs, cloudInitPath)
	}

	config.Elemental.Install.ConfigURLs = cloudInitURLs

	if err := installRegistrationYAML(config.Elemental.Registration); err != nil {
		return fmt.Errorf("failed to prepare after-install hook: %w", err)
	}

	installDataMap, err := structToMap(config.Elemental.Install)
	if err != nil {
		return fmt.Errorf("failed to decode elemental-cli install data: %w", err)
	}

	if err := elementalcli.Run(installDataMap); err != nil {
		return fmt.Errorf("failed to install elemental: %w", err)
	}

	return nil
}

func structToMap(str interface{}) (map[string]interface{}, error) {
	var mapStruct map[string]interface{}

	data, err := json.Marshal(str)
	if err == nil {
		if err := json.Unmarshal(data, &mapStruct); err == nil {
			return mapStruct, nil
		}
	}

	return nil, err
}

// isSystemInstalled checks if the host is currently installed
// TODO: make the function dependent on tmp.Register returned data
func isSystemInstalled() bool {
	_, err := os.Stat(stateInstallFile)
	return err == nil
}

func installRegistrationYAML(reg elementalv1.Registration) error {
	registrationInBytes, err := yaml.Marshal(elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: reg,
		},
	})
	if err != nil {
		return err
	}
	f, err := os.Create(afterInstallHook)
	if err != nil {
		return err
	}
	defer f.Close()

	err = yaml.NewEncoder(f).Encode(schema.YipConfig{
		Name: "Include registration config into installed system",
		Stages: map[string][]schema.Stage{
			"after-install": {
				schema.Stage{
					Directories: []schema.Directory{
						{
							Path:        filepath.Dir(registrationConf),
							Permissions: 0700,
						},
					}, Files: []schema.File{
						{
							Path:        registrationConf,
							Content:     string(registrationInBytes),
							Permissions: 0600,
						},
					},
				},
			},
		},
	})

	return err
}

func writeCloudInit(cloudConfig map[string]runtime.RawExtension) (string, error) {
	f, err := os.CreateTemp(os.TempDir(), "*.yaml")
	if err != nil {
		return "", err
	}
	defer f.Close()

	bytes, err := util.MarshalCloudConfig(cloudConfig)
	if err != nil {
		return "", err
	}

	log.Debugf("Decoded CloudConfig:\n%s\n", string(bytes))
	_, err = f.Write(bytes)
	return f.Name(), err
}

func writeSystemAgentConfig(config elementalv1.Elemental) (string, error) {
	kubeConfig := api.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters: map[string]*api.Cluster{
			"cluster": {
				Server:                   config.SystemAgent.URL,
				CertificateAuthorityData: []byte(config.Registration.CACert),
			}},
		AuthInfos: map[string]*api.AuthInfo{
			"user": {
				Token: config.SystemAgent.Token,
			}},
		Contexts: map[string]*api.Context{
			"context": {
				Cluster:  "cluster",
				AuthInfo: "user",
			}},
		CurrentContext: "context",
	}

	kubeconfigBytes, _ := clientcmd.Write(kubeConfig)

	connectionInfo := agent.ConnectionInfo{
		KubeConfig: string(kubeconfigBytes),
		Namespace:  config.SystemAgent.SecretNamespace,
		SecretName: config.SystemAgent.SecretName,
	}

	agentConfig := agent.AgentConfig{
		WorkDir:            filepath.Join(agentStateDir, "work"),
		AppliedPlanDir:     filepath.Join(agentStateDir, "applied"),
		LocalPlanDir:       filepath.Join(agentStateDir, "plans"),
		RemoteEnabled:      true,
		LocalEnabled:       true,
		ConnectionInfoFile: filepath.Join(agentStateDir, "elemental_connection.json"),
		PreserveWorkDir:    false,
	}

	connectionInfoBytes, _ := json.Marshal(connectionInfo)
	agentConfigBytes, _ := json.Marshal(agentConfig)

	var stages []schema.Stage

	stages = append(stages, schema.Stage{
		Files: []schema.File{
			{
				Path:        filepath.Join(agentStateDir, "elemental_connection.json"),
				Content:     string(connectionInfoBytes),
				Permissions: 0600,
			},
			{
				Path:        filepath.Join(agentConfDir, "config.yaml"),
				Content:     string(agentConfigBytes),
				Permissions: 0600,
			},
		},
	})

	f, err := os.CreateTemp(os.TempDir(), "*.yaml")
	if err != nil {
		return "", err
	}
	defer f.Close()

	err = yaml.NewEncoder(f).Encode(schema.YipConfig{
		Name: "Elemental System Agent Configuration",
		Stages: map[string][]schema.Stage{
			"initramfs": stages,
		},
	})

	return f.Name(), err
}
