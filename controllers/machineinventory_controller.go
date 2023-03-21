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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/util"
)

// Timeout to validate machine inventory adoption
const adoptionTimeout = 5

// MachineInventoryReconciler reconciles a MachineInventory object.
type MachineInventoryReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=machineinventories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=machineinventories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;watch;create;list;watch

func (r *MachineInventoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elementalv1.MachineInventory{}).
		Owns(&corev1.Secret{}).
		Watches(
			&source.Kind{
				Type: &corev1.Secret{},
			},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &elementalv1.MachineInventory{}}).
		WithEventFilter(r.ignoreIncrementalStatusUpdate()).
		Complete(r)
}

func (r *MachineInventoryReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) { //nolint:dupl
	logger := ctrl.LoggerFrom(ctx)

	mInventory := &elementalv1.MachineInventory{}
	err := r.Get(ctx, req.NamespacedName, mInventory)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(log.DebugDepth).Info("Object was not found, not an error")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get machine inventory object: %w", err)
	}

	patchBase := client.MergeFrom(mInventory.DeepCopy())

	// We have to sanitize the conditions because old API definitions didn't have proper validation.
	mInventory.Status.Conditions = util.RemoveInvalidConditions(mInventory.Status.Conditions)

	// Collect errors as an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, mInventory)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reconciling machine inventory object: %w", err))
	}

	machineInventoryStatusCopy := mInventory.Status.DeepCopy() // Patch call will erase the status

	if err := r.Patch(ctx, mInventory, patchBase); err != nil {
		errs = append(errs, fmt.Errorf("failed to patch status for machine inventory object: %w", err))
	}

	mInventory.Status = *machineInventoryStatusCopy

	if err := r.Status().Patch(ctx, mInventory, patchBase); err != nil {
		errs = append(errs, fmt.Errorf("failed to patch status for machine inventory object: %w", err))
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errorutils.NewAggregate(errs)
	}

	return result, nil
}

func (r *MachineInventoryReconciler) reconcile(ctx context.Context, mInventory *elementalv1.MachineInventory) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling machineinventory object")

	if mInventory.GetDeletionTimestamp() != nil {
		controllerutil.RemoveFinalizer(mInventory, elementalv1.MachineInventoryFinalizer)
		return ctrl.Result{}, nil
	}

	if err := r.createPlanSecret(ctx, mInventory); err != nil {
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.PlanCreationFailureReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to create plan secret: %w", err)
	}

	if err := r.updateInventoryWithPlanStatus(ctx, mInventory); err != nil {
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.PlanFailureReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to update inventory status with plan %w", err)
	}

	if requeue, err := r.updateInventoryWithAdoptionStatus(ctx, mInventory); err != nil {
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.AdoptionReadyCondition,
			Reason:  elementalv1.AdoptionFailureReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to update inventory status with plan %w", err)
	} else if requeue {
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *MachineInventoryReconciler) createPlanSecret(ctx context.Context, mInventory *elementalv1.MachineInventory) error {
	logger := ctrl.LoggerFrom(ctx)

	readyCondition := meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.ReadyCondition)
	if readyCondition != nil && readyCondition.Reason != elementalv1.PlanCreationFailureReason {
		logger.Info("Skipping plan secret creation because ready condition is already set")
		return nil
	}
	logger.Info("Creating plan secret")

	planSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: mInventory.Namespace,
			Name:      mInventory.Name,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: elementalv1.GroupVersion.String(),
					Kind:       "MachineInventory",
					Name:       mInventory.Name,
					UID:        mInventory.UID,
					Controller: pointer.Bool(true),
				},
			},
			Labels: map[string]string{
				elementalv1.ElementalManagedLabel: "",
			},
		},
		Type:       elementalv1.PlanSecretType,
		StringData: map[string]string{"plan": "{}"},
	}

	if err := r.Create(ctx, planSecret); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	mInventory.Status.Plan = &elementalv1.PlanStatus{
		PlanSecretRef: &corev1.ObjectReference{
			Namespace: planSecret.Namespace,
			Name:      planSecret.Name,
		},
	}

	meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
		Type:    elementalv1.ReadyCondition,
		Reason:  elementalv1.WaitingForPlanReason,
		Status:  metav1.ConditionFalse,
		Message: "waiting for plan to be applied",
	})

	return nil
}

func (r *MachineInventoryReconciler) updateInventoryWithPlanStatus(ctx context.Context, mInventory *elementalv1.MachineInventory) error {
	logger := ctrl.LoggerFrom(ctx)

	planSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: mInventory.Namespace,
			Name:      mInventory.Name,
		},
	}

	logger.Info("Attempting to set plan status")
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: mInventory.Namespace,
		Name:      mInventory.Name,
	}, planSecret); err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	appliedChecksum := string(planSecret.Data["applied-checksum"])
	failedChecksum := string(planSecret.Data["failed-checksum"])

	switch {
	case appliedChecksum != "":
		logger.Info("Plan successfully applied")
		mInventory.Status.Plan.State = elementalv1.PlanApplied
		mInventory.Status.Plan.Checksum = appliedChecksum
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.PlanSuccessfullyAppliedReason,
			Status:  metav1.ConditionTrue,
			Message: "plan successfully applied",
		})
		return nil
	case failedChecksum != "":
		logger.Info("Plan failed to be applied")
		mInventory.Status.Plan.State = elementalv1.PlanFailed
		mInventory.Status.Plan.Checksum = failedChecksum
		return fmt.Errorf("failed to apply plan")
	default:
		logger.Info("Waiting for plan to be applied")
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.WaitingForPlanReason,
			Status:  metav1.ConditionFalse,
			Message: "waiting for plan to be applied",
		})
		return nil
	}
}

// updateInventoryWithAdoptionStatus computes sanity checks on owner references to verify inventory owner is properly set.
// Returns true if a requeue to wait for owner setup is required, false if no requeue is needed.
func (r *MachineInventoryReconciler) updateInventoryWithAdoptionStatus(ctx context.Context, mInventory *elementalv1.MachineInventory) (bool, error) {
	logger := ctrl.LoggerFrom(ctx)

	owner := getSelectorOwner(mInventory)
	if owner == nil {
		logger.V(log.DebugDepth).Info("Waiting to be adopted")
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.AdoptionReadyCondition,
			Reason:  elementalv1.WaitingToBeAdoptedReason,
			Status:  metav1.ConditionFalse,
			Message: "Waiting to be adopted",
		})
		return false, nil
	}

	adoptedCondition := meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.AdoptionReadyCondition)
	if adoptedCondition != nil && adoptedCondition.Status == metav1.ConditionTrue {
		logger.V(log.DebugDepth).Info("Inventory already adopted")
		return false, nil
	}

	miSelector := &elementalv1.MachineInventorySelector{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: mInventory.Namespace,
		Name:      owner.Name,
	},
		miSelector,
	); err != nil {
		return false, fmt.Errorf("failed to get machine inventory selector: %w", err)
	}

	meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
		Type:    elementalv1.AdoptionReadyCondition,
		Reason:  elementalv1.ValidatingAdoptionReason,
		Status:  metav1.ConditionUnknown,
		Message: "Adoption being validated",
	})
	adoptedCondition = meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.AdoptionReadyCondition)

	deadLine := adoptedCondition.LastTransitionTime.Add(adoptionTimeout * time.Second)

	switch {
	case miSelector.Status.MachineInventoryRef == nil && time.Now().Before(deadLine):
		logger.V(log.DebugDepth).Info("Adoption being validated")
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.AdoptionReadyCondition,
			Reason:  elementalv1.ValidatingAdoptionReason,
			Status:  metav1.ConditionUnknown,
			Message: "Adoption being validated",
		})
		return true, nil
	case miSelector.Status.MachineInventoryRef == nil:
		removeSelectorOwnerShip(mInventory)
		return false, fmt.Errorf("Adoption timeout, dropping selector ownership. Deadline was: %v ", deadLine)
	case miSelector.Status.MachineInventoryRef.Name != mInventory.Name:
		removeSelectorOwnerShip(mInventory)
		return false, fmt.Errorf("Ownership mismatch, dropping selector ownership")
	default:
		logger.V(log.DebugDepth).Info("Successfully adopted")
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.AdoptionReadyCondition,
			Reason:  elementalv1.SuccessfullyAdoptedReason,
			Status:  metav1.ConditionTrue,
			Message: "Successfully adopted",
		})
		return false, nil
	}
}

func (r *MachineInventoryReconciler) ignoreIncrementalStatusUpdate() predicate.Funcs {
	return predicate.Funcs{
		// Avoid reconciling if the event triggering the reconciliation is related to incremental status updates
		// for MachineInventory resources only
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetObjectKind().GroupVersionKind().Kind != "MachineInventory" {
				return true
			}

			oldMInventory := e.ObjectOld.(*elementalv1.MachineInventory).DeepCopy()
			newMInventory := e.ObjectNew.(*elementalv1.MachineInventory).DeepCopy()

			oldMInventory.Status = elementalv1.MachineInventoryStatus{}
			newMInventory.Status = elementalv1.MachineInventoryStatus{}

			oldMInventory.ObjectMeta.ResourceVersion = ""
			newMInventory.ObjectMeta.ResourceVersion = ""

			return !cmp.Equal(oldMInventory, newMInventory)
		},
	}
}

func findSelectorOwner(machineInventory *elementalv1.MachineInventory) (int, *metav1.OwnerReference) {
	for idx, owner := range machineInventory.GetOwnerReferences() {
		if owner.APIVersion == elementalv1.GroupVersion.String() && owner.Kind == "MachineInventorySelector" {
			return idx, &owner
		}
	}
	return -1, nil
}

func getSelectorOwner(machineInventory *elementalv1.MachineInventory) *metav1.OwnerReference {
	_, owner := findSelectorOwner(machineInventory)
	return owner
}

func isAlreadyOwned(machineInventory *elementalv1.MachineInventory) bool {
	return getSelectorOwner(machineInventory) != nil
}

func removeSelectorOwnerShip(machineInventory *elementalv1.MachineInventory) {
	idx, _ := findSelectorOwner(machineInventory)
	if idx >= 0 {
		owners := machineInventory.GetOwnerReferences()
		owners[idx] = owners[len(owners)-1]
		machineInventory.OwnerReferences = owners[:len(owners)-1]
	}
}
