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

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drone/envsubst/v2"
	"sigs.k8s.io/yaml"
)

type E2EConfig struct {
	Chart            string `yaml:"chart"`
	CRDsChart        string `yaml:"crdsChart"`
	ExternalIP       string `yaml:"externalIP"`
	MagicDNS         string `yaml:"magicDNS"`
	BridgeIP         string `yaml:"bridgeIP"`
	OperatorReplicas string `yaml:"operatorReplicas"`
	NoSetup          bool   `yaml:"noSetup"`
	ArtifactsDir     string `yaml:"artifactsDir"`

	NginxVersion string `yaml:"nginxVersion"`
	NginxURL     string `yaml:"nginxURL"`

	CertManagerVersion  string `yaml:"certManagerVersion"`
	CertManagerChartURL string `yaml:"certManagerChartURL"`

	RancherVersion  string `yaml:"rancherVersion"`
	RancherChartURL string `yaml:"rancherChartURL"`

	SystemUpgradeControllerVersion string `yaml:"systemUpgradeControllerVersion"`
	SystemUpgradeControllerURL     string `yaml:"systemUpgradeControllerURL"`
}

// ReadE2EConfig read config from yaml and substitute variables using envsubst.
// All variables can be overridden by environmental variables.
func ReadE2EConfig(configPath string) (*E2EConfig, error) { //nolint:gocyclo
	config := &E2EConfig{}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if configData == nil {
		return nil, errors.New("config file can't be empty")
	}

	if err := yaml.Unmarshal(configData, config); err != nil {
		return nil, fmt.Errorf("failed to unmarhal config file: %s", err)
	}

	if chart := os.Getenv("CHART"); chart != "" {
		config.Chart = chart
		dir, chartFile := filepath.Split(chart)
		chartFile = strings.Replace(chartFile, "elemental-operator", "elemental-operator-crds", 1)
		config.CRDsChart = filepath.Join(dir, chartFile)
	}

	if config.Chart == "" {
		return nil, errors.New("no CHART provided, a elemental operator helm chart is required to run e2e tests")
	}

	if externalIP := os.Getenv("EXTERNAL_IP"); externalIP != "" {
		config.ExternalIP = externalIP
	}

	if config.ExternalIP == "" {
		return nil, errors.New("no EXTERNAL_IP provided, a known (reachable) node external ip it is required to run e2e tests")
	}

	if magicDNS := os.Getenv("MAGIC_DNS"); magicDNS != "" {
		config.MagicDNS = magicDNS
	}

	if bridgeIP := os.Getenv("BRIDGE_IP"); bridgeIP != "" {
		config.BridgeIP = bridgeIP
	}

	if replicas := os.Getenv("OPERATOR_REPLICAS"); replicas != "" {
		config.OperatorReplicas = replicas
	}

	if noSetup := os.Getenv("NO_SETUP"); noSetup != "" {
		config.NoSetup = true
	}

	if artifactsDir := os.Getenv("ARTIFACTS_DIR"); artifactsDir != "" {
		config.ArtifactsDir = artifactsDir
	}

	if nginxVersion := os.Getenv("NGINX_VERSION"); nginxVersion != "" {
		config.NginxVersion = nginxVersion
	}

	if nginxURL := os.Getenv("NGINX_URL"); nginxURL != "" {
		config.NginxURL = nginxURL
	}

	if certManagerVersion := os.Getenv("CERT_MANAGER_VERSION"); certManagerVersion != "" {
		config.CertManagerVersion = certManagerVersion
	}

	if certManagerURL := os.Getenv("CERT_MANAGER_CHART_URL"); certManagerURL != "" {
		config.CertManagerChartURL = certManagerURL
	}

	if rancherVersion := os.Getenv("RANCHER_VERSION"); rancherVersion != "" {
		config.RancherVersion = rancherVersion
	}

	if rancherURL := os.Getenv("RANCHER_CHART_URL"); rancherURL != "" {
		config.RancherChartURL = rancherURL
	}

	if sysUpgradeControllerVersion := os.Getenv("SYSTEM_UPGRADE_CONTROLLER_VERSION"); sysUpgradeControllerVersion != "" {
		config.SystemUpgradeControllerVersion = sysUpgradeControllerVersion
	}

	if sysUpgradeControllerURL := os.Getenv("SYSTEM_UPGRADE_CONTROLLER_URL"); sysUpgradeControllerURL != "" {
		config.SystemUpgradeControllerVersion = sysUpgradeControllerURL
	}

	if err := substituteVersions(config); err != nil {
		return nil, err
	}

	return config, nil
}

func substituteVersions(config *E2EConfig) error {
	nginxURL, err := envsubst.Eval(config.NginxURL, func(s string) string {
		return config.NginxVersion
	})
	if err != nil {
		return fmt.Errorf("failed to substiture nginx url: %w", err)
	}
	config.NginxURL = nginxURL

	certManagerURL, err := envsubst.Eval(config.CertManagerChartURL, func(s string) string {
		return config.CertManagerVersion
	})
	if err != nil {
		return fmt.Errorf("failed to substiture cert manager chart url: %w", err)
	}
	config.CertManagerChartURL = certManagerURL

	rancherURL, err := envsubst.Eval(config.RancherChartURL, func(s string) string {
		return config.RancherVersion
	})
	if err != nil {
		return fmt.Errorf("failed to substiture rancher chart url: %w", err)
	}
	config.RancherChartURL = rancherURL

	sysUpgradeControllerURL, err := envsubst.Eval(config.SystemUpgradeControllerURL, func(s string) string {
		return config.SystemUpgradeControllerVersion
	})
	if err != nil {
		return fmt.Errorf("failed to substiture system upgrade controller url: %w", err)
	}
	config.SystemUpgradeControllerURL = sysUpgradeControllerURL

	return nil
}
