/*
Copyright 2024.

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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
)

// ManagedOSChangelogReconciler reconciles a ManagedOSChangelog object
type ManagedOSChangelogReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedoschangelogs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedoschangelogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedoschangelogs/finalizers,verbs=update

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedOSChangelogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elementalv1.ManagedOSChangelog{}).
		Complete(r)
}

func (r *ManagedOSChangelogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	changelog := &elementalv1.ManagedOSChangelog{}
	if err := r.Get(ctx, req.NamespacedName, changelog); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(5).Info("Object was not found, not an error")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get managedoschangelog object: %w", err)
	}

	patchBase := client.MergeFrom(changelog.DeepCopy())

	// Collect errors aas an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, changelog)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reconciling changelog object: %w", err))
	}

	changelogStatusCopy := changelog.Status.DeepCopy() // Patch call will erase the status

	if err := r.Patch(ctx, changelog, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch status for managedoschangelog object: %w", err))
	}

	changelog.Status = *changelogStatusCopy

	if err := r.Status().Patch(ctx, changelog, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch status for changelog object: %w", err))
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errorutils.NewAggregate(errs)
	}

	return result, nil
}

func (r *ManagedOSChangelogReconciler) reconcile(ctx context.Context, changelog *elementalv1.ManagedOSChangelog) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling managedoschangelog object")

	if changelog.GetDeletionTimestamp() != nil {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}
