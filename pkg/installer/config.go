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
	return
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
