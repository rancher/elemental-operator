/*
Copyright © 2022 SUSE LLC

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

package registerCmd

import (
	"encoding/json"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mudler/yip/pkg/schema"
	cfg "github.com/rancher/elemental-operator/pkg/config"
	"github.com/rancher/elemental-operator/pkg/tpm"
	"github.com/rancher/elemental-operator/pkg/version"
	agent "github.com/rancher/system-agent/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	stateInstallFile = "/run/initramfs/cos-state/state.yaml"
	agentStateDir    = "/var/lib/elemental/agent"
	agentConfDir     = "/etc/rancher/elemental/agent"
	afterInstallHook = "/oem/install-hook.yaml"
	regConfDir       = "/oem/registration"

	// This file stores the registration URL and certificate used for the registration
	// this file will be stored into the install system by an after-install hook
	registrationConf = "/run/cos/oem/registration/config.yaml"
)

func NewRegisterCommand() *cobra.Command {
	var config cfg.Config
	var labels []string
	var debug bool

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register the node",
		Run: func(_ *cobra.Command, args []string) {
			if debug {
				logrus.SetLevel(logrus.DebugLevel)
			}
			logrus.Infof("Operator version %s, commit %s, commit date %s", version.Version, version.Commit, version.CommitDate)
			if len(args) == 0 {
				args = append(args, regConfDir)
			}

			for _, arg := range args {
				viper.AddConfigPath(arg)
				_ = filepath.WalkDir(arg, func(path string, d fs.DirEntry, err error) error {
					if !d.IsDir() && filepath.Ext(d.Name()) == ".yaml" {
						viper.SetConfigType("yaml")
						viper.SetConfigName(d.Name())
						if err := viper.MergeInConfig(); err != nil {
							logrus.Fatalf("failed to read config %s: %s", path, err)
						}
					}
					return nil
				})
			}

			if err := viper.Unmarshal(&config); err != nil {
				logrus.Fatal("failed to parse configuration: ", err)
			}

			if config.Elemental.Registration.Labels == nil {
				config.Elemental.Registration.Labels = map[string]string{}
			}

			for _, label := range labels {
				parts := strings.Split(label, "=")
				if len(parts) == 2 {
					config.Elemental.Registration.Labels[parts[0]] = parts[1]
				}
			}

			run(config)
		},
	}

	// Registration
	cmd.Flags().StringVar(&config.Elemental.Registration.URL, "registration-url", "", "Registration url to get the machine config from")
	cmd.Flags().StringVar(&config.Elemental.Registration.CACert, "registration-ca-cert", "", "File with the custom CA certificate to use against he registration url")
	cmd.Flags().BoolVar(&config.Elemental.Registration.EmulateTPM, "emulate-tpm", false, "Emulate /dev/tpm")
	cmd.Flags().Int64Var(&config.Elemental.Registration.EmulatedTPMSeed, "emulated-tpm-seed", 1, "Seed for /dev/tpm emulation")
	cmd.Flags().BoolVar(&config.Elemental.Registration.NoSMBIOS, "no-smbios", false, "Disable the use of dmidecode to get SMBIOS")
	cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "")

	return cmd
}

func run(config cfg.Config) {
	registration := config.Elemental.Registration

	if registration.URL == "" {
		return
	}

	var err error
	var data, caCert []byte

	/* Here we can have a file path or the cert data itself */
	_, err = os.Stat(registration.CACert)
	if err == nil {
		logrus.Debug("CACert passed as a file")
		caCert, err = ioutil.ReadFile(registration.CACert)
		if err != nil {
			logrus.Error(err)
		}
	} else {
		caCert = []byte(registration.CACert)
	}

	for {
		data, err = tpm.Register(registration.URL, caCert, !registration.NoSMBIOS, registration.EmulateTPM, registration.EmulatedTPMSeed, registration.Labels)
		if err != nil {
			logrus.Error("failed to register machine inventory: ", err)
			time.Sleep(time.Second * 5)
			continue
		}

		logrus.Debugf("Fetched configuration from manager cluster:\n%s\n\n", string(data))

		if yaml.Unmarshal(data, &config) != nil {
			logrus.Error("failed to parse registration configuration: ", err)
			time.Sleep(time.Second * 5)
			continue
		}

		break
	}

	if !isSystemInstalled() {
		cloudInitURLs := config.Elemental.Install.ConfigURLs
		if cloudInitURLs == nil {
			cloudInitURLs = []string{}
		}

		agentConfPath, err := writeSystemAgentConfig(config.Elemental)
		if err != nil {
			logrus.Fatal("failed to write system agent configuration: ", err)
		}
		cloudInitURLs = append(cloudInitURLs, agentConfPath)

		if len(config.CloudConfig) > 0 {
			cloudInitPath, err := writeCloudInit(config.CloudConfig)
			if err != nil {
				logrus.Fatal("failed to write custom cloud-init file: ", err)
			}
			cloudInitURLs = append(cloudInitURLs, cloudInitPath)
		}

		config.Elemental.Install.ConfigURLs = cloudInitURLs

		err = installRegistrationYAML(config.Elemental.Registration)
		if err != nil {
			logrus.Fatal("failed preparing after-install hook")
		}

		err = callElementalClient(config.Elemental)
		if err != nil {
			logrus.Fatal("failed calling elemental client: ", err)
		}
	}
}

// isSystemInstalled checks if the host is currently installed
// TODO: make the function dependent on tmp.Register returned data
func isSystemInstalled() bool {
	_, err := os.Stat(stateInstallFile)
	return err == nil
}

func installRegistrationYAML(reg cfg.Registration) error {
	registrationInBytes, err := yaml.Marshal(cfg.Config{
		Elemental: cfg.Elemental{
			Registration: cfg.Registration{
				URL:    reg.URL,
				CACert: reg.CACert,
			},
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

func writeCloudInit(data map[string]interface{}) (string, error) {
	f, err := os.CreateTemp(os.TempDir(), "*.yaml")
	if err != nil {
		return "", err
	}
	defer f.Close()

	bytes, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	bytes = append([]byte("#cloud-config\n"), bytes...)

	_, err = f.Write(bytes)
	return f.Name(), err
}

func writeSystemAgentConfig(config cfg.Elemental) (string, error) {
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

func callElementalClient(conf cfg.Elemental) error {
	ev, err := cfg.ToEnv(conf.Install)
	if err != nil {
		return err
	}

	installerOpts := []string{"elemental"}
	if conf.Install.Debug {
		installerOpts = append(installerOpts, "--debug")
	}
	installerOpts = append(installerOpts, "install")

	cmd := exec.Command("elemental")
	cmd.Env = append(os.Environ(), ev...)
	cmd.Stdout = os.Stdout
	cmd.Args = installerOpts
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
