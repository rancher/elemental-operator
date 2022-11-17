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

const (
	// ReadyCondition indicates the state of object.
	ReadyCondition = "Ready"
)

// Machine Registration conditions
const (
	// SuccefullyCreatedReason documents a machine registration object that was succefully created.
	SuccefullyCreatedReason = "SuccefullyCreated"

	// MissingTokenOrServerURLReason documents a machine registration object missing rancher server url or failed token generation.
	MissingTokenOrServerURLReason = "MissingTokenOrServerURL"

	// RbacCreationFailureReason documents a machine registration object that has RBAC creation failures.
	RbacCreationFailureReason = "RbacCreationFailure"
)

// Machine Inventory conditions
const (
	// SuccefullyCreatedPlanReason documents that the secret owned by the machine inventory was succesfully created.
	SuccefullyCreatedPlanReason = "SuccefullyCreatedPlan"

	// WaitingForPlanReason documents a machine inventory waiting for plan to applied.
	WaitingForPlanReason = "WaitingForPlan"

	// PlanFailure documents failure of plan owned by the machine inventory object.
	PlanFailureReason = "PlanFailure"

	// PlanSuccefullyAppliedReason documents that plan owned by the machine inventory object was succefully applied.
	PlanSuccefullyAppliedReason = "PlanSuccefullyApplied"
)

// Machine Selector conditions
const (
	// WaitingForInventoryReason documents that the machine selector is waiting for a matching machine inventory.
	WaitingForInventoryReason = "WaitingForInventory"

	// SuccefullyAdoptedInventoryReason documents that the machine selector succesfully adopted machine inventory.
	SuccefullyAdoptedInventoryReason = "SuccefullyAdoptedInventory"

	// FailedToAdoptInventoryReason documents that the machine selector failed to adopt machine inventory.
	FailedToAdoptInventoryReason = "FailedToAdoptInventory"

	// SuccefullyUpdatedPlanReason documents that the machine selector succesfully updated secret plan with bootstrap.
	SuccefullyUpdatedPlanReason = "SuccefullyUpdatedPlan"

	// FailedToUpdatePlanReason documents that the machine selector failed to update secret plan with bootstrap.
	FailedToUpdatePlanReason = "FailedToUpdatePlan"

	// SelectorReadyReason documents that the machine selector is ready.
	SelectorReadyReason = "SelectorReady"

	// FailedToSetAdressesReason documents that the machine selector controller failed to set adresses.
	FailedToSetAdressesReason = "FailedToSetAdresses"
)

// Managed OS Version Channel conditions
const (
	// InvalidConfigurationReason documents that managed OS version channel has invalid configuration.
	InvalidConfigurationReason = "InvalidConfiguration"

	// SyncingReason documents that managed OS version channel is synchronizing managed OS versions
	SyncingReason = "Synchronizing"

	// GotChannelDataReason documents that managed OS version channel successfully fetched managed OS versions data
	GotChannelDataReason = "GotChannelData"

	// SyncedReason documents that managed OS version channel finalized synchroniziation and managed OS versions, if any, were created
	SyncedReason = "Synchronized"

	// FailedToSyncReason documents that managed OS version channel failed synchronization
	FailedToSyncReason = "FailedToSync"

	// FailedToCreateVersionsReason documents that managed OS version channel failed to create managed OS versions
	FailedToCreateVersionsReason = "FailedToCreateVersions"
)

// Managed OS Image conditions
const (
	// FleetBundleCreation documents the state of the fleet bundle creation.
	FleetBundleCreation = "FleetBundleCreation"

	// FleetBundleCreatedSuccessReason documents that managed OS image controller fleet bundle was created successfully.
	FleetBundleCreateSuccessReason = "FleetBundleCreateSuccess"

	// FleetBundleCreateFailureReason documents that managed OS image controller failed to create fleet bundle.
	FleetBundleCreateFailureReason = "FleetBundleCreateFailure"
)
