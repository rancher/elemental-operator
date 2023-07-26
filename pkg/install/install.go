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
	"os"
	"path/filepath"

	"github.com/mudler/yip/pkg/schema"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/elementalcli"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/util"
	agent "github.com/rancher/system-agent/pkg/config"
	"github.com/twpayne/go-vfs"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	stateInstallFile = "/run/initramfs/cos-state/state.yaml"
	agentStateDir    = "/var/lib/elemental/agent"
	agentConfDir     = "/etc/rancher/elemental/agent"
	registrationConf = "/run/cos/oem/registration/config.yaml"
	oemDir           = "/oem"
	oemDirLive       = "/run/cos/oem"
)

type Installer interface {
	IsSystemInstalled() bool
	InstallElemental(config elementalv1.Config) error
	UpdateCloudConfig(config elementalv1.Config) error
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

func (i *installer) UpdateCloudConfig(config elementalv1.Config) error {
	var baseDir string
	if i.IsSystemInstalled() {
		baseDir = oemDir
	} else {
		baseDir = oemDirLive
	}

	registrationConfigDir := fmt.Sprintf("%s/registration", baseDir)
	if _, err := i.fs.Stat(registrationConfigDir); os.IsNotExist(err) {
		log.Debugf("Registration config dir '%s' does not exist. Creating now.", registrationConfigDir)
		if err := vfs.MkdirAll(i.fs, registrationConfigDir, 0700); err != nil {
			return fmt.Errorf("creating registration config directory: %w", err)
		}
	}

	if err := i.writeRegistrationConfig(config.Elemental.Registration, registrationConfigDir); err != nil {
		return fmt.Errorf("writing registration config: %w", err)
	}

	if err := i.writeSystemAgentConfigPlan(config.Elemental, baseDir); err != nil {
		return fmt.Errorf("writing system agent config plan: %w", err)
	}

	if err := i.writeCloudInitConfig(config.CloudConfig, baseDir); err != nil {
		return fmt.Errorf("writing cloud config: %w", err)
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

func (i *installer) writeRegistrationConfig(reg elementalv1.Registration, configDir string) error {
	registrationInBytes, err := yaml.Marshal(elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: reg,
		},
	})
	if err != nil {
		return fmt.Errorf("marshalling elemental configuration: %w", err)
	}
	configPath := fmt.Sprintf("%s/config.yaml", configDir)
	if err := i.fs.WriteFile(configPath, registrationInBytes, os.FileMode(0600)); err != nil {
		return fmt.Errorf("writing file '%s': %w", configPath, err)
	}
	return nil
}

func (i *installer) writeCloudInitConfig(cloudConfig map[string]runtime.RawExtension, configDir string) error {
	cloudConfigBytes, err := util.MarshalCloudConfig(cloudConfig)
	if err != nil {
		return fmt.Errorf("mashalling cloud config: %w", err)
	}

	log.Debugf("Decoded CloudConfig:\n%s\n", string(cloudConfigBytes))

	cloudConfigPath := fmt.Sprintf("%s/cloud-config.yaml", configDir)
	if err := i.fs.WriteFile(cloudConfigPath, cloudConfigBytes, os.FileMode(0600)); err != nil {
		return fmt.Errorf("writing file '%s': %w", cloudConfigPath, err)
	}
	return nil
}

func (i *installer) writeSystemAgentConfigPlan(config elementalv1.Elemental, configDir string) error {
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

	planBytes, err := yaml.Marshal(schema.YipConfig{
		Name: "Elemental System Agent Configuration",
		Stages: map[string][]schema.Stage{
			"initramfs": stages,
		},
	})
	if err != nil {
		return fmt.Errorf("marshalling elemental system agent config plan: %w", err)
	}

	planPath := fmt.Sprintf("%s/elemental-agent-config-plan.yaml", configDir)
	if err := i.fs.WriteFile(planPath, planBytes, os.FileMode(0600)); err != nil {
		return fmt.Errorf("writing file '%s': %w", planPath, err)
	}
	return nil
}
