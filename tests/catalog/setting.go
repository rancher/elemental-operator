package catalog

type Setting struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string `json:"kind" yaml:"kind"`
	Metadata   struct {
		Name string `json:"name" yaml:"name"`
	} `json:"metadata" yaml:"metadata"`
	Source string `json:"source" yaml:"source"`
	Value  string `json:"value" yaml:"value"`
}

func NewSetting(name string, source, value string) *Setting {
	return &Setting{
		APIVersion: "management.cattle.io/v3",
		Metadata: struct {
			Name string "json:\"name\" yaml:\"name\""
		}{Name: name},
		Kind:   "Setting",
		Source: source,
		Value:  value,
	}
}
