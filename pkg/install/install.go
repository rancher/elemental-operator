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
	"github.com/rancher/elemental-operator/pkg/util"
	agent "github.com/rancher/system-agent/pkg/config"
	"github.com/twpayne/go-vfs"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	stateInstallFile = "/run/initramfs/cos-state/state.yaml"
	agentStateDir    = "/var/lib/elemental/agent"
	agentConfDir     = "/etc/rancher/elemental/agent"
	afterInstallHook = "/oem/install-hook.yaml"
	registrationConf = "/run/cos/oem/registration/config.yaml"
)

type Installer interface {
	IsSystemInstalled() bool
	InstallElemental(config elementalv1.Config) error
	UpdateSystemAgentConfig(config elementalv1.Elemental) error
}

func NewInstaller(fs vfs.FS) Installer {
	return &installer{
		fs: fs,
	}
}

var _ Installer = (*installer)(nil)

type installer struct {
	fs vfs.FS
}

// IsSystemInstalled checks if the host is currently installed
// TODO: make the function dependent on tmp.Register returned data
func (i *installer) IsSystemInstalled() bool {
	_, err := i.fs.Stat(stateInstallFile)
	return err == nil
}

func (i *installer) InstallElemental(config elementalv1.Config) error {
	cloudInitURLs := config.Elemental.Install.ConfigURLs
	if cloudInitURLs == nil {
		cloudInitURLs = []string{}
	}

	agentConfPath, err := i.writeSystemAgentConfig(config.Elemental)
	if err != nil {
		return fmt.Errorf("failed to write system agent configuration: %w", err)
	}
	cloudInitURLs = append(cloudInitURLs, agentConfPath)

	if len(config.CloudConfig) > 0 {
		cloudInitPath, err := i.writeCloudInit(config.CloudConfig)
		if err != nil {
			return fmt.Errorf("failed to write custom cloud-init file: %w", err)
		}
		cloudInitURLs = append(cloudInitURLs, cloudInitPath)
	}

	config.Elemental.Install.ConfigURLs = cloudInitURLs

	if err := i.installRegistrationYAML(config.Elemental.Registration); err != nil {
		return fmt.Errorf("failed to prepare after-install hook: %w", err)
	}

	installDataMap, err := structToMap(config.Elemental.Install)
	if err != nil {
		return fmt.Errorf("failed to decode elemental-cli install data: %w", err)
	}

	if err := elementalcli.Run(installDataMap); err != nil {
		return fmt.Errorf("failed to install elemental: %w", err)
	}

	log.Info("Elemental installation completed, please reboot")
	return nil
}

func (i *installer) UpdateSystemAgentConfig(config elementalv1.Elemental) error {
	agentConfPath, err := i.writeSystemAgentConfig(config)
	if err != nil {
		return fmt.Errorf("failed to write system agent configuration: %w", err)
	}
	config.Install.ConfigURLs = []string{agentConfPath}
	installDataMap, err := structToMap(config.Install)
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

func (i *installer) installRegistrationYAML(reg elementalv1.Registration) error {
	registrationInBytes, err := yaml.Marshal(elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: reg,
		},
	})
	if err != nil {
		return err
	}
	f, err := i.fs.Create(afterInstallHook)
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
