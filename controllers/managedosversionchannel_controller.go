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

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ManagedOSVersionChannelReconciler reconciles a ManagedOSVersionChannel object.
type ManagedOSVersionChannelReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversionchannels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversionchannels/status,verbs=get;update;patch;list

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

	if len(errs) > 0 {
		return ctrl.Result{}, errorutils.NewAggregate(errs)
	}

	return result, nil
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

	meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
		Type:   elementalv1.ReadyCondition,
		Reason: elementalv1.SuccefullyCreatedReason,
		Status: metav1.ConditionTrue,
	})

	return ctrl.Result{}, nil
}
