package catalog

type MachineRegistration struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
	Metadata   struct {
		Name string `json:"name" yaml:"name"`
	} `json:"metadata" yaml:"metadata"`
	Spec struct {
		CloudConfig map[string]interface{} `json:"cloudConfig" yaml:"cloudConfig"`
	} `json:"spec" yaml:"spec"`
}

func NewMachineRegistration(name string, cloudConfig map[string]interface{}) *MachineRegistration {
	return &MachineRegistration{
		APIVersion: "rancheros.cattle.io/v1",
		Metadata: struct {
			Name string "json:\"name\" yaml:\"name\""
		}{Name: name},
		Kind: "MachineRegistration",
		Spec: struct {
			CloudConfig map[string]interface{} "json:\"cloudConfig\" yaml:\"cloudConfig\""
		}{
			CloudConfig: cloudConfig,
		},
	}
}
