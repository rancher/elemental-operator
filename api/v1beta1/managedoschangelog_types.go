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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ManagedOSChangelogFinalizer = "managedoschangelog.elemental.cattle.io"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type ManagedOSChangelog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedOSChangelogSpec   `json:"spec,omitempty"`
	Status ManagedOSChangelogStatus `json:"status,omitempty"`
}

type ManagedOSChangelogSpec struct {
	// ManagedOSVersionChannelRef a referemce to the related ManagedOSVersionChannel.
	ManagedOSVersionChannelRef *corev1.ObjectReference `json:"channelRef"`
	// ManagedOSVersion the name of the ManagedOSVersion resource for which the changelog
	// should be generated.
	ManagedOSVersion string `json:"osVersion,omitempty"`
	// LifetimeMinutes the time at which the changelog data will be cleaned up.
	// Default is 60 minutes, set to 0 to disable.
	// +kubebuilder:default:=60
	// +optional
	LifetimeMinutes int32 `json:"cleanupAfterMinutes"`
	// Refresh triggers to build again a cleaned up changelog.
	// +optional
	Refresh bool `json:"refresh"`
}

type ChangelogState string

const (
	ChangelogInit       ChangelogState = "Initialized"
	ChangelogStarted    ChangelogState = "Started"
	ChangelogCompleted  ChangelogState = "Completed"
	ChangelogFailed     ChangelogState = "Failed"
	ChangelogNotStarted ChangelogState = "NotStarted"
)

type ManagedOSChangelogStatus struct {
	// Conditions describe the state of the changelog object.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// ChangelogURL the URL where changelog data can be displayed
	// +optional
	ChangelogURL string `json:"changelogURL,omitempty"`
	// State reflect the state of the changelog generation process.
	// +kubebuilder:validation:Enum=Initialized;Started;Completed;Failed;NotStarted
	// +optional
	State ChangelogState `json:"state,omitempty"`
}

// +kubebuilder:object:root=true

// ManagedOSChangelogList contains a list of ManagedOSChangelogs.
type ManagedOSChangelogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedOSChangelog `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedOSChangelog{}, &ManagedOSChangelogList{})
}
