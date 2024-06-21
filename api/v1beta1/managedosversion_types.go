/*
Copyright © 2022 - 2024 SUSE LLC

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
	containerType             = "container"
	isoType                   = "iso"
	ManagedOSVersionFinalizer = "managedosversion.elemental.cattle.io"
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

// ImageCommons is the basic set of data every single image type has
type ImageCommons struct {
	DisplayName string `json:"displayName,omitempty"`
}

// ISO is the basic set of data to refer to an specific ISO
type ISOImage struct {
	ImageCommons `json:",inline"`
	ImageURI     string `json:"uri,omitempty"`
}

// ContainerImage is the metadata for ManagedOSVersions which carries over
// information about the upgrade
type ContainerImage struct {
	ImageCommons `json:",inline"`
	ImageURI     string `json:"upgradeImage,omitempty"`
}

// IsContainerImage returns true if the metadata attached to the version is refered to container image type
func (m *ManagedOSVersion) IsContainerImage() bool {
	return strings.ToLower(m.Spec.Type) == containerType
}

// IsISOImage returns true if the metadata attached to the version is refered to container image type
func (m *ManagedOSVersion) IsISOImage() bool {
	return strings.ToLower(m.Spec.Type) == isoType
}

// ContainerImage returns the metadata of ManegedOSVersion of container type
func (m *ManagedOSVersion) ContainerImage() (*ContainerImage, error) {
	c := &ContainerImage{}
	if !m.IsContainerImage() {
		return nil, fmt.Errorf("ManagedOsVersion %s is not of a container type: %s", m.Name, m.Spec.Type)
	}
	return c, m.MetadataObject(c)
}

// ISOImage returns the metadata of ManegedOSVersion of container type
func (m *ManagedOSVersion) ISOImage() (*ISOImage, error) {
	i := &ISOImage{}
	if !m.IsISOImage() {
		return nil, fmt.Errorf("ManagedOsVersion %s is not of a iso type: %s", m.Name, m.Spec.Type)
	}
	return i, m.MetadataObject(i)
}
