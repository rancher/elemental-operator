/*
Copyright © 2022 - 2025 SUSE LLC

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
	"errors"
	"fmt"
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ManagedOSVersionReconciler reconciles a ManagedOSVersion object.
type ManagedOSVersionReconciler struct {
	client.Client
	DefaultRegistry string
	Scheme          *runtime.Scheme
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversions/status,verbs=get;update;patch;list
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosimages,verbs=get;list;watch

func (r *ManagedOSVersionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elementalv1.ManagedOSVersion{}).
		Watches(
			&elementalv1.ManagedOSImage{},
			handler.EnqueueRequestsFromMapFunc(r.ManagedOSImageToManagedOSVersion),
		).
		Complete(r)
}

func (r *ManagedOSVersionReconciler) ManagedOSImageToManagedOSVersion(ctx context.Context, obj client.Object) []ctrl.Request {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Enqueueing ManagedOSVersion reconciliation from ManagedOSImage")

	requests := []ctrl.Request{}

	// Verify we are actually handling a ManagedOSImage object
	managedOSImage, ok := obj.(*elementalv1.ManagedOSImage)
	if !ok {
		logger.Error(errors.New("Enqueueing error"), fmt.Sprintf("Expected a ManagedOSImage object, but got %T", obj))
		return []ctrl.Request{}
	}

	// Check the ManagedOSImage was referencing any ManagedOSVersion
	if len(managedOSImage.Spec.ManagedOSVersionName) > 0 {
		logger.Info("Adding ManagedOSVersion to reconciliation request", "ManagedOSVersion", managedOSImage.Spec.ManagedOSVersionName)
		name := client.ObjectKey{Namespace: managedOSImage.Namespace, Name: managedOSImage.Spec.ManagedOSVersionName}
		requests = append(requests, ctrl.Request{NamespacedName: name})
	}

	return requests
}

func (r *ManagedOSVersionReconciler) Reconcile(ctx context.Context, req reconcile.Request) (_ ctrl.Result, rerr error) {
	logger := ctrl.LoggerFrom(ctx).WithValues("Namespace", req.Namespace).WithValues("ManagedOSVersion", req.Name)
	logger.Info("Reconciling ManagedOSVersion")

	// Fetch the ManagedOSVersion
	managedOSVersion := &elementalv1.ManagedOSVersion{}
	if err := r.Client.Get(ctx, req.NamespacedName, managedOSVersion); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetching ManagedOSVersion: %w", err)
	}

	// Ensure we patch the latest version otherwise we could erratically overlap with other controllers (e.g. backup and restore)
	patchBase := client.MergeFromWithOptions(managedOSVersion.DeepCopy(), client.MergeFromWithOptimisticLock{})
	defer func() {
		managedOSVersionStatusCopy := managedOSVersion.Status.DeepCopy() // Patch call will erase the status

		if err := r.Patch(ctx, managedOSVersion, patchBase); err != nil {
			rerr = errors.Join(rerr, fmt.Errorf("patching ManagedOSVersion: %w", err))
		}

		managedOSVersion.Status = *managedOSVersionStatusCopy

		// If the object was waiting for deletion and we just removed the finalizer, we will get a not found error
		if err := r.Status().Patch(ctx, managedOSVersion, patchBase); err != nil && !apierrors.IsNotFound(err) {
			rerr = errors.Join(rerr, fmt.Errorf("patching ManagedOSVersion status: %w", err))
		}
	}()

	// The object is not up for deletion
	if managedOSVersion.GetDeletionTimestamp().IsZero() {
		// The object is not being deleted, so register the finalizer
		if !controllerutil.ContainsFinalizer(managedOSVersion, elementalv1.ManagedOSVersionFinalizer) {
			controllerutil.AddFinalizer(managedOSVersion, elementalv1.ManagedOSVersionFinalizer)
		}

		// Reconcile ManagedOSVersion
		result, err := r.reconcileNormal(ctx, managedOSVersion)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("reconciling ManagedOSVersion: %w", err)
		}
		return result, nil
	}

	// Object is up for deletion
	if controllerutil.ContainsFinalizer(managedOSVersion, elementalv1.ManagedOSVersionFinalizer) {
		result, err := r.reconcileDelete(ctx, managedOSVersion)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("reconciling ManagedOSVersion deletion: %w", err)
		}
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *ManagedOSVersionReconciler) reconcileNormal(ctx context.Context, managedOSVersion *elementalv1.ManagedOSVersion) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx).WithValues("Namespace", managedOSVersion.Namespace).WithValues("ManagedOSVersion", managedOSVersion.Name)
	logger.Info("Normal ManagedOSVersion reconcile")
	// Nothing to do here
	return ctrl.Result{}, nil
}

func (r *ManagedOSVersionReconciler) reconcileDelete(ctx context.Context, managedOSVersion *elementalv1.ManagedOSVersion) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx).WithValues("Namespace", managedOSVersion.Namespace).WithValues("ManagedOSVersion", managedOSVersion.Name)
	logger.Info("Deletion ManagedOSVersion reconcile")
	managedOSImages, err := r.getManagedOSImages(ctx, *managedOSVersion)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting linked ManagedOSImages: %w", err)
	}
	// If there are ManagedOSImages referencing this ManagedOSVersion, hold on deletion.
	if len(managedOSImages.Items) > 0 {
		imageNames := []string{}
		for _, image := range managedOSImages.Items {
			imageNames = append(imageNames, image.Name)
		}
		logger.Info("ManagedOSVersion still in use by ManagedOSImages", "ManagedOSImages", strings.Join(imageNames, ","))
		return ctrl.Result{}, nil
	}

	controllerutil.RemoveFinalizer(managedOSVersion, elementalv1.ManagedOSVersionFinalizer)
	return ctrl.Result{}, nil
}

// getManagedOSImages returns a list of ManagedOSImages referencing the ManagedOSVersion.
func (r *ManagedOSVersionReconciler) getManagedOSImages(ctx context.Context, managedOSVersion elementalv1.ManagedOSVersion) (*elementalv1.ManagedOSImageList, error) {
	managedOSImages := &elementalv1.ManagedOSImageList{}

	if err := r.List(ctx, managedOSImages, client.InNamespace(managedOSVersion.Namespace), client.MatchingLabels(map[string]string{
		elementalv1.ElementalManagedOSImageVersionNameLabel: managedOSVersion.Name,
	})); err != nil {
		return nil, fmt.Errorf("listing ManagedOSImages: %w", err)
	}

	return managedOSImages, nil
}
