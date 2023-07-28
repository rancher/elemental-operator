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

package install

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/mudler/yip/pkg/schema"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/elementalcli"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/register"
	"github.com/rancher/elemental-operator/pkg/util"
	agent "github.com/rancher/system-agent/pkg/config"
	"github.com/twpayne/go-vfs"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	stateInstallFile  = "/run/initramfs/cos-state/state.yaml"
	agentStateDir     = "/var/lib/elemental/agent"
	agentConfDir      = "/etc/rancher/elemental/agent"
	registrationConf  = "/oem/registration/config.yaml"
	registrationState = "/oem/registration/state.yaml"
)

type Installer interface {
	ResetElemental(config elementalv1.Config) error
	InstallElemental(config elementalv1.Config, state register.State) error
}

func NewInstaller(fs vfs.FS) Installer {
	return &installer{
		fs:     fs,
		runner: elementalcli.NewRunner(),
	}
}

var _ Installer = (*installer)(nil)

type installer struct {
	fs     vfs.FS
	runner elementalcli.Runner
}

func (i *installer) InstallElemental(config elementalv1.Config, state register.State) error {
	if config.Elemental.Install.ConfigURLs == nil {
		config.Elemental.Install.ConfigURLs = []string{}
	}

	additionalConfigs, err := i.getCloudInitConfigs(config)
	if err != nil {
		return fmt.Errorf("generating additional cloud configs: %w", err)
	}
	registrationStatePath, err := i.writeRegistrationState(state)

	if err != nil {
		return fmt.Errorf("writing registration state plan: %w", err)
	}
	additionalConfigs = append(additionalConfigs, registrationStatePath)

	config.Elemental.Install.ConfigURLs = append(config.Elemental.Install.ConfigURLs, additionalConfigs...)

	if err := i.runner.Install(config.Elemental.Install); err != nil {
		return fmt.Errorf("failed to install elemental: %w", err)
	}

	log.Info("Elemental install completed, please reboot")
	return nil
}

func (i *installer) ResetElemental(config elementalv1.Config) error {
	if config.Elemental.Reset.ConfigURLs == nil {
		config.Elemental.Reset.ConfigURLs = []string{}
	}

	additionalConfigs, err := i.getCloudInitConfigs(config)
	if err != nil {
		return fmt.Errorf("generating additional cloud configs: %w", err)
	}
	config.Elemental.Reset.ConfigURLs = append(config.Elemental.Reset.ConfigURLs, additionalConfigs...)

	if err := i.runner.Reset(config.Elemental.Reset); err != nil {
		return fmt.Errorf("failed to reset elemental: %w", err)
	}

	log.Info("Elemental reset completed, please reboot")
	return nil
}

func (i *installer) getCloudInitConfigs(config elementalv1.Config) ([]string, error) {
	configs := []string{}
	agentConfPath, err := i.writeSystemAgentConfig(config.Elemental)
	if err != nil {
		return nil, fmt.Errorf("writing system agent configuration: %w", err)
	}
	configs = append(configs, agentConfPath)

	if len(config.CloudConfig) > 0 {
		cloudInitPath, err := i.writeCloudInit(config.CloudConfig)
		if err != nil {
			return nil, fmt.Errorf("writing custom cloud-init file: %w", err)
		}
		configs = append(configs, cloudInitPath)
	}

	registrationConfPath, err := i.writeRegistrationYAML(config.Elemental.Registration)
	if err != nil {
		return nil, fmt.Errorf("writing registration conf plan: %w", err)
	}
	configs = append(configs, registrationConfPath)

	return configs, nil
}

func (i *installer) writeRegistrationYAML(reg elementalv1.Registration) (string, error) {
	f, err := i.fs.Create("/tmp/elemental-registration-conf.yaml")
	if err != nil {
		return "", fmt.Errorf("creating temporary registration conf plan file: %w", err)
	}
	defer f.Close()
	registrationInBytes, err := yaml.Marshal(elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: reg,
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshalling registration config: %w", err)
	}

	err = yaml.NewEncoder(f).Encode(schema.YipConfig{
		Name: "Include registration config into installed system",
		Stages: map[string][]schema.Stage{
			"initramfs": {
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

	return f.Name(), err
}

func (i *installer) writeRegistrationState(state register.State) (string, error) {
	f, err := i.fs.Create("/tmp/elemental-registration-state.yaml")
	if err != nil {
		return "", fmt.Errorf("creating temporary registration state plan file: %w", err)
	}
	defer f.Close()
	stateBytes, err := yaml.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("marshalling registration state: %w", err)
	}

	err = yaml.NewEncoder(f).Encode(schema.YipConfig{
		Name: "Include registration state into installed system",
		Stages: map[string][]schema.Stage{
			"initramfs": {
				schema.Stage{
					If: fmt.Sprintf("[ ! -f %s ]", registrationState),
					Directories: []schema.Directory{
						{
							Path:        filepath.Dir(registrationState),
							Permissions: 0700,
						},
					}, Files: []schema.File{
						{
							Path:        registrationState,
							Content:     string(stateBytes),
							Permissions: 0600,
						},
					},
				},
			},
		},
	})
	return f.Name(), err
}

func (i *installer) writeCloudInit(cloudConfig map[string]runtime.RawExtension) (string, error) {
	f, err := i.fs.Create("/tmp/elemental-cloud-init.yaml")
	if err != nil {
		return "", fmt.Errorf("creating temporary cloud init file: %w", err)
	}
	defer f.Close()

	bytes, err := util.MarshalCloudConfig(cloudConfig)
	if err != nil {
		return "", fmt.Errorf("mashalling cloud config: %w", err)
	}

	log.Debugf("Decoded CloudConfig:\n%s\n", string(bytes))
	_, err = f.Write(bytes)
	return f.Name(), err
}

func (i *installer) writeSystemAgentConfig(config elementalv1.Elemental) (string, error) {
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

	f, err := i.fs.Create("/tmp/elemental-system-agent.yaml")
	if err != nil {
		return "", fmt.Errorf("creating temporary elemental-system-agent file: %w", err)
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
