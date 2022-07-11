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
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	containerType = "container"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ManagedOSVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedOSVersionSpec   `json:"spec"`
	Status ManagedOSVersionStatus `json:"status"`
}

type ManagedOSVersionSpec struct {
	Version          string                   `json:"version,omitempty"`
	Type             string                   `json:"type,omitempty"`
	MinVersion       string                   `json:"minVersion,omitempty"`
	Metadata         *fleet.GenericMap        `json:"metadata,omitempty"`
	UpgradeContainer *upgradev1.ContainerSpec `json:"upgradeContainer,omitempty"`
}

type ManagedOSVersionStatus struct {
	fleet.BundleStatus
}

// MetadataObject converts the Metadata in the ManagedOSVersionSpec to a concrete object
func (g *ManagedOSVersion) MetadataObject(v interface{}) error {
	return object.Render(g.Spec.Metadata.Data, v)
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
func (g *ManagedOSVersion) IsContainerImage() bool {
	return strings.ToLower(g.Spec.Type) == containerType
}

// ContainerImageMetadata returns a ContainerImageMetadata struct from the underlaying metadata
func (g *ManagedOSVersion) ContainerImageMetadata() (*ContainerImage, error) {
	c := &ContainerImage{}

	if g.IsContainerImage() {
		err := g.MetadataObject(c)
		if err != nil {
			return nil, fmt.Errorf("foo")

		}

		return c, nil
	}

	return nil, fmt.Errorf("metadata is not containerimage type")
}

// Metadata returns the underlaying basic metadata required to handle upgrades
func (g *ManagedOSVersion) Metadata() (*Metadata, error) {
	c := &Metadata{}
	return c, g.MetadataObject(c)
}
