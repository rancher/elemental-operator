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

package v1beta1

import (
	"fmt"
	"strings"

	"github.com/rancher/elemental-operator/pkg/object"
	fleetv1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

const (
	containerType = "container"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type ManagedOSVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedOSVersionSpec   `json:"spec,omitempty"`
	Status ManagedOSVersionStatus `json:"status,omitempty"`
}

type ManagedOSVersionSpec struct {
	// +optional
	Version string `json:"version,omitempty"`
	// +optional
	Type string `json:"type,omitempty"`
	// +optional
	MinVersion string `json:"minVersion,omitempty"`
	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:XPreserveUnknownFields
	// +optional
	Metadata map[string]runtime.RawExtension `json:"metadata,omitempty"`
	// +optional
	UpgradeContainer *upgradev1.ContainerSpec `json:"upgradeContainer,omitempty"`
}

type ManagedOSVersionStatus struct {
	fleetv1.BundleStatus `json:""` //nolint
}

// +kubebuilder:object:root=true

// ManagedOSVersionList contains a list of ManagedOSVersions.
type ManagedOSVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedOSVersion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedOSVersion{}, &ManagedOSVersionList{})
}

// MetadataObject converts the Metadata in the ManagedOSVersionSpec to a concrete object
func (m *ManagedOSVersion) MetadataObject(v interface{}) error {
	return object.RenderRawExtension(m.Spec.Metadata, v)
}

// ContainerImage is the metadata for ManagedOSVersions which carries over
// information about the upgrade
type ContainerImage struct {
	Metadata
	TargetUpgradeImage string `json:"targetUpgradeImage,omitempty"`
}

// ISO is a ISO upgrade strategy
type ISO struct {
	Metadata
	URL      string `json:"isoURL,omitempty"`
	Checksum string `json:"isoChecksum,omitempty"`
}

// Metadata is the basic set of data required to construct objects needed to perform upgrades via Kubernetes
type Metadata struct {
	ImageURI string `json:"upgradeImage,omitempty"`
}

// IsContainerImage returns true if the metadata attached to the version is refered to a
// upgrade via container strategy
func (m *ManagedOSVersion) IsContainerImage() bool {
	return strings.ToLower(m.Spec.Type) == containerType
}

// ContainerImageMetadata returns a ContainerImageMetadata struct from the underlaying metadata
func (m *ManagedOSVersion) ContainerImageMetadata() (*ContainerImage, error) {
	c := &ContainerImage{}

	if m.IsContainerImage() {
		err := m.MetadataObject(c)
		if err != nil {
			return nil, fmt.Errorf("foo")

		}

		return c, nil
	}

	return nil, fmt.Errorf("metadata is not containerimage type")
}

// Metadata returns the underlaying basic metadata required to handle upgrades
func (m *ManagedOSVersion) Metadata() (*Metadata, error) {
	c := &Metadata{}
	return c, m.MetadataObject(c)
}
