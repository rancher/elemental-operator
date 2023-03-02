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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
)

type SeedImageReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=seedimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=seedimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=machineregistrations,verbs=get;watch;list
// TODO: restrict access to pods to the required namespace only
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;watch;create;list

// TODO: extend SetupWithManager with "Watches" and "WithEventFilter"
func (r *SeedImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elementalv1.SeedImage{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

func (r *SeedImageReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	seedImg := &elementalv1.SeedImage{}
	if err := r.Get(ctx, req.NamespacedName, seedImg); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(5).Info("Object was not found, not an error")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get seedimage object: %w", err)
	}

	patchBase := client.MergeFrom(seedImg.DeepCopy())

	// Collect errors as an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, seedImg)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reconciling seedimage object: %w", err))
	}

	seedImgStatusCopy := seedImg.Status.DeepCopy() // Patch call will erase the status

	if err := r.Patch(ctx, seedImg, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch status for seedimage object: %w", err))
	}

	seedImg.Status = *seedImgStatusCopy

	if err := r.Status().Patch(ctx, seedImg, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch status for seedimage object: %w", err))
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errorutils.NewAggregate(errs)
	}

	return result, nil
}

func (r *SeedImageReconciler) reconcile(ctx context.Context, seedImg *elementalv1.SeedImage) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling seedimage object")

	if seedImg.GetDeletionTimestamp() != nil {
		// TODO: r.reconcileDelete()
		return ctrl.Result{}, nil
	}

	controllerutil.AddFinalizer(seedImg, elementalv1.SeedImageFinalizer)

	// TODO: add logic to manage the Pod

	return ctrl.Result{}, nil
}
