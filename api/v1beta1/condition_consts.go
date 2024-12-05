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

const (
	// ReadyCondition indicates the state of object.
	ReadyCondition = "Ready"
)

// Machine Registration conditions
const (
	// SuccessfullyCreatedReason documents a machine registration object that was successfully created.
	SuccessfullyCreatedReason = "SuccessfullyCreated"

	// MissingTokenOrServerURLReason documents a machine registration object missing rancher server url or failed token generation.
	MissingTokenOrServerURLReason = "MissingTokenOrServerURL"

	// RbacCreationFailureReason documents a machine registration object that has RBAC creation failures.
	RbacCreationFailureReason = "RbacCreationFailure"
)

// Machine Inventory conditions
const (
	// PlanCreationFailureReason documents that the secret plan owned by the machine inventory could not be created
	PlanCreationFailureReason = "PlanCreationFailureReason"

	// WaitingForPlanReason documents a machine inventory waiting for plan to applied.
	WaitingForPlanReason = "WaitingForPlan"

	// PlanFailure documents failure of plan owned by the machine inventory object.
	PlanFailureReason = "PlanFailure"

	// PlanFailure documents failure in deleting all IPClaims associated to this machine inventory.
	IPReleaseFailureReason = "IPReleaseFailure"

	// PlanSuccessfullyAppliedReason documents that plan owned by the machine inventory object was successfully applied.
	PlanSuccessfullyAppliedReason = "PlanSuccessfullyApplied"
)

const (
	// AdoptedCondition documents the state of a machine selector adopting the machine inventory
	AdoptionReadyCondition = "AdoptionReady"

	// WaitingToBeAdoptedReason documents that the machine inventory is waiting to be adopted
	WaitingToBeAdoptedReason = "WaitingToBeAdopted"

	// ValidatinAdoptionReason documents that the machine inventory is in the process of validating its owner has the appropriate status
	ValidatingAdoptionReason = "ValidatingAdoption"

	// SuccessfullyAdoptedReason documents that the machine inventory is successully adopted by a machine selector
	SuccessfullyAdoptedReason = "SuccessfullyAdopted"

	// AdoptionFailureReason documents that the machine inventory adoption process failed
	AdoptionFailureReason = "AdoptionFailure"

	// NetworkConfigReady documents the state of a machine inventory network config
	NetworkConfigReady = "NetworkConfigReady"

	// NetworkConfigFailure documents a failure when reconciling the network config for this Machine Inventory.
	NetworkConfigFailure = "NetworkConfigFailure"

	// WaitingForIPAddressReason documents a machine inventory waiting for an IPAddress to be allocated by an IPAM provider.
	WaitingForIPAddressReason = "WaitingForIPAddress"

	// ReconcilingNetworkConfig documents the operator needs to do some work to reconcile the NetworkConfig.
	ReconcilingNetworkConfig = "ReconcilingNetworkConfig"
)

// Machine Selector conditions
const (
	// WaitingForInventoryReason documents that the machine selector is waiting for a matching machine inventory.
	WaitingForInventoryReason = "WaitingForInventory"

	// SuccessfullyUpdatedPlanReason documents that the machine selector successfully updated secret plan with bootstrap.
	SuccessfullyUpdatedPlanReason = "SuccessfullyUpdatedPlan"

	// FailedToUpdatePlanReason documents that the machine selector failed to update secret plan with bootstrap.
	FailedToUpdatePlanReason = "FailedToUpdatePlan"

	// FailedToSetAdressesReason documents that the machine selector controller failed to set adresses.
	FailedToSetAdressesReason = "FailedToSetAdresses"

	// SelectorReadyReason documents that the machine selector is ready.
	SelectorReadyReason = "SelectorReady"
)

const (
	// InventoryReady documents the state of the selector adopting an inventory
	InventoryReadyCondition = "InventoryReady"

	// WaitForInventoryCheckReason documents the selector is waiting for the inventory to validate the adoption
	WaitForInventoryCheckReason = "WaitForInventoryCheck"

	// SuccessfullyAdoptedInventoryReason documents that the machine selector successfully adopted machine inventory.
	SuccessfullyAdoptedInventoryReason = "SuccessfullyAdoptedInventory"

	// FailedToAdoptInventoryReason documents that the machine selector failed to adopt machine inventory.
	FailedToAdoptInventoryReason = "FailedToAdoptInventory"
)

// Managed OS Version Channel conditions
const (
	// InvalidConfigurationReason documents that managed OS version channel has invalid configuration.
	InvalidConfigurationReason = "InvalidConfiguration"

	// SyncingReason documents that managed OS version channel is synchronizing managed OS versions
	SyncingReason = "Synchronizing"

	// SyncedReason documents that managed OS version channel finalized synchroniziation and managed OS versions, if any, were created
	SyncedReason = "Synchronized"

	// FailedToSyncReason documents that managed OS version channel failed synchronization
	FailedToSyncReason = "FailedToSync"

	// FailedToCreatePodReason documents that managed OS version channel failed to create the synchronization pod
	FailedToCreatePodReason = "FailedToCreatePod"

	// ChannelDisabledReason documents that the managed OS version channel is not enabled
	ChannelDisabledReason = "ChannelDisabled"
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

// Seed Image conditions
const (
	// ResourcesNotCreatedYet documents resources creation not started yet.
	ResourcesNotCreatedYet = "ResourcesNotCreatedYet"

	// SetOwnerFailureReason documents failure setting MachineRegistratioRef as SeedImage owner.
	SetOwnerFailureReason = "RegistrationNotFound"

	// PodCreationFailureReason documents Pod creation failure.
	PodCreationFailureReason = "PodCreationFailure"

	// ServiceCreationFailureReason documents Service creation failure.
	ServiceCreationFailureReason = "ServiceCreationFailure"

	// ResourcesSuccessfullyCreatedReason documents all the resources needed to start the build image task were successfully created.
	ResourcesSuccessfullyCreatedReason = "ResourcesSuccessfullyCreated"
)

// Managed OS Changelog conditions
const (
	// WorkerPodReadyCondition is the condition type tracking the state of the worked pod.
	WorkerPodReadyCondition = "WorkerPodReady"
	// WorkerPodNotStartedReason documents worker pod not being started.
	WorkerPodNotStartedReason = "WorkerPodNotStarted"
	// WorkerPodInitializing documents worker pod being initialized.
	WorkerPodInitReason = "WorkerPodInitializing"
	// WorkerPodCompletionFailureReason documents failure to successfully complete the worker pod tasks.
	WorkerPodCompletionFailureReason = "WorkerPodCompletionFailure"
	// WorkerPodCompletionSuccessReason documents worker pod completed its tasks and is successfully running.
	WorkerPodCompletionSuccessReason = "WorkerPodCompletionSuccess"
	// WorkerPodDeadline documents worker pod deadline has elapsed.
	WorkerPodDeadline = "WorkerPodDeadline"
	// WorkerPodUnknown documents worker pod in an unknown status.
	WorkerPodUnknown = "WorkerPodUnknown"
)

// Seed Image conditions
const (
	// SeedImageConditionReady is the condition type tracking the state of the seed image build pod.
	SeedImageConditionReady = "SeedImageReady"
	// SeedImageBuildNotStartedReason documents seed image build job not started.
	SeedImageBuildNotStartedReason = "SeedImageBuildNotStarted"
	// SeedImageBuildOngoingReason documents seed image build job is ongoing.
	SeedImageBuildOngoingReason = "SeedImageBuildOngoing"
	// SeedImageBuildFailureReason documents seed image build job failure.
	SeedImageBuildFailureReason = "SeedImageBuildFailure"
	// SeedIMageExposeFailureReason documents failure to set the URL to download the seed image.
	SeedImageExposeFailureReason = "SeedImageExposeFailure"
	// SeedImageBuildSuccessReason documents seed image build job success.
	SeedImageBuildSuccessReason = "SeedImageBuildSuccess"
	// SeedImageBuildDeadline documents seed image build deadline has elapsed.
	SeedImageBuildDeadline = "SeedImageBuildDeadline"
	// SeedImageBuildUnknown documents seed image build job in unknown status.
	SeedImageBuildUnknown = "SeedImageBuildUnknown"
)
