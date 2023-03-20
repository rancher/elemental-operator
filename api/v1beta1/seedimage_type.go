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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type SeedImageSpec struct {
	// BaseImg the base elemental image used to build the seed image.
	// +optional
	BaseImage string `json:"baseImage"`
	// MachineRegistrationRef a reference to the related MachineRegistration.
	MachineRegistrationRef *corev1.ObjectReference `json:"registrationRef"`
	// CloudConfig contains cloud-config data to be put in the generated iso.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:XPreserveUnknownFields
	// +optional
	CloudConfig map[string]runtime.RawExtension `json:"cloud-config,omitempty" yaml:"cloud-config,omitempty"`
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
