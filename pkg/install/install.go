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

package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jaypipes/ghw"
	"github.com/jaypipes/ghw/pkg/block"
	"github.com/rancher/yip/pkg/schema"
	"github.com/twpayne/go-vfs"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/controllers"
	systemagent "github.com/rancher/elemental-operator/internal/system-agent"
	"github.com/rancher/elemental-operator/pkg/elementalcli"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/network"
	"github.com/rancher/elemental-operator/pkg/register"
	"github.com/rancher/elemental-operator/pkg/util"
)

const (
	agentStateDir     = "/var/lib/elemental/agent"
	agentConfDir      = "/etc/rancher/elemental/agent"
	registrationConf  = "/oem/registration/config.yaml"
	registrationState = "/oem/registration/state.yaml"
)

// Temporary cloud-init configuration files.
// These paths will be passed to the `elemental` cli as additional `config-urls`.
const (
	tempRegistrationConf  = "/tmp/elemental-registration-conf.yaml"
	tempRegistrationState = "/tmp/elemental-registration-state.yaml"
	tempCloudInit         = "/tmp/elemental-cloud-init.yaml"
	tempSystemAgent       = "/tmp/elemental-system-agent.yaml"
	tempNetworkConfig     = "/tmp/elemental-network-config.yaml"
)

type Installer interface {
	ResetElemental(config elementalv1.Config, state register.State) error
	InstallElemental(config elementalv1.Config, state register.State, networkConfig elementalv1.NetworkConfig) error
	WriteLocalSystemAgentConfig(config elementalv1.Elemental) error
}

func NewInstaller(fs vfs.FS, disks []*block.Disk, networkConfigurator network.Configurator) Installer {
	return &installer{
		fs:                  fs,
		disks:               disks,
		runner:              elementalcli.NewRunner(),
		networkConfigurator: networkConfigurator,
	}
}

var _ Installer = (*installer)(nil)

type installer struct {
	fs                  vfs.FS
	disks               []*block.Disk
	runner              elementalcli.Runner
	networkConfigurator network.Configurator
}

func (i *installer) InstallElemental(config elementalv1.Config, state register.State, networkConfig elementalv1.NetworkConfig) error {
	if config.Elemental.Install.ConfigURLs == nil {
		config.Elemental.Install.ConfigURLs = []string{}
	}

	if config.Elemental.Install.Device == "" {
		deviceName, err := i.findInstallationDevice(config.Elemental.Install.DeviceSelector)
		if err != nil {
			return fmt.Errorf("failed picking installation device: %w", err)
		}

		config.Elemental.Install.Device = deviceName
	} else if len(config.Elemental.Install.DeviceSelector) > 0 {
		log.Warningf("Both device and device-selector set, using device-field '%s'", config.Elemental.Install.Device)
	}

	additionalConfigs, err := i.getCloudInitConfigs(config, state)
	if err != nil {
		return fmt.Errorf("generating additional cloud configs: %w", err)
	}

	config.Elemental.Install.ConfigURLs = append(config.Elemental.Install.ConfigURLs, additionalConfigs...)

	log.Info("Applying network config")
	if err := i.networkConfigurator.ApplyConfig(networkConfig, tempNetworkConfig); err != nil {
		return fmt.Errorf("applying network config: %w", err)
	}
	config.Elemental.Install.ConfigURLs = append(config.Elemental.Install.ConfigURLs, tempNetworkConfig)

	if err := i.runner.Install(config.Elemental.Install); err != nil {
		return fmt.Errorf("failed to install elemental: %w", err)
	}

	log.Info("Elemental install completed, please reboot")
	return nil
}

func (i *installer) ResetElemental(config elementalv1.Config, state register.State) error {
	if config.Elemental.Reset.ConfigURLs == nil {
		config.Elemental.Reset.ConfigURLs = []string{}
	}

	additionalConfigs, err := i.getCloudInitConfigs(config, state)
	if err != nil {
		return fmt.Errorf("generating additional cloud configs: %w", err)
	}
	config.Elemental.Reset.ConfigURLs = append(config.Elemental.Reset.ConfigURLs, additionalConfigs...)

	if err := i.runner.Reset(config.Elemental.Reset); err != nil {
		return fmt.Errorf("failed to reset elemental: %w", err)
	}

	if err := i.cleanupResetPlan(); err != nil {
		return fmt.Errorf("cleaning up reset plan: %w", err)
	}

	log.Info("Elemental reset completed, please reboot")
	return nil
}

func (i *installer) findInstallationDevice(selector elementalv1.DeviceSelector) (string, error) {
	devices := map[string]*ghw.Disk{}

	for _, disk := range i.disks {
		devices[disk.Name] = disk
	}

	for _, disk := range i.disks {
		for _, sel := range selector {
			matches, err := matches(disk, sel)
			if err != nil {
				return "", err
			}

			if !matches {
				log.Debugf("%s does not match selector %s", disk.Name, sel.Key)
				delete(devices, disk.Name)
				break
			}
		}
	}

	log.Debugf("%d disks matching selector", len(devices))

	for _, dev := range devices {
		return fmt.Sprintf("/dev/%s", dev.Name), nil
	}

	return "", fmt.Errorf("no device found matching selector")
}

func matches(disk *block.Disk, req elementalv1.DeviceSelectorRequirement) (bool, error) {
	switch req.Operator {
	case elementalv1.DeviceSelectorOpIn:
		return matchesIn(disk, req)
	case elementalv1.DeviceSelectorOpNotIn:
		return matchesNotIn(disk, req)
	case elementalv1.DeviceSelectorOpLt:
		return matchesLt(disk, req)
	case elementalv1.DeviceSelectorOpGt:
		return matchesGt(disk, req)
	default:
		return false, fmt.Errorf("unknown operator: %s", req.Operator)
	}
}

func matchesIn(disk *block.Disk, req elementalv1.DeviceSelectorRequirement) (bool, error) {
	if req.Key != elementalv1.DeviceSelectorKeyName {
		return false, fmt.Errorf("cannot use In operator on numerical values %s", req.Key)
	}

	for _, val := range req.Values {
		if val == disk.Name || val == fmt.Sprintf("/dev/%s", disk.Name) {
			return true, nil
		}
	}

	return false, nil
}
func matchesNotIn(disk *block.Disk, req elementalv1.DeviceSelectorRequirement) (bool, error) {
	matches, err := matchesIn(disk, req)
	return !matches, err
}
func matchesLt(disk *block.Disk, req elementalv1.DeviceSelectorRequirement) (bool, error) {
	if req.Key != elementalv1.DeviceSelectorKeySize {
		return false, fmt.Errorf("cannot use Lt operator on string values %s", req.Key)

	}

	keySize, err := resource.ParseQuantity(req.Values[0])
	if err != nil {
		return false, fmt.Errorf("failed to parse quantity %s", req.Values[0])
	}

	diskSize := resource.NewQuantity(int64(disk.SizeBytes), resource.BinarySI)

	return diskSize.Cmp(keySize) == -1, nil
}
func matchesGt(disk *block.Disk, req elementalv1.DeviceSelectorRequirement) (bool, error) {
	if req.Key != elementalv1.DeviceSelectorKeySize {
		return false, fmt.Errorf("cannot use Gt operator on string values %s", req.Key)
	}

	keySize, err := resource.ParseQuantity(req.Values[0])
	if err != nil {
		return false, fmt.Errorf("failed to parse quantity %s", req.Values[0])
	}

	diskSize := resource.NewQuantity(int64(disk.SizeBytes), resource.BinarySI)

	return diskSize.Cmp(keySize) == 1, nil
}

// getCloudInitConfigs creates cloud-init configuration files that can be passed as additional `config-urls`
// to the `elemental` cli. We exploit this mechanism to persist information during `elemental install`
// or `elemental reset` calls into the newly installed or resetted system.
func (i *installer) getCloudInitConfigs(config elementalv1.Config, state register.State) ([]string, error) {
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

	registrationStatePath, err := i.writeRegistrationState(state)
	if err != nil {
		return nil, fmt.Errorf("writing registration state plan: %w", err)
	}
	configs = append(configs, registrationStatePath)

	return configs, nil
}

func (i *installer) writeRegistrationYAML(reg elementalv1.Registration) (string, error) {
	f, err := i.fs.Create(tempRegistrationConf)
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

	if err := yaml.NewEncoder(f).Encode(schema.YipConfig{
		Name: "Include registration config into installed system",
		Stages: map[string][]schema.Stage{
			"initramfs": {
				schema.Stage{
					If: fmt.Sprintf("[ ! -f %s ]", registrationConf),
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
	}); err != nil {
		return "", fmt.Errorf("writing encoded registration config config: %w", err)
	}
	return f.Name(), nil
}

func (i *installer) writeRegistrationState(state register.State) (string, error) {
	f, err := i.fs.Create(tempRegistrationState)
	if err != nil {
		return "", fmt.Errorf("creating temporary registration state plan file: %w", err)
	}
	defer f.Close()
	stateBytes, err := yaml.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("marshalling registration state: %w", err)
	}

	if err := yaml.NewEncoder(f).Encode(schema.YipConfig{
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
	}); err != nil {
		return "", fmt.Errorf("writing encoded registration state config: %w", err)
	}
	return f.Name(), nil
}

func (i *installer) writeCloudInit(cloudConfig map[string]runtime.RawExtension) (string, error) {
	f, err := i.fs.Create(tempCloudInit)
	if err != nil {
		return "", fmt.Errorf("creating temporary cloud init file: %w", err)
	}
	defer f.Close()

	bytes, err := util.MarshalCloudConfig(cloudConfig)
	if err != nil {
		return "", fmt.Errorf("mashalling cloud config: %w", err)
	}

	log.Debugf("Decoded CloudConfig:\n%s\n", string(bytes))
	if _, err = f.Write(bytes); err != nil {
		return "", fmt.Errorf("writing cloud config: %w", err)
	}
	return f.Name(), nil
}

func (i *installer) getConnectionInfoBytes(config elementalv1.Elemental) ([]byte, error) {
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

	connectionInfo := systemagent.ConnectionInfo{
		KubeConfig: string(kubeconfigBytes),
		Namespace:  config.SystemAgent.SecretNamespace,
		SecretName: config.SystemAgent.SecretName,
	}

	connectionInfoBytes, err := json.Marshal(connectionInfo)
	if err != nil {
		return nil, fmt.Errorf("encoding connectionInfo: %w", err)
	}
	return connectionInfoBytes, nil
}

func (i *installer) getAgentConfigBytes() ([]byte, error) {
	agentConfig := systemagent.AgentConfig{
		WorkDir:            filepath.Join(agentStateDir, "work"),
		AppliedPlanDir:     filepath.Join(agentStateDir, "applied"),
		LocalPlanDir:       filepath.Join(agentStateDir, "plans"),
		RemoteEnabled:      true,
		LocalEnabled:       false,
		ConnectionInfoFile: filepath.Join(agentStateDir, "elemental_connection.json"),
		PreserveWorkDir:    false,
	}

	agentConfigBytes, err := yaml.Marshal(agentConfig)
	if err != nil {
		return nil, fmt.Errorf("encoding agentConfig: %w", err)
	}
	return agentConfigBytes, nil
}

// Write system agent config files to local filesystem
func (i *installer) WriteLocalSystemAgentConfig(config elementalv1.Elemental) error {
	connectionInfoBytes, err := i.getConnectionInfoBytes(config)
	if err != nil {
		return fmt.Errorf("getting connection info: %w", err)
	}
	if _, err := i.fs.Stat(agentStateDir); os.IsNotExist(err) {
		log.Debugf("agent state dir '%s' does not exist. Creating now.", agentStateDir)
		if err := vfs.MkdirAll(i.fs, agentStateDir, 0700); err != nil {
			return fmt.Errorf("creating agent state directory: %w", err)
		}
	}
	connectionInfoFile := filepath.Join(agentStateDir, "elemental_connection.json")
	err = os.WriteFile(connectionInfoFile, connectionInfoBytes, 0600)
	if err != nil {
		return fmt.Errorf("writing connection info file: %w", err)
	}
	log.Infof("connection info file '%s' written.", connectionInfoFile)

	agentConfigBytes, err := i.getAgentConfigBytes()
	if err != nil {
		return fmt.Errorf("getting agent config: %w", err)
	}
	if _, err := i.fs.Stat(agentConfDir); os.IsNotExist(err) {
		log.Debugf("agent config dir '%s' does not exist. Creating now.", agentConfDir)
		if err := vfs.MkdirAll(i.fs, agentConfDir, 0700); err != nil {
			return fmt.Errorf("creating agent config directory: %w", err)
		}
	}
	agentConfigFile := filepath.Join(agentConfDir, "config.yaml")
	err = os.WriteFile(agentConfigFile, agentConfigBytes, 0600)
	if err != nil {
		return fmt.Errorf("writing agent config file: %w", err)
	}
	log.Infof("agent config file '%s' written.", agentConfigFile)
	return nil
}

// Write system agent cloud-init config to be consumed by install
func (i *installer) writeSystemAgentConfig(config elementalv1.Elemental) (string, error) {
	connectionInfoBytes, err := i.getConnectionInfoBytes(config)
	if err != nil {
		return "", fmt.Errorf("getting connection info: %w", err)
	}
	agentConfigBytes, err := i.getAgentConfigBytes()
	if err != nil {
		return "", fmt.Errorf("getting agent config: %w", err)
	}

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

	f, err := i.fs.Create(tempSystemAgent)
	if err != nil {
		return "", fmt.Errorf("creating temporary elemental-system-agent file: %w", err)
	}
	defer f.Close()

	if err := yaml.NewEncoder(f).Encode(schema.YipConfig{
		Name: "Elemental System Agent Configuration",
		Stages: map[string][]schema.Stage{
			"initramfs": stages,
		},
	}); err != nil {
		return "", fmt.Errorf("writing encoded system agent config: %w", err)
	}
	return f.Name(), nil
}

func (i *installer) cleanupResetPlan() error {
	_, err := i.fs.Stat(controllers.LocalResetPlanPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("checking presence of file '%s': %w", controllers.LocalResetPlanPath, err)
	}
	if os.IsNotExist(err) {
		log.Debugf("local reset plan '%s' does not exist, nothing to do", controllers.LocalResetPlanPath)
		return nil
	}
	return i.fs.Remove(controllers.LocalResetPlanPath)
}
