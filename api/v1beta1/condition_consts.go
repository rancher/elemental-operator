package v1beta1

const (
	// ReadyCondition indicates the state of object.
	ReadyCondition = "Ready"
)

// Machine Registration conditions
const (
	// SuccefullyCreatedReason documents a machine registration object that was succefully created.
	SuccefullyCreatedReason = "SuccefullyCreated"

	// MissingTokenOrServerUrlReason documents a machine registration object missing rancher server url or failed token generation.
	MissingTokenOrServerUrlReason = "MissingTokenOrServerUrl"

	// RbacCreationFailureReason documents a machine registration object that has RBAC creation failures.
	RbacCreationFailureReason = "RBACCreationFailure"
)

// Machine Inventory conditions
const (
	// SuccefullyCratedPlanReason documents that the secret owned by the machine inventory was succesfully created.
	SuccefullyCratedPlanReason = "SuccefullyCratedPlan"

	// WaitingForPlanReason documents a machine inventory waiting for plan to applied.
	WaitingForPlanReason = "WaitingForPlan"

	// PlanFailure documents failure of plan owned by the machine inventory object.
	PlanFailureReason = "PlanFailure"

	// PlanSuccefullyAppliedReason documents that plan owned by the machine inventory object was succefully applied.
	PlanSuccefullyAppliedReason = "PlanSuccefullyApplied"
)

// Machine Selector conditions
const (
	// WaitingForInventory documents that the machine selector is waiting for a matching machine inventory.
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
