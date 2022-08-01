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

package config

type Install struct {
	Firmware string `json:"firmware,omitempty" yaml:"firmware,omitempty"`
	Device   string `json:"device,omitempty" yaml:"device,omitempty"`
	NoFormat bool   `json:"no-format,omitempty" yaml:"no-format,omitempty"`

	ConfigURLs []string `json:"config-urls,omitempty" yaml:"config-urls,omitempty"`
	ISO        string   `json:"iso,omitempty" yaml:"iso,omitempty"`
	SystemURI  string   `json:"system-uri,omitempty" yaml:"system-uri,omitempty"`

	Debug    bool   `json:"debug,omitempty" yaml:"debug,omitempty"`
	TTY      string `json:"tty,omitempty" yaml:"tty,omitempty"`
	PowerOff bool   `json:"poweroff,omitempty" yaml:"poweroff,omitempty"`
	Reboot   bool   `json:"reboot,omitempty" yaml:"reboot,omitempty"`
	EjectCD  bool   `json:"eject-cd,omitempty" yaml:"eject-cd,omitempty"`
}

func (in *Install) DeepCopy() *Install {
	if in == nil {
		return nil
	}
	out := new(Install)
	in.DeepCopyInto(out)
	return out
}

func (in *Install) DeepCopyInto(out *Install) {
	*out = *in
}

type Registration struct {
	URL             string            `json:"url,omitempty" yaml:"url,omitempty" mapstructure:"url"`
	CACert          string            `json:"ca-cert,omitempty" yaml:"ca-cert,omitempty" mapstructure:"ca-cert"`
	EmulateTPM      bool              `json:"emulate-tpm,omitempty" yaml:"emulate-tpm,omitempty" mapstructure:"emulate-tpm"`
	EmulatedTPMSeed int64             `json:"emulated-tpm-seed,omitempty" yaml:"emulated-tpm-seed,omitempty" mapstructure:"emulated-tpm-seed"`
	NoSMBIOS        bool              `json:"no-smbios,omitempty" yaml:"no-smbios,omitempty" mapstructure:"no-smbios"`
	Labels          map[string]string `json:"labels,omitempty" yaml:"labels,omitempty" mapstructure:"labels"`
}

type SystemAgent struct {
	URL             string `json:"url,omitempty" yaml:"url,omitempty"`
	Token           string `json:"token,omitempty" yaml:"token,omitempty"`
	SecretName      string `json:"secret-name,omitempty" yaml:"secret-name,omitempty"`
	SecretNamespace string `json:"secret-namespace,omitempty" yaml:"secret-namespace,omitempty"`
}

type Elemental struct {
	Install      Install      `json:"install,omitempty" yaml:"install,omitempty"`
	Registration Registration `json:"registration,omitempty" yaml:"registration,omitempty"`
	SystemAgent  SystemAgent  `json:"system-agent,omitempty" yaml:"system-agent,omitempty"`
}

type Config struct {
	Elemental   Elemental              `yaml:"elemental" json:"elemental,omitempty"`
	CloudConfig map[string]interface{} `yaml:"cloud-config,omitempty" json:"cloud-config,omitempty"`
}

func (in *Config) DeepCopyInto(out *Config) {
	*out = *in
}

func (in *Config) DeepCopy() *Config {
	if in == nil {
		return nil
	}
	out := new(Config)
	in.DeepCopyInto(out)
	return out
}
