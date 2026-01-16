/*
Copyright © 2022 - 2026 SUSE LLC

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

package network

import (
	"fmt"
	"path/filepath"
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/util"
	"github.com/rancher/yip/pkg/schema"
	"github.com/twpayne/go-vfs"
	k8syaml "sigs.k8s.io/yaml"
)

type networkManagerConfigurator struct {
	fs     vfs.FS
	runner util.CommandRunner
}

func (n *networkManagerConfigurator) GetNetworkConfigApplicator(networkConfig elementalv1.NetworkConfig) (schema.YipConfig, error) {
	configApplicator := schema.YipConfig{}

	if len(networkConfig.Config) == 0 {
		log.Warning("no network config data to decode")
		return configApplicator, ErrEmptyConfig
	}

	yamlData, err := util.JSONObjectToYamlBytes(networkConfig.Config)
	if err != nil {
		return configApplicator, fmt.Errorf("converting network config to yaml: %w", err)
	}

	// The YAML we implicitly expect is:
	//
	// config:
	//   eth0: |
	//	   [connection]
	//	   id=Wired connection 1
	//     [ipv4]
	//	   address1={my-ip-1}
	//   eth1: |
	//	   [connection]
	//	   id=Wired connection 2
	//     [ipv4]
	//	   address1={my-ip-2}
	connections := map[string]string{}
	if err := k8syaml.Unmarshal(yamlData, &connections); err != nil {
		return configApplicator, fmt.Errorf("unmarshalling connections wrapper: %w", err)
	}

	// Replace the "{my-ip-name}" placeholders with real IPAddresses
	for connectionName := range connections {
		for ipName, ipAddress := range networkConfig.IPAddresses {
			connections[connectionName] = strings.ReplaceAll(connections[connectionName], fmt.Sprintf("{%s}", ipName), ipAddress)
		}
	}

	// Add files to the yip config
	yipFiles := []schema.File{}
	for connectionName, connectionConfig := range connections {
		yipFiles = append(yipFiles, schema.File{
			Path:        filepath.Join(systemConnectionsDir, fmt.Sprintf("%s.nmconnection", connectionName)),
			Permissions: 0600,
			Content:     connectionConfig,
		})
	}

	// Wrap up the yip config
	configApplicator.Name = applicatorName
	configApplicator.Stages = map[string][]schema.Stage{
		applicatorStage: {
			schema.Stage{
				Files: yipFiles,
			},
		},
	}

	return configApplicator, nil
}
