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

package v1beta1

import (
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type ManagedOSVersionChannel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedOSVersionChannelSpec   `json:"spec,omitempty"`
	Status ManagedOSVersionChannelStatus `json:"status,omitempty"`
}

type ManagedOSVersionChannelSpec struct {
	// +optional
	Type string `json:"type,omitempty"`
	// +optional
	// +kubebuilder:default:="1h"
	SyncInterval string `json:"syncInterval,omitempty"`
	// DeleteNoLongerInSyncVersions automatically deletes
	// all no-longer-in-sync ManagedOSVersions that were created by this channel.
	// +optional
	// +kubebuilder:default:=false
	DeleteNoLongerInSyncVersions bool `json:"deleteNoLongerInSyncVersions,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:XPreserveUnknownFields
	// +optional
	Options map[string]runtime.RawExtension `json:"options,omitempty"`
	// +optional
	UpgradeContainer *upgradev1.ContainerSpec `json:"upgradeContainer,omitempty"`
}

type ManagedOSVersionChannelStatus struct {
	// Conditions describe the state of the managed OS version object.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// LastSyncedTime is the timestamp of the last synchronization
	// +optional
	LastSyncedTime *metav1.Time `json:"lastSyncedTime,omitempty"`
	// FailedSynchronizationAttempts counts the number of consecutive synchronization failures
	// +optional
	FailedSynchronizationAttempts int `json:"failedSynchronizationAttempts,omitempty"`
	// SyncedGeneration tracks the spec generation of the last synchronization
	// +optional
	SyncedGeneration int64 `json:"syncedGeneration,omitempty"`
}

// +kubebuilder:object:root=true

// ManagedOSVersionChannelList contains a list of ManagedOSVersionChannels.
type ManagedOSVersionChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedOSVersionChannel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedOSVersionChannel{}, &ManagedOSVersionChannelList{})
}
