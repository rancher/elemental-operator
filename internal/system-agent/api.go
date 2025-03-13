/*
Copyright © 2022 - 2025 SUSE LLC

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
	WorkDir                       string `json:"workDirectory,omitempty" yaml:"workDirectory,omitempty"`
	LocalEnabled                  bool   `json:"localEnabled,omitempty" yaml:"localEnabled,omitempty"`
	LocalPlanDir                  string `json:"localPlanDirectory,omitempty" yaml:"localPlanDirectory,omitempty"`
	AppliedPlanDir                string `json:"appliedPlanDirectory,omitempty" yaml:"appliedPlanDirectory,omitempty"`
	RemoteEnabled                 bool   `json:"remoteEnabled,omitempty" yaml:"remoteEnabled,omitempty"`
	ConnectionInfoFile            string `json:"connectionInfoFile,omitempty" yaml:"connectionInfoFile,omitempty"`
	PreserveWorkDir               bool   `json:"preserveWorkDirectory,omitempty" yaml:"preserveWorkDirectory,omitempty"`
	ImagesDir                     string `json:"imagesDirectory,omitempty" yaml:"imagesDirectory,omitempty"`
	AgentRegistriesFile           string `json:"agentRegistriesFile,omitempty" yaml:"agentRegistriesFile,omitempty"`
	ImageCredentialProviderConfig string `json:"imageCredentialProviderConfig,omitempty" yaml:"imageCredentialProviderConfig,omitempty"`
	ImageCredentialProviderBinDir string `json:"imageCredentialProviderBinDirectory,omitempty" yaml:"imageCredentialProviderBinDirectory,omitempty"`
	InterlockDir                  string `json:"interlockDirectory,omitempty" yaml:"interlockDirectory,omitempty"`
}

type ConnectionInfo struct {
	KubeConfig string `json:"kubeConfig"`
	Namespace  string `json:"namespace"`
	SecretName string `json:"secretName"`
}
