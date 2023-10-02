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

const (
	// ElementalManagedLabel label used to put on resources managed by the elemental operator.
	ElementalManagedLabel = "elemental.cattle.io/managed"

	// ElementalManagedLabel label used to put on resources managed by the elemental operator.
	ElementalManagedOSVersionChannelLabel = "elemental.cattle.io/channel"

	// SASecretSuffix is the suffix used to name registration service account's token secret
	SASecretSuffix = "-token"

	// TimeoutEnvVar is the environment variable key passed to pods to express a timeout
	TimeoutEnvVar = "ELEMENTAL_TIMEOUT"
)
