/*
Copyright © 2022 - 2023 SUSE LLC

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type SeedImageSpec struct {
	// BaseImg the base elemental image used to build the seed image.
	// +optional
	BaseImage string `json:"baseImage"`
	// MachineRegistrationRef a reference to the related MachineRegistration.
	MachineRegistrationRef *corev1.ObjectReference `json:"registrationRef"`
	// BuildContainer settings for a custom container used to generate the
	// downloadable image.
	// +optional
	BuildContainer *BuildContainer `json:"buildContainer"`
	// LifetimeMinutes the time at which the built seed image will be cleaned up.
	// If when the lifetime elapses the built image is being downloaded, the active
	// download will be completed before removing the built image.
	// Default is 60 minutes, set to 0 to disable.
	// +kubebuilder:default:=60
	// +optional
	LifetimeMinutes int32 `json:"cleanupAfterMinutes"`
	// RetriggerBuild triggers to build again a cleaned up seed image.
	// +optional
	RetriggerBuild bool `json:"retriggerBuild"`
	// Size specifies the size of the volume used to store the image.
	// Defaults to 6Gi
	// +kubebuilder:default:=6442450944
	// +optional
	Size resource.Quantity `json:"size"`
	// Type specifies the type of seed image to built.
	// Valid values are iso|raw
	// Defaults to "iso"
	// +kubebuilder:validation:Enum=iso;raw
	// +kubebuilder:default:=iso
	Type SeedImageType `json:"type"`
	// Platform specifies the target platform for the built image. Example: linux/amd64
	// +kubebuilder:example=linux/amd64
	// +kubebuilder:validation:Pattern=`^$|^\S+\/\S+$`
	// +kubebuilder:validation:Type=string
	// +optional
	TargetPlatform Platform `json:"targetPlatform"`
	// CloudConfig contains cloud-config data to be put in the generated iso.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:XPreserveUnknownFields
	// +optional
	CloudConfig map[string]runtime.RawExtension `json:"cloud-config,omitempty" yaml:"cloud-config,omitempty"`
}

type Platform string

type SeedImageType string

const (
	TypeIso SeedImageType = "iso"
	TypeRaw SeedImageType = "raw"
)

type BuildContainer struct {
	// Name of the spawned container
	// +optional
	Name string `json:"name"`
	// Image container image to run
	// +optional
	Image string `json:"image"`
	// Command same as corev1.Container.Command
	// +optional
	Command []string `json:"command"`
	// Args same as corev1.Container.Args
	// +optional
	Args []string `json:"args"`
	// Args same as corev1.Container.ImagePullPolicy
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy"`
}

type SeedImageState string

const (
	SeedImageInit       SeedImageState = "Initialized"
	SeedImageStarted    SeedImageState = "Started"
	SeedImageCompleted  SeedImageState = "Completed"
	SeedImageFailed     SeedImageState = "Failed"
	SeedImageNotStarted SeedImageState = "NotStarted"
)

type SeedImageStatus struct {
	// Conditions describe the state of the machine registration object.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// DownloadToken a token to identify the seed image to download.
	// +optional
	DownloadToken string `json:"downloadToken,omitempty"`
	// DownloadURL the URL from which the SeedImage can be downloaded once built.
	// +optional
	DownloadURL string `json:"downloadURL,omitempty"`
	// State reflect the state of the seed image build process.
	// +kubebuilder:validation:Enum=Initialized;Started;Completed;Failed;NotStarted
	// +optional
	State SeedImageState `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type SeedImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SeedImageSpec   `json:"spec,omitempty"`
	Status SeedImageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SeedImageList contains a list of SeedImages
type SeedImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SeedImage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SeedImage{}, &SeedImageList{})
}
