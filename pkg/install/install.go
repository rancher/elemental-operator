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
	"errors"
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

const (
	afterInstallStage = "after-install"
	afterResetStage   = "after-reset"
	// Used in after-{install|reset} stages
	elementalAfterHook = "elemental-after-hook.yaml"

	// Deterministic Elemental yip paths
	registrationConfigPath = "elemental-registration.yaml"
	stateConfigPath        = "elemental-state.yaml"
	cloudInitConfigPath    = "elemental-cloud-init.yaml"
	systemAgentConfigPath  = "elemental-system-agent.yaml"
	networkConfigPath      = "elemental-network.yaml"

	// OEM is mounted on different paths depending if we are resetting (from recovery) or installing (from live media)
	installOEMMount = "/run/elemental/oem"
	resetOEMMount   = "/oem"
)

type UpgradeContext struct {
	Config          elementalcli.UpgradeConfig
	HostDir         string `mapstructure:"host-dir,omitempty"`
	CloudConfigPath string `mapstructure:"cloud-config,omitempty"`
}

type Installer interface {
	ResetElemental(config elementalv1.Config, state register.State, networkConfig elementalv1.NetworkConfig) error
	ResetElementalNetwork() error
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

	if err := i.writeAfterHookConfigurator(afterInstallStage, installOEMMount, config, state, networkConfig); err != nil {
		return fmt.Errorf("writing %s configurator: %w", afterInstallStage, err)
	}

	if err := i.runner.Install(config.Elemental.Install); err != nil {
		return fmt.Errorf("failed to install elemental: %w", err)
	}

	log.Info("Elemental install completed, please reboot")
	return nil
}

func (i *installer) ResetElemental(config elementalv1.Config, state register.State, networkConfig elementalv1.NetworkConfig) error {
	if config.Elemental.Reset.ConfigURLs == nil {
		config.Elemental.Reset.ConfigURLs = []string{}
	}

	if err := i.writeAfterHookConfigurator(afterResetStage, resetOEMMount, config, state, networkConfig); err != nil {
		return fmt.Errorf("writing %s configurator: %w", afterResetStage, err)
	}

	if err := i.runner.Reset(config.Elemental.Reset); err != nil {
		return fmt.Errorf("failed to reset elemental: %w", err)
	}

	if err := i.cleanupResetPlan(); err != nil {
		return fmt.Errorf("cleaning up reset plan: %w", err)
	}

	log.Info("Elemental reset completed, please reboot")
	return nil
}

func (i *installer) ResetElementalNetwork() error {
	if err := i.networkConfigurator.ResetNetworkConfig(); err != nil {
		return fmt.Errorf("resetting network config: %w", err)
	}
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

func (i *installer) writeAfterHookConfigurator(stage string, oemMount string, config elementalv1.Config, state register.State, networkConfig elementalv1.NetworkConfig) error {
	afterHookConfigurator := schema.YipConfig{}
	afterHookConfigurator.Name = "Elemental Finalize System"
	afterHookConfigurator.Stages = map[string][]schema.Stage{
		stage: {},
	}

	// Registration
	registrationYipBytes, err := i.registrationConfigYip(config.Elemental.Registration)
	if err != nil {
		return fmt.Errorf("getting registration config yip: %w", err)
	}
	registrationYip := yipAfterHookWrap(string(registrationYipBytes), filepath.Join(oemMount, registrationConfigPath), "Registration Config")
	afterHookConfigurator.Stages[stage] = append(afterHookConfigurator.Stages[stage], registrationYip)

	// State
	stateYipBytes, err := i.registrationStateYip(state)
	if err != nil {
		return fmt.Errorf("getting registration state config yip: %w", err)
	}
	stateYip := yipAfterHookWrap(string(stateYipBytes), filepath.Join(oemMount, stateConfigPath), "Registration State Config")
	afterHookConfigurator.Stages[stage] = append(afterHookConfigurator.Stages[stage], stateYip)

	// Cloud Init
	cloudInitYipBytes, err := i.cloudInitYip(config.CloudConfig)
	if err != nil {
		return fmt.Errorf("getting cloud-init config yip: %w", err)
	}
	cloudInitYip := yipAfterHookWrap(string(cloudInitYipBytes), filepath.Join(oemMount, cloudInitConfigPath), "Cloud Init Config")
	afterHookConfigurator.Stages[stage] = append(afterHookConfigurator.Stages[stage], cloudInitYip)

	// Elemental System Agent
	systemAgentYipBytes, err := i.elementalSystemAgentYip(config.Elemental)
	if err != nil {
		return fmt.Errorf("getting elemental system agent config yip: %w", err)
	}
	systemAgentYip := yipAfterHookWrap(string(systemAgentYipBytes), filepath.Join(oemMount, systemAgentConfigPath), "Elemental System Agent Config")
	afterHookConfigurator.Stages[stage] = append(afterHookConfigurator.Stages[stage], systemAgentYip)

	// Network Config
	networkConfigYipBytes, err := i.networkConfigYip(networkConfig)
	if err != nil && !errors.Is(err, network.ErrEmptyConfig) {
		return fmt.Errorf("getting network config yip: %w", err)
	}
	if !errors.Is(err, network.ErrEmptyConfig) {
		networkConfigYip := yipAfterHookWrap(string(networkConfigYipBytes), filepath.Join(oemMount, networkConfigPath), "Network Config")
		afterHookConfigurator.Stages[stage] = append(afterHookConfigurator.Stages[stage], networkConfigYip)
	}

	// Create dir if not exist
	if err := vfs.MkdirAll(i.fs, elementalcli.TempCloudInitDir, 0700); err != nil {
		return fmt.Errorf("creating directory '%s': %w", elementalcli.TempCloudInitDir, err)
	}
	// Create the after hook configurator yip
	f, err := i.fs.Create(filepath.Join(elementalcli.TempCloudInitDir, elementalAfterHook))
	if err != nil {
		return fmt.Errorf("creating file '%s': %w", filepath.Join(elementalcli.TempCloudInitDir, elementalAfterHook), err)
	}
	defer f.Close()

	if err := yaml.NewEncoder(f).Encode(afterHookConfigurator); err != nil {
		return fmt.Errorf("writing encoded after hook configurator: %w", err)
	}

	return nil
}

func yipAfterHookWrap(content string, path string, name string) schema.Stage {
	config := schema.Stage{Name: name}
	config.Files = []schema.File{
		{
			Path:        path,
			Content:     content,
			Permissions: 0600,
		},
	}
	return config
}

func (i *installer) registrationConfigYip(reg elementalv1.Registration) ([]byte, error) {
	registrationInBytes, err := yaml.Marshal(elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: reg,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling registration config: %w", err)
	}

	yipConfig := schema.YipConfig{
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
	}

	yipConfigBytes, err := yaml.Marshal(yipConfig)
	if err != nil {
		return nil, fmt.Errorf("marshalling yip registration config: %w", err)
	}

	return yipConfigBytes, nil
}

func (i *installer) registrationStateYip(state register.State) ([]byte, error) {
	stateBytes, err := yaml.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshalling registration state: %w", err)
	}

	yipConfig := schema.YipConfig{
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
	}

	yipConfigBytes, err := yaml.Marshal(yipConfig)
	if err != nil {
		return nil, fmt.Errorf("marshalling yip state config: %w", err)
	}

	return yipConfigBytes, nil
}

func (i *installer) cloudInitYip(cloudConfig map[string]runtime.RawExtension) ([]byte, error) {
	bytes, err := util.MarshalCloudConfig(cloudConfig)
	if err != nil {
		return nil, fmt.Errorf("mashalling cloud config: %w", err)
	}

	return bytes, nil
}

func (i *installer) networkConfigYip(networkConfig elementalv1.NetworkConfig) ([]byte, error) {
	networkYipConfig, err := i.networkConfigurator.GetNetworkConfigApplicator(networkConfig)
	if err != nil {
		return nil, fmt.Errorf("getting network config applicator: %w", err)
	}

	yipConfigBytes, err := yaml.Marshal(networkYipConfig)
	if err != nil {
		return nil, fmt.Errorf("marshalling yip network config: %w", err)
	}

	return yipConfigBytes, nil
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

func (i *installer) getAgentConfigEnvs(config elementalv1.Elemental) []byte {
	return []byte(fmt.Sprintf("CATTLE_AGENT_STRICT_VERIFY=\"%t\"", config.SystemAgent.StrictTLSMode))
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

	elementalAgentEnvsFile := filepath.Join(agentConfDir, "envs")
	err = os.WriteFile(elementalAgentEnvsFile, i.getAgentConfigEnvs(config), 0600)
	if err != nil {
		return fmt.Errorf("writing agent envs file: %w", err)
	}
	log.Infof("agent envs file '%s' written.", elementalAgentEnvsFile)

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

func (i *installer) elementalSystemAgentYip(config elementalv1.Elemental) ([]byte, error) {
	connectionInfoBytes, err := i.getConnectionInfoBytes(config)
	if err != nil {
		return nil, fmt.Errorf("getting connection info: %w", err)
	}
	agentConfigBytes, err := i.getAgentConfigBytes()
	if err != nil {
		return nil, fmt.Errorf("getting agent config: %w", err)
	}
	agentEnvsBytes := i.getAgentConfigEnvs(config)

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
			{
				Path:        filepath.Join(agentConfDir, "envs"),
				Content:     string(agentEnvsBytes),
				Permissions: 0600,
			},
		},
	})

	yipConfig := schema.YipConfig{
		Name: "Elemental System Agent Configuration",
		Stages: map[string][]schema.Stage{
			"initramfs": stages,
		},
	}

	yipConfigBytes, err := yaml.Marshal(yipConfig)
	if err != nil {
		return nil, fmt.Errorf("marshalling yip system agent config: %w", err)
	}

	return yipConfigBytes, nil
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
