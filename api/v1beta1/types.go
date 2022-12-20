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

package v1beta1

import runtime "k8s.io/apimachinery/pkg/runtime"

type Install struct {
	// +optional
	Firmware string `json:"firmware,omitempty" yaml:"firmware,omitempty"`
	// +optional
	Device string `json:"device,omitempty" yaml:"device,omitempty"`
	// +optional
	NoFormat bool `json:"no-format,omitempty" yaml:"no-format,omitempty"`
	// +optional
	ConfigURLs []string `json:"config-urls,omitempty" yaml:"config-urls,omitempty"`
	// +optional
	ISO string `json:"iso,omitempty" yaml:"iso,omitempty"`
	// +optional
	SystemURI string `json:"system-uri,omitempty" yaml:"system-uri,omitempty"`
	// +optional
	Debug bool `json:"debug,omitempty" yaml:"debug,omitempty"`
	// +optional
	TTY string `json:"tty,omitempty" yaml:"tty,omitempty"`
	// +optional
	PowerOff bool `json:"poweroff,omitempty" yaml:"poweroff,omitempty"`
	// +optional
	Reboot bool `json:"reboot,omitempty" yaml:"reboot,omitempty"`
	// +optional
	EjectCD bool `json:"eject-cd,omitempty" yaml:"eject-cd,omitempty"`
	// +optional
	DisableBootEntry bool `json:"disable-boot-entry,omitempty" yaml:"disable-boot-entry,omitempty"`
	// +optional
	ConfigDir string `json:"config-dir,omitempty" yaml:"config-dir,omitempty"`
}

type Registration struct {
	// +optional
	URL string `json:"url,omitempty" yaml:"url,omitempty" mapstructure:"url"`
	// +optional
	CACert string `json:"ca-cert,omitempty" yaml:"ca-cert,omitempty" mapstructure:"ca-cert"`
	// +optional
	EmulateTPM bool `json:"emulate-tpm,omitempty" yaml:"emulate-tpm,omitempty" mapstructure:"emulate-tpm"`
	// +optional
	EmulatedTPMSeed int64 `json:"emulated-tpm-seed,omitempty" yaml:"emulated-tpm-seed,omitempty" mapstructure:"emulated-tpm-seed"`
	// +optional
	NoSMBIOS bool `json:"no-smbios,omitempty" yaml:"no-smbios,omitempty" mapstructure:"no-smbios"`
}

type SystemAgent struct {
	// +optional
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
	// +optional
	Token string `json:"token,omitempty" yaml:"token,omitempty"`
	// +optional
	SecretName string `json:"secret-name,omitempty" yaml:"secret-name,omitempty"`
	// +optional
	SecretNamespace string `json:"secret-namespace,omitempty" yaml:"secret-namespace,omitempty"`
}

type Elemental struct {
	// +optional
	Install Install `json:"install,omitempty" yaml:"install,omitempty"`
	// +optional
	Registration Registration `json:"registration,omitempty" yaml:"registration,omitempty"`
	// +optional
	SystemAgent SystemAgent `json:"system-agent,omitempty" yaml:"system-agent,omitempty"`
}

type Config struct {
	// +optional
	Elemental Elemental `json:"elemental,omitempty" yaml:"elemental"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:XPreserveUnknownFields
	// +optional
	CloudConfig map[string]runtime.RawExtension `json:"cloud-config,omitempty" yaml:"cloud-config,omitempty"`
}
