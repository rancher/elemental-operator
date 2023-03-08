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
	"errors"
	"fmt"

	"github.com/google/go-cmp/cmp"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/randomtoken"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/util"
)

// MachineRegistrationReconciler reconciles a MachineRegistration object.
type MachineRegistrationReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=machineregistrations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=machineregistrations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings;roles,verbs=create;delete;list;watch
// +kubebuilder:rbac:groups="management.cattle.io",resources=settings,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=create;delete;get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=create;delete;list;watch;update

func (r *MachineRegistrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elementalv1.MachineRegistration{}).
		WithEventFilter(r.ignoreIncrementalStatusUpdate()).
		Complete(r)
}

func (r *MachineRegistrationReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) { //nolint:dupl
	logger := ctrl.LoggerFrom(ctx)

	mRegistration := &elementalv1.MachineRegistration{}
	err := r.Get(ctx, req.NamespacedName, mRegistration)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Object was not found, registration client has to create it")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get machine registration object: %w", err)
	}

	patchBase := client.MergeFrom(mRegistration.DeepCopy())

	// We have to sanitize the conditions because old API definitions didn't have proper validation.
	mRegistration.Status.Conditions = util.RemoveInvalidConditions(mRegistration.Status.Conditions)

	// Collect errors as an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, mRegistration)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reconciling machine registration object: %w", err))
	}

	machineRegistrationStatusCopy := mRegistration.Status.DeepCopy() // Patch call will erase the status

	if err := r.Patch(ctx, mRegistration, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch status for machine registration object: %w", err))
	}

	mRegistration.Status = *machineRegistrationStatusCopy

	if err := r.Status().Patch(ctx, mRegistration, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch status for machine registration object: %w", err))
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errorutils.NewAggregate(errs)
	}

	return result, nil
}

func (r *MachineRegistrationReconciler) reconcile(ctx context.Context, mRegistration *elementalv1.MachineRegistration) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling machineregistration object")

	if mRegistration.GetDeletionTimestamp() != nil {
		return r.reconcileDelete(ctx, mRegistration)
	}

	if meta.IsStatusConditionTrue(mRegistration.Status.Conditions, elementalv1.ReadyCondition) {
		logger.Info("Machine registration is ready, no need to reconcile it")
		return ctrl.Result{}, nil
	}

	controllerutil.AddFinalizer(mRegistration, elementalv1.MachineRegistrationFinalizer)

	if err := r.setRegistrationTokenAndURL(ctx, mRegistration); err != nil {
		meta.SetStatusCondition(&mRegistration.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.MissingTokenOrServerURLReason,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to set registration token and url: %w", err)
	}

	if err := r.createRBACObjects(ctx, mRegistration); err != nil {
		meta.SetStatusCondition(&mRegistration.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.RbacCreationFailureReason,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to create RBAC objects: %w", err)
	}

	meta.SetStatusCondition(&mRegistration.Status.Conditions, metav1.Condition{
		Type:   elementalv1.ReadyCondition,
		Reason: elementalv1.SuccessfullyCreatedReason,
		Status: metav1.ConditionTrue,
	})

	return ctrl.Result{}, nil
}

func (r *MachineRegistrationReconciler) setRegistrationTokenAndURL(ctx context.Context, mRegistration *elementalv1.MachineRegistration) error {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Setting registration token and url")

	serverURL, err := r.getRancherServerURL(ctx)
	if err != nil {
		return err
	}

	if mRegistration.Status.RegistrationToken == "" {
		mRegistration.Status.RegistrationToken, err = randomtoken.Generate()
		if err != nil {
			return fmt.Errorf("failed to generate registration token: %w", err)
		}
	}

	mRegistration.Status.RegistrationURL = fmt.Sprintf("%s/elemental/registration/%s", serverURL, mRegistration.Status.RegistrationToken)

	return nil
}

func (r *MachineRegistrationReconciler) getRancherServerURL(ctx context.Context) (string, error) {
	logger := ctrl.LoggerFrom(ctx)

	setting := &managementv3.Setting{}
	if err := r.Get(ctx, types.NamespacedName{Name: "server-url"}, setting); err != nil {
		return "", fmt.Errorf("failed to get server url setting: %w", err)
	}

	if setting.Value == "" {
		err := errors.New("server-url is not set")
		logger.Error(err, "can't get server-url")
		return "", err
	}

	return setting.Value, nil
}

func (r *MachineRegistrationReconciler) createRBACObjects(ctx context.Context, mRegistration *elementalv1.MachineRegistration) error {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling RBAC resources")

	ownerReferences := []metav1.OwnerReference{
		{
			APIVersion: elementalv1.GroupVersion.String(),
			Kind:       "MachineRegistration",
			Name:       mRegistration.Name,
			UID:        mRegistration.UID,
			Controller: pointer.Bool(true),
		},
	}

	logger.Info("Creating role")
	if err := r.Create(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mRegistration.Name,
			Namespace:       mRegistration.Namespace,
			OwnerReferences: ownerReferences,
			Labels: map[string]string{
				elementalv1.ElementalManagedLabel: "",
			},
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Verbs:     []string{"get", "watch", "list", "update", "patch"}, // TODO: Review permissions, does it need update, patch?
			Resources: []string{"secrets"},
		}, {
			APIGroups: []string{"management.cattle.io"},
			Verbs:     []string{"get", "watch", "list"},
			Resources: []string{"settings"},
		},
		},
	}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create role: %w", err)
	}

	logger.Info("Creating service account")
	if err := r.Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mRegistration.Name,
			Namespace:       mRegistration.Namespace,
			OwnerReferences: ownerReferences,
			Labels: map[string]string{
				elementalv1.ElementalManagedLabel: "",
			},
		},
		Secrets: []corev1.ObjectReference{
			{
				Name: mRegistration.Name + "-token",
			},
		},
	}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	logger.Info("Creating token secret for the service account")
	if err := r.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            mRegistration.Name + "-token",
			Namespace:       mRegistration.Namespace,
			OwnerReferences: ownerReferences,
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": mRegistration.Name,
			},
			Labels: map[string]string{
				elementalv1.ElementalManagedLabel: "",
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	logger.Info("Creating role binding")
	if err := r.Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       mRegistration.Namespace,
			Name:            mRegistration.Name,
			OwnerReferences: ownerReferences,
			Labels: map[string]string{
				elementalv1.ElementalManagedLabel: "",
			},
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      mRegistration.Name,
			Namespace: mRegistration.Namespace,
		}},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     mRegistration.Name,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	logger.Info("Setting service account ref")
	mRegistration.Status.ServiceAccountRef = &corev1.ObjectReference{
		Kind:      "ServiceAccount",
		Namespace: mRegistration.Namespace,
		Name:      mRegistration.Name,
	}

	return nil
}

func (r *MachineRegistrationReconciler) reconcileDelete(ctx context.Context, mRegistration *elementalv1.MachineRegistration) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Deleting RBAC resources")

	if err := r.Client.Delete(ctx, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: mRegistration.Namespace,
			Name:      mRegistration.Name,
		}}); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to delete role: %w", err)
	}
	if err := r.Client.Delete(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: mRegistration.Namespace,
			Name:      mRegistration.Name,
		}}); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to delete service account: %w", err)
	}
	if err := r.Client.Delete(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: mRegistration.Namespace,
			Name:      mRegistration.Name + "-token",
		}}); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to delete service account: %w", err)
	}
	if err := r.Client.Delete(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: mRegistration.Namespace,
			Name:      mRegistration.Name,
		}}); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to delete role binding: %w", err)
	}

	controllerutil.RemoveFinalizer(mRegistration, elementalv1.MachineRegistrationFinalizer)

	return ctrl.Result{}, nil
}

func (r *MachineRegistrationReconciler) ignoreIncrementalStatusUpdate() predicate.Funcs {
	return predicate.Funcs{
		// Avoid reconciling if the event triggering the reconciliation is related to incremental status updates
		// for MachineRegistration resources only
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetObjectKind().GroupVersionKind().Kind != "MachineRegistration" {
				return true
			}

			oldMRegistration := e.ObjectOld.(*elementalv1.MachineRegistration).DeepCopy()
			newMregistration := e.ObjectNew.(*elementalv1.MachineRegistration).DeepCopy()

			oldMRegistration.Status = elementalv1.MachineRegistrationStatus{}
			newMregistration.Status = elementalv1.MachineRegistrationStatus{}

			oldMRegistration.ObjectMeta.ResourceVersion = ""
			newMregistration.ObjectMeta.ResourceVersion = ""

			return !cmp.Equal(oldMRegistration, newMregistration)
		},
	}
}
