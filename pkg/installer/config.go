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

package installer

type Install struct {
	ForceEFI bool   `json:"force-efi,omitempty"`
	Device   string `json:"device,omitempty"`
	NoFormat bool   `json:"no-format,omitempty"`

	ISO       string `json:"iso,omitempty"`
	SystemURL string `json:"system.url,omitempty"`

	Debug    bool   `json:"debug,omitempty"`
	TTY      string `json:"tty,omitempty"`
	PowerOff bool   `json:"poweroff,omitempty"`
	Reboot   bool   `json:"reboot,omitempty"`

	Password string   `json:"password,omitempty"`
	SSHKeys  []string `json:"ssh_keys,omitempty"`
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
	URL             string            `json:"url,omitempty"`
	CACert          string            `json:"ca_cert,omitempty"`
	EmulateTPM      bool              `json:"emulateTPM,omitempty"`
	EmulatedTPMSeed int64             `json:"emulatedTPMSeed,omitempty"`
	NoSMBIOS        bool              `json:"noSMBIOS,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type SystemAgent struct {
	URL             string `json:"url,omitempty"`
	Token           string `json:"token,omitempty"`
	SecretName      string `json:"secret_name,omitempty"`
	SecretNamespace string `json:"secret_namespace,omitempty"`
}

type Elemental struct {
	Install      Install      `json:"install,omitempty"`
	Registration Registration `json:"registration,omitempty"`
	SystemAgent  SystemAgent  `json:"system_agent,omitempty"`
}

type Config struct {
	Elemental Elemental `json:"elemental,omitempty"`
}
