package systemagent

type Plan struct {
	Files                []File                `json:"files,omitempty"`
	OneTimeInstructions  []OneTimeInstruction  `json:"instructions,omitempty"`
	Probes               map[string]Probe      `json:"probes,omitempty"`
	PeriodicInstructions []PeriodicInstruction `json:"periodicInstructions,omitempty"`
}

type OneTimeInstruction struct {
	CommonInstruction
	SaveOutput bool `json:"saveOutput,omitempty"`
}

type CommonInstruction struct {
	Name    string   `json:"name,omitempty"`
	Image   string   `json:"image,omitempty"`
	Env     []string `json:"env,omitempty"`
	Args    []string `json:"args,omitempty"`
	Command string   `json:"command,omitempty"`
}

type PeriodicInstruction struct {
	CommonInstruction
	PeriodSeconds    int  `json:"periodSeconds,omitempty"`
	SaveStderrOutput bool `json:"saveStderrOutput,omitempty"`
}

type File struct {
	Content     string `json:"content,omitempty"`
	Directory   bool   `json:"directory,omitempty"`
	UID         int    `json:"uid,omitempty"`
	GID         int    `json:"gid,omitempty"`
	Path        string `json:"path,omitempty"`
	Permissions string `json:"permissions,omitempty"`
}

type Probe struct {
	Name                string        `json:"name,omitempty"`
	InitialDelaySeconds int           `json:"initialDelaySeconds,omitempty"`
	TimeoutSeconds      int           `json:"timeoutSeconds,omitempty"`
	SuccessThreshold    int           `json:"successThreshold,omitempty"`
	FailureThreshold    int           `json:"failureThreshold,omitempty"`
	HTTPGetAction       HTTPGetAction `json:"httpGet,omitempty"`
}

type HTTPGetAction struct {
	URL        string `json:"url,omitempty"`
	Insecure   bool   `json:"insecure,omitempty"`
	ClientCert string `json:"clientCert,omitempty"`
	ClientKey  string `json:"clientKey,omitempty"`
	CACert     string `json:"caCert,omitempty"`
}

type AgentConfig struct {
	WorkDir                       string `json:"workDirectory,omitempty"`
	LocalEnabled                  bool   `json:"localEnabled,omitempty"`
	LocalPlanDir                  string `json:"localPlanDirectory,omitempty"`
	AppliedPlanDir                string `json:"appliedPlanDirectory,omitempty"`
	RemoteEnabled                 bool   `json:"remoteEnabled,omitempty"`
	ConnectionInfoFile            string `json:"connectionInfoFile,omitempty"`
	PreserveWorkDir               bool   `json:"preserveWorkDirectory,omitempty"`
	ImagesDir                     string `json:"imagesDirectory,omitempty"`
	AgentRegistriesFile           string `json:"agentRegistriesFile,omitempty"`
	ImageCredentialProviderConfig string `json:"imageCredentialProviderConfig,omitempty"`
	ImageCredentialProviderBinDir string `json:"imageCredentialProviderBinDirectory,omitempty"`
	InterlockDir                  string `json:"interlockDirectory,omitempty"`
}

type ConnectionInfo struct {
	KubeConfig string `json:"kubeConfig"`
	Namespace  string `json:"namespace"`
	SecretName string `json:"secretName"`
}