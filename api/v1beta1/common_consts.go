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

const (
	// ElementalManagedLabel label used to put on resources managed by the elemental operator.
	ElementalManagedLabel = "elemental.cattle.io/managed"

	// ElementalManagedOSImageVersionNameLabel label is used to filter ManagedOSImages referencing a ManagedOSVersion.
	ElementalManagedOSImageVersionNameLabel = "elemental.cattle.io/managed-os-version-name"

	// ElementalManagedOSVersionChannelLabel is used to filter a set of ManagedOSVersions given the channel they originate from.
	ElementalManagedOSVersionChannelLabel = "elemental.cattle.io/channel"

	// ElementalManagedOSVersionChannelLastSyncAnnotation reports when a ManagedOSVersion was last synced from a channel.
	ElementalManagedOSVersionChannelLastSyncAnnotation = "elemental.cattle.io/channel-last-sync"

	// ElementalManagedOSVersionNoLongerSyncedAnnotation is used to mark a no longer in sync ManagedOSVersion, this highlight it can be deleted.
	ElementalManagedOSVersionNoLongerSyncedAnnotation = "elemental.cattle.io/channel-no-longer-in-sync"
	ElementalManagedOSVersionNoLongerSyncedValue      = "true"

	// SASecretSuffix is the suffix used to name registration service account's token secret
	SASecretSuffix = "-token"

	// TimeoutEnvVar is the environment variable key passed to pods to express a timeout
	TimeoutEnvVar = "ELEMENTAL_TIMEOUT"
)
