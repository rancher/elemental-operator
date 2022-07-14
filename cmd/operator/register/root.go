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
	"github.com/rancher/elemental-operator/pkg/install"
	"github.com/rancher/elemental-operator/pkg/tpm"
	agent "github.com/rancher/system-agent/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
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
			if len(args) == 0 {
				args = append(args, "/oem")
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

		if yaml.Unmarshal(data, &config) != nil {
			logrus.Error("failed to parse registration configuration: ", err)
			time.Sleep(time.Second * 5)
			continue
		}

		break
	}

	err = writeCloudInit(data)
	if err != nil {
		logrus.Fatal("failed to write cloud init: ", err)
	}

	cloudInitPath, err := writeYIPConfig(config.Elemental)
	if err != nil {
		logrus.Fatal("failed to write yip config: ", err)
	}

	err = writeElementalConfig(config.Elemental, cloudInitPath)
	if err != nil {
		logrus.Fatal("failed to write elemental config: ", err)
	}

}

func writeCloudInit(data []byte) error {
	f, err := os.OpenFile("/oem/userdata", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	if err != nil {
		return err
	}

	if _, err = f.Write(data); err != nil {
		return err
	}

	return nil
}

func writeYIPConfig(config cfg.Elemental) (string, error) {
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
		WorkDir:            "/var/lib/elemental/agent/work",
		AppliedPlanDir:     "/var/lib/elemental/agent/applied",
		RemoteEnabled:      true,
		ConnectionInfoFile: "/var/lib/elemental/agent/elemental_connection.json",
		PreserveWorkDir:    false,
	}

	connectionInfoBytes, _ := json.Marshal(connectionInfo)
	agentConfigBytes, _ := json.Marshal(agentConfig)

	var stages []schema.Stage

	stages = append(stages, schema.Stage{
		Files: []schema.File{
			{
				Path:        "/var/lib/elemental/agent/elemental_connection.json",
				Content:     string(connectionInfoBytes),
				Permissions: 0600,
			},
			{
				Path:        "/etc/rancher/elemental/agent/config.yaml",
				Content:     string(agentConfigBytes),
				Permissions: 0600,
			},
		},
	})

	if config.Install.Password != "" {
		stages = append(stages, schema.Stage{
			Users: map[string]schema.User{
				"root": {
					Name:         "root",
					PasswordHash: config.Install.Password,
				},
			},
		})
	}

	if len(config.Install.SSHKeys) > 0 {
		stages = append(stages, schema.Stage{
			Users: map[string]schema.User{
				"root": {
					Name:              "root",
					SSHAuthorizedKeys: config.Install.SSHKeys,
					Homedir:           "/root",
				},
			},
		})
	}

	f, err := os.CreateTemp(os.TempDir(), "*.yip")
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	if err != nil {
		return "", err
	}

	err = yaml.NewEncoder(f).Encode(schema.YipConfig{
		Name: "Elemental System Agent Configuration",
		Stages: map[string][]schema.Stage{
			"initramfs": stages,
		},
	})

	return f.Name(), err
}

func writeElementalConfig(conf cfg.Elemental, cloudInitPath string) error {

	conf.Install.ConfigURL = cloudInitPath
	fullConf := cfg.Config{Elemental: conf}
	ev, err := cfg.ToEnv(fullConf)
	if err != nil {
		return err
	}

	install.PrintEnv(fullConf)

	installerOpts := []string{"elemental", "install"}

	cmd := exec.Command("elemental")
	cmd.Env = append(os.Environ(), ev...)
	cmd.Stdout = os.Stdout
	cmd.Args = installerOpts
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
