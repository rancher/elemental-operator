/*
Copyright © 2022 - 2025 SUSE LLC

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
)

type nmstateConfigurator struct {
	fs     vfs.FS
	runner util.CommandRunner
}

func (n *nmstateConfigurator) GetNetworkConfigApplicator(networkConfig elementalv1.NetworkConfig) (schema.YipConfig, error) {
	configApplicator := schema.YipConfig{}

	if len(networkConfig.Config) == 0 {
		log.Warning("no network config data to decode")
		return configApplicator, ErrEmptyConfig
	}

	yamlData, err := util.JSONObjectToYamlBytes(networkConfig.Config)
	if err != nil {
		return configApplicator, fmt.Errorf("converting network config to yaml: %w", err)
	}

	yamlStringData := string(yamlData)

	// Go through the nmstate yaml config and replace template placeholders "{my-ip-name}" with actual IP.
	for name, ipAddress := range networkConfig.IPAddresses {
		yamlStringData = strings.ReplaceAll(yamlStringData, fmt.Sprintf("{%s}", name), ipAddress)
	}

	// Dump the digested config somewhere
	if err := vfs.MkdirAll(n.fs, filepath.Dir(nmstateTempPath), 0700); err != nil {
		return configApplicator, fmt.Errorf("creating dir '%s': %w", filepath.Dir(nmstateTempPath), err)
	}
	if err := n.fs.WriteFile(nmstateTempPath, []byte(yamlStringData), 0600); err != nil {
		return configApplicator, fmt.Errorf("writing file '%s': %w", nmstateTempPath, err)
	}

	// Try to apply it
	if err := n.runner.Run("nmstatectl", "apply", nmstateTempPath); err != nil {
		return configApplicator, fmt.Errorf("running: nmstatectl apply %s: %w", nmstateTempPath, err)
	}

	// Now fetch all /etc/NetworkManager/system-connections/*.nmconnection files.
	// Each file is added to the configApplicator.
	files, err := n.fs.ReadDir(systemConnectionsDir)
	if err != nil {
		return configApplicator, fmt.Errorf("reading directory '%s': %w", systemConnectionsDir, err)
	}

	yipFiles := []schema.File{}
	for _, file := range files {
		fileName := file.Name()
		if strings.HasSuffix(fileName, ".nmconnection") {
			bytes, err := n.fs.ReadFile(filepath.Join(systemConnectionsDir, fileName))
			if err != nil {
				return configApplicator, fmt.Errorf("reading file '%s': %w", fileName, err)
			}
			yipFiles = append(yipFiles, schema.File{
				Path:        filepath.Join(systemConnectionsDir, fileName),
				Permissions: 0600,
				Content:     string(bytes),
			})
		}
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
