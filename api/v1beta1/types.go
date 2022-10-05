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
	Firmware string `json:"firmware,omitempty"`
	// +optional
	Device string `json:"device,omitempty"`
	// +optional
	NoFormat bool `json:"no-format,omitempty"`
	// +optional
	ConfigURLs []string `json:"config-urls,omitempty"`
	// +optional
	ISO string `json:"iso,omitempty"`
	// +optional
	SystemURI string `json:"system-uri,omitempty"`
	// +optional
	Debug bool `json:"debug,omitempty"`
	// +optional
	TTY string `json:"tty,omitempty"`
	// +optional
	PowerOff bool `json:"poweroff,omitempty"`
	// +optional
	Reboot bool `json:"reboot,omitempty"`
	// +optional
	EjectCD bool `json:"eject-cd,omitempty"`
}

type Registration struct {
	// +optional
	URL string `json:"url,omitempty"`
	// +optional
	CACert string `json:"ca-cert,omitempty"`
	// +optional
	EmulateTPM bool `json:"emulate-tpm,omitempty"`
	// +optional
	EmulatedTPMSeed int64 `json:"emulated-tpm-seed,omitempty"`
	// +optional
	NoSMBIOS bool `json:"no-smbios,omitempty"`
}

type SystemAgent struct {
	// +optional
	URL string `json:"url,omitempty"`
	// +optional
	Token string `json:"token,omitempty"`
	// +optional
	SecretName string `json:"secret-name,omitempty"`
	// +optional
	SecretNamespace string `json:"secret-namespace,omitempty"`
}

type Elemental struct {
	// +optional
	Install Install `json:"install,omitempty"`
	// +optional
	Registration Registration `json:"registration,omitempty"`
	// +optional
	SystemAgent SystemAgent `json:"system-agent,omitempty"`
}

type Config struct {
	// +optional
	Elemental Elemental `json:"elemental,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:XPreserveUnknownFields
	// +optional
	CloudConfig map[string]runtime.RawExtension `json:"cloud-config,omitempty"`
}
