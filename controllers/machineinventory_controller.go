package controllers

import (
	"context"
	"fmt"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// MachineInventoryReconciler reconciles a MachineInventory object.
type MachineInventoryReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;watch;create;update;delete

func (r *MachineInventoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapFunc := r.machineInventorySelectorMapFunc()
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
		Watches(
			&source.Kind{
				Type: &elementalv1.MachineInventorySelector{},
			},
			handler.EnqueueRequestsFromMapFunc(mapFunc),
		).
		Complete(r)
}

func (r *MachineInventoryReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	mInventory := &elementalv1.MachineInventory{}
	err := r.Get(ctx, req.NamespacedName, mInventory)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(5).Info("Object was not found, not an error")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get machine inventory object: %w", err)
	}

	patchBase := client.MergeFrom(mInventory.DeepCopy())

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

	logger.Info("Reconciling machineregistration object")

	if mInventory.GetDeletionTimestamp() != nil {
		controllerutil.RemoveFinalizer(mInventory, elementalv1.MachineInventoryFinalizer) // TODO: Handle deletion
		return ctrl.Result{}, nil
	}

	if err := r.createPlanSecret(ctx, mInventory); err != nil {
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.SuccefullyCratedPlanReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	if err := r.updateInventoryWithPlanStatus(ctx, mInventory); err != nil {
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.PlanFailureReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	controllerutil.AddFinalizer(mInventory, elementalv1.MachineInventoryFinalizer)

	return ctrl.Result{}, nil
}

func (r *MachineInventoryReconciler) createPlanSecret(ctx context.Context, mInventory *elementalv1.MachineInventory) error {
	logger := ctrl.LoggerFrom(ctx)

	if readyCondition := meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.ReadyCondition); readyCondition != nil {
		logger.V(5).Info("Skipping plan secret creation because ready condition is already set")
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

	if err := r.Get(ctx, types.NamespacedName{
		Namespace: mInventory.Namespace,
		Name:      mInventory.Name,
	}, planSecret); err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	appliedChecksum := string(planSecret.Data["applied-checksum"])
	failedChecksum := string(planSecret.Data["failed-checksum"])

	logger.Info("Attempting to set plan status")

	switch {
	case appliedChecksum != "":
		logger.V(5).Info("Plan succefully applied")
		mInventory.Status.Plan.State = elementalv1.PlanApplied
		mInventory.Status.Plan.Checksum = appliedChecksum
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.PlanSuccefullyAppliedReason,
			Status:  metav1.ConditionTrue,
			Message: "plan succefully applied",
		})
		return nil
	case failedChecksum != "":
		logger.V(5).Info("Plan failed to be applied")
		mInventory.Status.Plan.State = elementalv1.PlanFailed
		mInventory.Status.Plan.Checksum = failedChecksum
		return fmt.Errorf("failed to apply plan")
	default:
		logger.V(5).Info("Waiting for plan to be applied")
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.WaitingForPlanReason,
			Status:  metav1.ConditionFalse,
			Message: "waiting for plan to be applied",
		})
		return nil
	}
}

// machineInventorySelectorMapFunc is used to filter out machine inventory selector that we
// don't want to trigger a reconciliation of machine inventory. Currenly all selector will pass.
func (r *MachineInventoryReconciler) machineInventorySelectorMapFunc() handler.MapFunc {
	return func(o client.Object) []reconcile.Request {
		mInventorySelector, ok := o.(*elementalv1.MachineInventorySelector)
		if !ok {
			return nil
		}
		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: mInventorySelector.Namespace,
					Name:      mInventorySelector.Name,
				},
			},
		}
	}
}
