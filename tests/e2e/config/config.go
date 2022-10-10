package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/drone/envsubst/v2"
	"sigs.k8s.io/yaml"
)

type E2EConfig struct {
	Chart            string `yaml:"chart"`
	ExternalIP       string `yaml:"externalIP"`
	MagicDNS         string `yaml:"magicDNS"`
	BridgeIP         string `yaml:"bridgeIP"`
	OperatorReplicas string `yaml:"operatorReplicas"`
	NoSetup          bool   `yaml:"noSetup"`

	NginxVersion string `yaml:"nginxVersion"`
	NginxURL     string `yaml:"nginxURL"`

	CertManagerVersion  string `yaml:"certManagerVersion"`
	CertManagerChartURL string `yaml:"certManagerChartURL"`

	RancherVersion  string `yaml:"rancherVersion"`
	RancherChartUrl string `yaml:"rancherChartURL"`

	SystemUpgradeControllerVersion string `yaml:"systemUpgradeControllerVersion"`
	SystemUpgradeControllerURL     string `yaml:"systemUpgradeControllerURL"`
}

// ReadE2EConfig read config from yaml and substitute variables using envsubst.
// All variables can be overridden by environmental variables.
func ReadE2EConfig(configPath string) (*E2EConfig, error) {
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
		config.RancherChartUrl = rancherVersion
	}

	if rancherURL := os.Getenv("RANCHER_CHART_URL"); rancherURL != "" {
		config.RancherChartUrl = rancherURL
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

	rancherURL, err := envsubst.Eval(config.RancherChartUrl, func(s string) string {
		return config.RancherVersion
	})
	if err != nil {
		return fmt.Errorf("failed to substiture rancher chart url: %w", err)
	}
	config.RancherChartUrl = rancherURL

	sysUpgradeControllerURL, err := envsubst.Eval(config.SystemUpgradeControllerURL, func(s string) string {
		return config.SystemUpgradeControllerVersion
	})
	if err != nil {
		return fmt.Errorf("failed to substiture system upgrade controller url: %w", err)
	}
	config.SystemUpgradeControllerURL = sysUpgradeControllerURL

	return nil
}
