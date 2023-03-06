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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/pointer"
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
		return r.reconcileDelete(ctx, seedImg)
	}

	controllerutil.AddFinalizer(seedImg, elementalv1.SeedImageFinalizer)

	if err := r.createBuildImagePod(ctx, seedImg); err != nil {
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.PodCreationFailureReason,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to create pod: %w", err)
	}

	if err := r.createBuildImageService(ctx, seedImg); err != nil {
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.ServiceCreationFailureReason,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to create service: %w", err)
	}

	meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
		Type:   elementalv1.ReadyCondition,
		Reason: elementalv1.ResourcesSuccessfullyCreatedReason,
		Status: metav1.ConditionTrue,
	})

	return ctrl.Result{}, nil
}

func (r *SeedImageReconciler) reconcileDelete(ctx context.Context, seedImg *elementalv1.SeedImage) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Deleting pod and service resources")

	if err := r.Client.Delete(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: seedImg.Namespace,
			Name:      seedImg.Name,
		}}); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to delete pod: %w", err)
	}

	if err := r.Client.Delete(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: seedImg.Namespace,
			Name:      seedImg.Name,
		}}); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to delete service: %w", err)
	}

	controllerutil.RemoveFinalizer(seedImg, elementalv1.SeedImageFinalizer)

	return ctrl.Result{}, nil
}

func (r *SeedImageReconciler) createBuildImagePod(ctx context.Context, seedImg *elementalv1.SeedImage) error {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling POD resources")

	mRegistration := &elementalv1.MachineRegistration{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      seedImg.Spec.MachineRegistrationRef.Name,
		Namespace: seedImg.Spec.MachineRegistrationRef.Namespace,
	}, mRegistration); err != nil {
		return err
	}

	logger.V(5).Info("Creating pod")

	pod := fillBuildImagePod(seedImg.Name, seedImg.Namespace,
		seedImg.Spec.BaseImage, mRegistration.Status.RegistrationURL)
	pod.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: elementalv1.GroupVersion.String(),
			Kind:       "SeedImage",
			Name:       seedImg.Name,
			UID:        seedImg.UID,
			Controller: pointer.Bool(true),
		},
	}

	if err := r.Create(ctx, pod); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func fillBuildImagePod(name, namespace, seedImgURL, regURL string) *corev1.Pod {
	const buildImage = "registry.opensuse.org/isv/rancher/elemental/stable/teal53/15.4/rancher/elemental-builder-image/5.3:latest"
	const serveImage = "registry.opensuse.org/opensuse/nginx:latest"
	const volLim = 4 * 1024 * 1024 * 1024 // 4 GiB
	const volRes = 2 * 1024 * 1024 * 1024 // 2 GiB

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app.kubernetes.io/name": name},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:  "build",
					Image: buildImage,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceEphemeralStorage: *resource.NewQuantity(volLim, resource.BinarySI),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceEphemeralStorage: *resource.NewQuantity(volRes, resource.BinarySI),
						},
					},
					Command: []string{"/bin/bash", "-c"},
					Args: []string{
						fmt.Sprintf("%s; %s; %s",
							fmt.Sprintf("curl -Lo base.img %s", seedImgURL),
							fmt.Sprintf("curl -ko reg.yaml %s", regURL),
							"xorriso -indev base.img -outdev /iso/elemental.iso -map reg.yaml /reg.yaml -boot_image any replay"),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "iso-storage",
							MountPath: "/iso",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "serve",
					Image: serveImage,
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 80,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "iso-storage",
							MountPath: "/srv/www/htdocs",
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
			Volumes: []corev1.Volume{
				{
					Name: "iso-storage",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: resource.NewQuantity(volLim, resource.BinarySI),
						},
					},
				},
			},
		},
	}
	return pod
}

func (r *SeedImageReconciler) createBuildImageService(ctx context.Context, seedImg *elementalv1.SeedImage) error {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling Service resources")

	logger.V(5).Info("Creating service")

	service := fillBuildImageService(seedImg.Name, seedImg.Namespace)
	service.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: elementalv1.GroupVersion.String(),
			Kind:       "SeedImage",
			Name:       seedImg.Name,
			UID:        seedImg.UID,
			Controller: pointer.Bool(true),
		},
	}

	if err := r.Create(ctx, service); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func fillBuildImageService(name, namespace string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": name},
			Ports: []corev1.ServicePort{
				{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
				},
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}

	return service
}
