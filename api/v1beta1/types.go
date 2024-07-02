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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

const TPMRandomSeedValue = -1

type Install struct {
	// +optional
	Firmware string `json:"firmware,omitempty" yaml:"firmware,omitempty"`
	// +optional
	Device string `json:"device,omitempty" yaml:"device,omitempty"`
	// +optional
	DeviceSelector DeviceSelector `json:"device-selector,omitempty" yaml:"device-selector,omitempty"`
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
	// +optional
	// +kubebuilder:default:={"type": "loopdevice"}
	Snapshotter SnapshotterConfig `json:"snapshotter,omitempty" yaml:"snapshotter,omitempty"`
}

type SnapshotterConfig struct {
	// Type sets the snapshotter type a new installation, available options are 'loopdevice' and 'btrfs'
	// +optional
	// +kubebuilder:default:=loopdevice
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

type Reset struct {
	// +optional
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
	// +optional
	// +kubebuilder:default:=true
	ResetPersistent bool `json:"reset-persistent,omitempty" yaml:"reset-persistent,omitempty" mapstructure:"reset-persistent"`
	// +optional
	// +kubebuilder:default:=true
	ResetOEM bool `json:"reset-oem,omitempty" yaml:"reset-oem,omitempty" mapstructure:"reset-oem"`
	// +optional
	ConfigURLs []string `json:"config-urls,omitempty" yaml:"config-urls,omitempty" mapstructure:"config-urls"`
	// +optional
	SystemURI string `json:"system-uri,omitempty" yaml:"system-uri,omitempty" mapstructure:"system-uri"`
	// +optional
	Debug bool `json:"debug,omitempty" yaml:"debug,omitempty" mapstructure:"debug"`
	// +optional
	PowerOff bool `json:"poweroff,omitempty" yaml:"poweroff,omitempty" mapstructure:"poweroff"`
	// +optional
	// +kubebuilder:default:=true
	Reboot bool `json:"reboot,omitempty" yaml:"reboot,omitempty" mapstructure:"reboot"`
	// +optional
	DisableBootEntry bool `json:"disable-boot-entry,omitempty" yaml:"disable-boot-entry,omitempty"`
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
	// +optional
	// +kubebuilder:default:=tpm
	Auth string `json:"auth,omitempty" yaml:"auth,omitempty" mapstructure:"auth"`
	// +optional
	NoToolkit bool `json:"no-toolkit,omitempty" yaml:"no-toolkit,omitempty" mapstructure:"no-toolkit"`
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
	// +kubebuilder:default:={"reset-persistent":true,"reset-oem":true,"reboot":true}
	Reset Reset `json:"reset,omitempty" yaml:"reset,omitempty"`
	// +optional
	Registration Registration `json:"registration,omitempty" yaml:"registration,omitempty"`
	// +optional
	SystemAgent SystemAgent `json:"system-agent,omitempty" yaml:"system-agent,omitempty"`
}

type Network struct {
	// +optional
	IPPoolRef *corev1.TypedLocalObjectReference `json:"ipPoolRef,omitempty"`
}

type Config struct {
	// +optional
	Elemental Elemental `json:"elemental,omitempty" yaml:"elemental"`
	// +optional
	Network NetworkTemplate `json:"network,omitempty" yaml:"network"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:XPreserveUnknownFields
	// +optional
	CloudConfig map[string]runtime.RawExtension `json:"cloud-config,omitempty" yaml:"cloud-config,omitempty"`
}

type DeviceSelector []DeviceSelectorRequirement

type DeviceSelectorRequirement struct {
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Name;Size
	Key DeviceSelectorKey `json:"key"`
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=In;NotIn;Gt;Lt
	Operator DeviceSelectorOperator `json:"operator"`
	// +optional
	Values []string `json:"values,omitempty"`
}

type DeviceSelectorKey string
type DeviceSelectorOperator string

const (
	DeviceSelectorOpIn    DeviceSelectorOperator = "In"
	DeviceSelectorOpNotIn DeviceSelectorOperator = "NotIn"
	DeviceSelectorOpGt    DeviceSelectorOperator = "Gt"
	DeviceSelectorOpLt    DeviceSelectorOperator = "Lt"

	DeviceSelectorKeyName DeviceSelectorKey = "Name"
	DeviceSelectorKeySize DeviceSelectorKey = "Size"
)

// NetworkTemplate contains a list of IPAddressPools and a network config template.
// This template can be defined in both MachineRegistrations and MachineSelectors.
type NetworkTemplate struct {
	IPAddresses []IPAddressPool                 `json:"ipAddresses,omitempty" yaml:"ipAddresses,omitempty"`
	Config      map[string]runtime.RawExtension `json:"config,omitempty" yaml:"config,omitempty"`
}

// NetworkConfig contains a list of claimed IPAddresses and a network config template.
// This config is a digested NetworkTemplate, the MachineInventory Ready condition highlight that
// this config is ready to be consumed, this means all needed IPAddressClaims for this machine
// have been created and the IPAM provider served real IPAddresses that can be applied to the machine.
type NetworkConfig struct {
	IPAddresses map[string]string               `json:"ipAddresses,omitempty"`
	Config      map[string]runtime.RawExtension `json:"config,omitempty"`
}

// IPAddressPool contains an IPAddressPool reference that can be used to generate an IPAddressClaim.
type IPAddressPool struct {
	Name      string                            `json:"name,omitempty"`
	IPPoolRef *corev1.TypedLocalObjectReference `json:"ipPoolRef,omitempty"`
}
