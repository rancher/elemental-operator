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
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
)

// ManagedOSChangelogReconciler reconciles a ManagedOSChangelog object
type ManagedOSChangelogReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	WorkerPodImage           string
	WorkerPodImagePullPolicy corev1.PullPolicy
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedoschangelogs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedoschangelogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedoschangelogs/finalizers,verbs=update
//
// +kubebuilder:rbac:groups="",namespace=fleet-default,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",namespace=fleet-default,resources=pods/status,verbs=get
// +kubebuilder:rbac:groups="",namespace=fleet-default,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",namespace=fleet-default,resources=services/status,verbs=get

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedOSChangelogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elementalv1.ManagedOSChangelog{}).
		Owns(&corev1.Pod{}).
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

	// Init the Ready condition as we want it to be the first one displayed
	if readyCond := meta.FindStatusCondition(changelog.Status.Conditions, elementalv1.ReadyCondition); readyCond == nil {
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:   elementalv1.ReadyCondition,
			Reason: elementalv1.ResourcesNotCreatedYet,
			Status: metav1.ConditionUnknown,
		})
	}

	channel := &elementalv1.ManagedOSVersionChannel{}

	if err := r.reconcileManagedOSChangelogOwner(ctx, changelog, channel); err != nil {
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.SetOwnerFailureReason,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to set managedoschangelog owner: %w", err)
	}

	if err := r.reconcileWorkerPod(ctx, changelog, channel); err != nil {
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.PodCreationFailureReason,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to reconcile pod: %w", err)
	}

	if err := r.reconcileWorkerService(ctx, changelog); err != nil {
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.ServiceCreationFailureReason,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to create service: %w", err)
	}

	meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
		Type:    elementalv1.ReadyCondition,
		Reason:  elementalv1.ResourcesSuccessfullyCreatedReason,
		Status:  metav1.ConditionTrue,
		Message: "resources created successfully",
	})

	return ctrl.Result{}, nil
}

func (r *ManagedOSChangelogReconciler) reconcileManagedOSChangelogOwner(ctx context.Context,
	changelog *elementalv1.ManagedOSChangelog, channel *elementalv1.ManagedOSVersionChannel) error {
	if err := r.Get(ctx, types.NamespacedName{
		Name:      changelog.Spec.ManagedOSVersionChannelRef.Name,
		Namespace: changelog.Spec.ManagedOSVersionChannelRef.Namespace,
	}, channel); err != nil {
		return err
	}

	for _, o := range channel.OwnerReferences {
		if o.UID == channel.UID {
			return nil
		}
	}

	return controllerutil.SetOwnerReference(channel, changelog, r.Scheme)
}

func (r *ManagedOSChangelogReconciler) reconcileWorkerPod(ctx context.Context,
	changelog *elementalv1.ManagedOSChangelog, channel *elementalv1.ManagedOSVersionChannel) error {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling Pod")

	podChannelImg := ""
	if val, ok := channel.Spec.Options["image"]; !ok {
		return fmt.Errorf("managedosversionchannel 'image' option is missing")
	} else {
		// drop quotes from the image val
		podChannelImg = regexp.MustCompile(`^"(.*)"$`).ReplaceAllString(string(val.Raw), `$1`)
	}

	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: changelog.Name, Namespace: changelog.Namespace}, pod)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		logger.V(5).Info("Pod already created")
		return r.updateStatusFromWorkerPod(ctx, changelog, pod)
	}

	logger.V(5).Info("Creating pod")

	pod = r.fillWorkerPod(changelog, podChannelImg)
	if err := controllerutil.SetControllerReference(changelog, pod, r.Scheme); err != nil {
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.WorkerPodReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.WorkerPodNotStartedReason,
			Message: err.Error(),
		})
		return err
	}

	if err := r.Create(ctx, pod); err != nil && !apierrors.IsAlreadyExists(err) {
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.WorkerPodReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.WorkerPodNotStartedReason,
			Message: err.Error(),
		})
		return err
	}

	meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
		Type:    elementalv1.WorkerPodReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  elementalv1.WorkerPodInitReason,
		Message: "worker pod started",
	})
	return nil
}

func (r *ManagedOSChangelogReconciler) reconcileWorkerService(ctx context.Context,
	changelog *elementalv1.ManagedOSChangelog) error {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling Service")

	deadlineElapsed := false
	if readyCondition := meta.FindStatusCondition(changelog.Status.Conditions, elementalv1.WorkerPodReadyCondition); readyCondition != nil &&
		readyCondition.Status == metav1.ConditionTrue &&
		readyCondition.Reason == elementalv1.WorkerPodDeadline {
		deadlineElapsed = true
	}

	foundSvc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: changelog.Name, Namespace: changelog.Namespace}, foundSvc)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if err == nil {
		logger.V(5).Info("Service already there")

		// ensure the service was created by us
		for _, owner := range foundSvc.GetOwnerReferences() {
			if owner.UID == changelog.UID {
				if deadlineElapsed {
					logger.V(5).Info("Worker pod deadline passed, delete associated service", "service", foundSvc.Name)
					if err := r.Delete(ctx, foundSvc); err != nil {
						return fmt.Errorf("failed to delete service %s: %w", foundSvc.Name, err)
					}
				}
				return nil
			}
		}

		return fmt.Errorf("service already exists and was not created by this controller")
	}

	if deadlineElapsed {
		return nil
	}

	logger.V(5).Info("Creating service")

	// TODO: move fillBuildImageService() to a shared package (actually is in the SeedImage controller code)
	service := fillBuildImageService(changelog.Name, changelog.Namespace)

	if err := controllerutil.SetControllerReference(changelog, service, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, service); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (r *ManagedOSChangelogReconciler) updateStatusFromWorkerPod(ctx context.Context,
	changelog *elementalv1.ManagedOSChangelog, pod *corev1.Pod) error {
	logger := ctrl.LoggerFrom(ctx)

	if !pod.DeletionTimestamp.IsZero() {
		logger.V(5).Info("Wait the worker Pod to terminate")
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.WorkerPodReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.WorkerPodNotStartedReason,
			Message: "wait old worker Pod termination",
		})
		return nil
	}

	if meta.IsStatusConditionTrue(changelog.Status.Conditions, elementalv1.WorkerPodReadyCondition) &&
		pod.Status.Phase != corev1.PodSucceeded {
		return nil
	}

	logger.V(5).Info("Sync WorkerPodReady condition from worker Pod", "pod-phase", pod.Status.Phase)

	switch pod.Status.Phase {
	case corev1.PodPending:
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.WorkerPodReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.WorkerPodInitReason,
			Message: "changelog retrieval ongoing",
		})
		return nil
	case corev1.PodRunning:
		rancherURL, err := r.getRancherServerAddress(ctx)
		if err != nil {
			errMsg := fmt.Errorf("failed to get Rancher Server Address: %w", err)
			meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
				Type:    elementalv1.WorkerPodReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  elementalv1.WorkerPodCompletionFailureReason,
				Message: errMsg.Error(),
			})
			return errMsg
		}
		// Let's check here we have an associated Service, so the changelog could be displayed
		if err := r.Get(ctx, types.NamespacedName{Name: changelog.Name, Namespace: changelog.Namespace},
			&corev1.Service{}); err != nil {
			errMsg := fmt.Errorf("failed to get associated service: %w", err)
			meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
				Type:    elementalv1.WorkerPodReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  elementalv1.WorkerPodCompletionFailureReason,
				Message: errMsg.Error(),
			})
			return errMsg
		}
		token := changelog.Spec.ManagedOSVersion
		changelog.Status.ChangelogURL = fmt.Sprintf("https://%s/elemental/changelog/%s/", rancherURL, token)
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.WorkerPodReadyCondition,
			Status:  metav1.ConditionTrue,
			Reason:  elementalv1.WorkerPodCompletionSuccessReason,
			Message: "changelog available",
		})
		return nil
	case corev1.PodFailed:
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.WorkerPodReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.WorkerPodCompletionFailureReason,
			Message: "worker pod failed",
		})
		return nil
	case corev1.PodSucceeded:
		if err := r.Delete(ctx, pod); err != nil {
			errMsg := fmt.Errorf("failed to delete worker pod: %w", err)
			meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
				Type:    elementalv1.WorkerPodReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  elementalv1.WorkerPodDeadline,
				Message: errMsg.Error(),
			})
			return errMsg
		}
		changelog.Status.ChangelogURL = ""
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.WorkerPodReadyCondition,
			Status:  metav1.ConditionTrue,
			Reason:  elementalv1.WorkerPodDeadline,
			Message: "worker pod deadline elapsed",
		})
		return nil
	default:
		meta.SetStatusCondition(&changelog.Status.Conditions, metav1.Condition{
			Type:    elementalv1.WorkerPodReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  elementalv1.WorkerPodUnknown,
			Message: fmt.Sprintf("pod phase %s", pod.Status.Phase),
		})
		return nil

	}
}

func (r *ManagedOSChangelogReconciler) fillWorkerPod(changelog *elementalv1.ManagedOSChangelog,
	channelImage string) *corev1.Pod {
	name := changelog.Name
	namespace := changelog.Namespace
	image := r.WorkerPodImage
	imagePullPolicy := r.WorkerPodImagePullPolicy
	deadline := changelog.Spec.LifetimeMinutes
	diskMaxSize, _ := resource.ParseQuantity("100M")
	extractCmd := []string{
		"cp",
		fmt.Sprintf("/changelogs/%s.updates.log", changelog.Spec.ManagedOSVersion),
		"/data/",
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app.kubernetes.io/name": name},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:            "extract",
					Image:           channelImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         []string{"/bin/busybox"},
					Args:            extractCmd,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "changelog-storage",
							MountPath: "/data",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:            "serve",
					Image:           image,
					ImagePullPolicy: imagePullPolicy,
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 80,
						},
					},
					Args: []string{"-d", "/srv", "-t", fmt.Sprintf("%d", deadline*60)},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "changelog-storage",
							MountPath: "/srv",
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
			Volumes: []corev1.Volume{
				{
					Name: "changelog-storage",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: &diskMaxSize,
						},
					},
				},
			},
		},
	}
	return pod
}

func (r *ManagedOSChangelogReconciler) getRancherServerAddress(ctx context.Context) (string, error) {
	logger := ctrl.LoggerFrom(ctx)

	setting := &managementv3.Setting{}
	if err := r.Get(ctx, types.NamespacedName{Name: "server-url"}, setting); err != nil {
		return "", fmt.Errorf("failed to get server url setting: %w", err)
	}

	if setting.Value == "" {
		err := fmt.Errorf("server-url is not set")
		logger.Error(err, "can't get server-url")
		return "", err
	}

	return strings.TrimPrefix(setting.Value, "https://"), nil
}
