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

package controllers

import (
	"context"
	"fmt"
	"time"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/syncer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const retry = 5 * time.Second

// ManagedOSVersionChannelReconciler reconciles a ManagedOSVersionChannel object.
type ManagedOSVersionChannelReconciler struct {
	client.Client
	Config         *rest.Config
	OperatorImage  string
	syncerProvider syncer.SyncerProvider
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversionchannels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversionchannels/status,verbs=get;update;patch;list
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversions,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=manangedosversions/status,verbs=get;update;patch

func (r *ManagedOSVersionChannelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elementalv1.ManagedOSVersionChannel{}).
		Complete(r)
}

func (r *ManagedOSVersionChannelReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) { //nolint:dupl
	logger := ctrl.LoggerFrom(ctx)

	managedOSVersionChannel := &elementalv1.ManagedOSVersionChannel{}
	err := r.Get(ctx, req.NamespacedName, managedOSVersionChannel)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Object was not found, not an error")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get managed OS version channel object: %w", err)
	}

	patchBase := client.MergeFrom(managedOSVersionChannel.DeepCopy())

	// Collect errors as an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, managedOSVersionChannel)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reconciling managed OS version channel object: %w", err))
	}

	managedosversionchannelStatusCopy := managedOSVersionChannel.Status.DeepCopy() // Patch call will erase the status

	if err := r.Patch(ctx, managedOSVersionChannel, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch status for managed OS version channel object: %w", err))
	}

	managedOSVersionChannel.Status = *managedosversionchannelStatusCopy

	if err := r.Status().Patch(ctx, managedOSVersionChannel, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch status for managed OS version channel object: %w", err))
	}

	return result, errorutils.NewAggregate(errs)
}

func (r *ManagedOSVersionChannelReconciler) reconcile(ctx context.Context, managedOSVersionChannel *elementalv1.ManagedOSVersionChannel) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling managed OS version channel object")

	if managedOSVersionChannel.Spec.Type == "" { // TODO: This should be part of validation webhook and moved out of the controller
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.InvalidConfigurationReason,
			Status:  metav1.ConditionTrue,
			Message: "spec.Type can't be empty",
		})
		return ctrl.Result{}, nil
	}
	interval, err := time.ParseDuration(managedOSVersionChannel.Spec.SyncInterval)
	if err != nil { // TODO: This should be part of validation webhook and moved out of the controller
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.InvalidConfigurationReason,
			Status:  metav1.ConditionTrue,
			Message: "spec.SyncInterval is not parseable by time.ParseDuration",
		})
		return ctrl.Result{}, nil
	}

	sync, err := r.syncerProvider.NewOSVersionsSyncer(managedOSVersionChannel.Spec, r.OperatorImage, r.Config)
	if err != nil {
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.InvalidConfigurationReason,
			Status:  metav1.ConditionTrue,
			Message: "failed to create a syncer",
		})
		return ctrl.Result{}, nil
	}

	if managedOSVersionChannel.Status.LastSyncedTime != nil {
		lastSync := managedOSVersionChannel.Status.LastSyncedTime.Time
		scheduledTime := lastSync.Add(interval)
		if time.Now().Before(scheduledTime) {
			return reconcile.Result{RequeueAfter: scheduledTime.Sub(time.Now())}, nil
		}
	}

	vers, requeue, err := sync.Sync(ctx, r.Client, managedOSVersionChannel)
	if err != nil {
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToSyncReason,
			Status:  metav1.ConditionFalse,
			Message: "Failed syncing channel",
		})
		return reconcile.Result{RequeueAfter: retry}, err
	}

	if requeue {
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.SyncingReason,
			Status:  metav1.ConditionFalse,
			Message: "On going channel synchronization",
		})
		return reconcile.Result{RequeueAfter: retry}, nil
	}

	err = r.syncVersions(ctx, vers, managedOSVersionChannel)
	if err != nil {
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToCreateVersionsReason,
			Status:  metav1.ConditionFalse,
			Message: "Failed creating managed OS versions",
		})
		return reconcile.Result{RequeueAfter: retry}, err
	}

	meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
		Type:   elementalv1.ReadyCondition,
		Reason: elementalv1.SyncedReason,
		Status: metav1.ConditionTrue,
	})

	now := metav1.Now()
	managedOSVersionChannel.Status.LastSyncedTime = &now

	return ctrl.Result{RequeueAfter: interval}, nil
}

func (r *ManagedOSVersionChannelReconciler) syncVersions(ctx context.Context, vers []elementalv1.ManagedOSVersion, ch *elementalv1.ManagedOSVersionChannel) error {
	var errs []error
	logger := ctrl.LoggerFrom(ctx)

	for _, v := range vers {
		vcpy := v.DeepCopy()
		vcpy.ObjectMeta.Namespace = ch.Namespace
		vcpy.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: elementalv1.GroupVersion.String(),
				Kind:       "ManagedOSVersionChannel",
				Name:       ch.Name,
				UID:        ch.UID,
				Controller: pointer.Bool(true),
			},
		}

		if ch.Spec.UpgradeContainer != nil {
			vcpy.Spec.UpgradeContainer = ch.Spec.UpgradeContainer
		}

		if err := r.Create(ctx, vcpy); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logger.Info("There is alerady a version defined for", "managedosversion", vcpy.Name)
				// TODO shouldn't we patch/update it?
			} else {
				logger.Error(err, "failed to create", "managedosversion", vcpy.Name)
				errs = append(errs, err)
			}
		}
	}
	return errorutils.NewAggregate(errs)
}
