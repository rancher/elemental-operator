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
