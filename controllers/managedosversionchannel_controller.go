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
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/syncer"
	"github.com/rancher/elemental-operator/pkg/util"
)

const (
	baseRateTime               = 1 * time.Second
	maxDelayTime               = 256 * time.Second
	displayContainer           = "display"
	defaultMinTimeBetweenSyncs = 60 * time.Second
)

// ManagedOSVersionChannelReconciler reconciles a ManagedOSVersionChannel object.
type ManagedOSVersionChannelReconciler struct {
	client.Client
	kcl           *kubernetes.Clientset
	OperatorImage string
	// syncerProvider is mostly an interface to facilitate unit tests
	syncerProvider syncer.Provider
	// minTimeBetweenSyncs is mostly added here to be configurable in tests
	minTimeBetweenSyncs time.Duration
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversionchannels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversionchannels/status,verbs=get;update;patch;list
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversions,verbs=get;list;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversions/status,verbs=get;update;patch

func (r *ManagedOSVersionChannelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	var err error

	if r.syncerProvider == nil {
		r.syncerProvider = syncer.DefaultProvider{}
	}
	if r.minTimeBetweenSyncs == 0 {
		r.minTimeBetweenSyncs = defaultMinTimeBetweenSyncs
	}
	r.kcl, err = kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(baseRateTime, maxDelayTime),
		}).
		For(&elementalv1.ManagedOSVersionChannel{}).
		Owns(&corev1.Pod{}).
		WithEventFilter(filterChannelEvents()).
		Complete(r)
}

func (r *ManagedOSVersionChannelReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) { //nolint:dupl
	logger := ctrl.LoggerFrom(ctx)

	managedOSVersionChannel := &elementalv1.ManagedOSVersionChannel{}
	err := r.Get(ctx, req.NamespacedName, managedOSVersionChannel)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(log.DebugDepth).Info("Object was not found, not an error")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get managed OS version channel object: %w", err)
	}

	// Ensure we patch the latest version otherwise we could erratically overlap with other controllers (e.g. backup and restore)
	patchBase := client.MergeFromWithOptions(managedOSVersionChannel.DeepCopy(), client.MergeFromWithOptimisticLock{})

	// We have to sanitize the conditions because old API definitions didn't have proper validation.
	managedOSVersionChannel.Status.Conditions = util.RemoveInvalidConditions(managedOSVersionChannel.Status.Conditions)

	// Collect errors as an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, managedOSVersionChannel)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reconciling managed OS version channel object: %w", err))
	}

	managedosversionchannelStatusCopy := managedOSVersionChannel.Status.DeepCopy() // Patch call will erase the status

	if err := r.Patch(ctx, managedOSVersionChannel, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch managed OS version channel object: %w", err))
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
		msg := "spec.Type can't be empty"
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.InvalidConfigurationReason,
			Status:  metav1.ConditionFalse,
			Message: msg,
		})
		logger.Error(nil, msg)
		return ctrl.Result{}, nil
	}

	interval, err := time.ParseDuration(managedOSVersionChannel.Spec.SyncInterval)
	if err != nil { // TODO: This should be part of validation webhook and moved out of the controller
		msg := "spec.SyncInterval is not parseable by time.ParseDuration"
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.InvalidConfigurationReason,
			Status:  metav1.ConditionFalse,
			Message: msg,
		})
		logger.Error(nil, msg)
		return ctrl.Result{}, nil
	}

	sync, err := r.syncerProvider.NewOSVersionsSyncer(managedOSVersionChannel.Spec, r.OperatorImage)
	if err != nil { // TODO: This should be part of validation webhook and moved out of the controller
		msg := "spec.Type is not supported"
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.InvalidConfigurationReason,
			Status:  metav1.ConditionFalse,
			Message: msg,
		})
		logger.Error(nil, msg)
		return ctrl.Result{}, nil
	}

	readyCondition := meta.FindStatusCondition(managedOSVersionChannel.Status.Conditions, elementalv1.ReadyCondition)
	if readyCondition == nil {
		// First reconcile loop does not have a ready condition
		return ctrl.Result{}, r.createSyncerPod(ctx, managedOSVersionChannel, sync)
	}

	lastSync := managedOSVersionChannel.Status.LastSyncedTime
	if lastSync != nil && lastSync.Add(r.minTimeBetweenSyncs).After(time.Now()) {
		logger.Info("synchronization already done shortly before", "lastSync", lastSync)
		return ctrl.Result{}, nil
	}

	pod := &corev1.Pod{}
	err = r.Get(ctx, client.ObjectKey{
		Namespace: managedOSVersionChannel.Namespace,
		Name:      managedOSVersionChannel.Name,
	}, pod)
	if err != nil {
		if apierrors.IsNotFound(err) && readyCondition.Reason != elementalv1.SyncingReason {
			return ctrl.Result{}, r.createSyncerPod(ctx, managedOSVersionChannel, sync)
		}
		logger.Error(err, "failed getting pod resource", "pod", pod.Name)
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToSyncReason,
			Status:  metav1.ConditionFalse,
			Message: "failed channel synchronization",
		})
		return ctrl.Result{}, err
	}

	// Sometimes during upgrade the new elemental-channel will be created using
	// the old logic. This checks if this happened we recreate the syncer pod.
	if pod.Spec.Containers[0].Image != r.OperatorImage {
		_ = r.Delete(ctx, pod)
		return ctrl.Result{}, r.createSyncerPod(ctx, managedOSVersionChannel, sync)
	}

	return r.handleSyncPod(ctx, pod, managedOSVersionChannel, interval), nil
}

// handleSyncPod is the method responcible to manage the lifecycle of the channel synchronization pod
func (r *ManagedOSVersionChannelReconciler) handleSyncPod(ctx context.Context, pod *corev1.Pod, ch *elementalv1.ManagedOSVersionChannel, interval time.Duration) ctrl.Result {
	var data []byte
	var err error

	logger := ctrl.LoggerFrom(ctx)

	defer func() {
		r.handleFailedSync(ctx, pod, ch, err)
	}()

	// Once the Pod already started we do not return error in case of failure so synchronization is not retriggered, we just set
	// to false readyness condition and schedule a new resynchronization for the next interval
	switch pod.Status.Phase {
	case corev1.PodPending, corev1.PodRunning:
		logger.Info("Waiting until the pod is on succeeded state ", "pod", pod.Name)
		meta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.SyncingReason,
			Status:  metav1.ConditionFalse,
			Message: "ongoing channel synchronization",
		})
		return ctrl.Result{RequeueAfter: r.minTimeBetweenSyncs / 2}
	case corev1.PodSucceeded:
		data, err = r.syncerProvider.ReadPodLogs(ctx, r.kcl, pod, displayContainer)
		if err != nil {
			return ctrl.Result{RequeueAfter: interval}
		}
		err = r.createManagedOSVersions(ctx, ch, data)
		if err != nil {
			return ctrl.Result{RequeueAfter: interval}
		}
		if err = r.Delete(ctx, pod); err != nil {
			logger.Error(err, "could not delete the pod", "pod", pod.Name)
		}
		logger.Info("Channel data loaded")
		meta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.SyncedReason,
			Status:  metav1.ConditionTrue,
			Message: "successfully loaded channel data",
		})
		now := metav1.Now()
		ch.Status.LastSyncedTime = &now
		return ctrl.Result{RequeueAfter: interval}
	default:
		// Any other phase (failed or unknown) is considered an error
		err = fmt.Errorf("synchronization pod failed")
		return ctrl.Result{RequeueAfter: interval}
	}
}

// handleFailedSync deletes the pod, produces error log traces and sets the failure state if error is not nil, otherwise sets a success state
func (r *ManagedOSVersionChannelReconciler) handleFailedSync(ctx context.Context, pod *corev1.Pod, ch *elementalv1.ManagedOSVersionChannel, err error) {
	if err != nil {
		logger := ctrl.LoggerFrom(ctx)
		now := metav1.Now()
		ch.Status.LastSyncedTime = &now
		logger.Error(err, "failed handling syncer pod", "pod", pod.Name)
		meta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToSyncReason,
			Status:  metav1.ConditionFalse,
			Message: "failed channel synchronization",
		})
		if dErr := r.Delete(ctx, pod); dErr != nil {
			logger.Error(dErr, "could not delete the pod", "pod", pod.Name)
		}
	}
}

// createManagedOSVersions unmarshals managedOSVersions from a byte array and creates them.
func (r *ManagedOSVersionChannelReconciler) createManagedOSVersions(ctx context.Context, ch *elementalv1.ManagedOSVersionChannel, data []byte) error {
	logger := ctrl.LoggerFrom(ctx)

	vers := []elementalv1.ManagedOSVersion{}
	err := json.Unmarshal(data, &vers)
	if err != nil {
		logger.Error(err, "Failed unmarshalling managedOSVersions")
		return err
	}

	curVersions := r.getAllOwnedManagedOSVersions(ctx, client.ObjectKey{
		Name:      ch.Name,
		Namespace: ch.Namespace,
	})

	var errs []error

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
		vcpy.ObjectMeta.Labels = map[string]string{
			elementalv1.ElementalManagedOSVersionChannelLabel: ch.Name,
		}

		if ch.Spec.UpgradeContainer != nil {
			vcpy.Spec.UpgradeContainer = ch.Spec.UpgradeContainer
		}

		if cv, ok := curVersions[v.Name]; ok {
			patchBase := client.MergeFrom(cv.DeepCopy())
			cv.Spec = vcpy.Spec
			err = r.Patch(ctx, cv, patchBase)
			if err != nil {
				logger.Error(err, "failed to patch a managedosversion", "name", cv.Name)
				errs = append(errs, err)
			} else {
				logger.Info("patched managedOSVersion", "name", cv.Name)
			}
		} else if err = r.Create(ctx, vcpy); err != nil {
			if apierrors.IsAlreadyExists(err) {
				logger.Error(err, "already existing managedOSVersion", "name", vcpy.Name)
			} else {
				logger.Error(err, "failed to create a managedosversion", "name", vcpy.Name)
			}
			errs = append(errs, err)
		} else {
			logger.Info("managedOSVersion created", "name", vcpy.Name)
		}
	}

	return errorutils.NewAggregate(errs)
}

// getAllOwnedManagedOSVersions returns a map of all ManagedOSVersions labeled with the given channel, resource name is used as the map key
func (r *ManagedOSVersionChannelReconciler) getAllOwnedManagedOSVersions(ctx context.Context, chKey client.ObjectKey) map[string]*elementalv1.ManagedOSVersion {
	logger := ctrl.LoggerFrom(ctx)
	versions := &elementalv1.ManagedOSVersionList{}
	result := map[string]*elementalv1.ManagedOSVersion{}

	err := r.List(ctx, versions, client.InNamespace(chKey.Namespace), client.MatchingLabels(map[string]string{
		elementalv1.ElementalManagedOSVersionChannelLabel: chKey.Name,
	}))
	if err != nil {
		// only log error and return an empty map
		logger.Error(err, "failed listing existing versions from channel")
		return result
	}

	for _, ver := range versions.Items {
		result[ver.Name] = ver.DeepCopy()
	}

	return result
}

// createSyncerPod creates the pod according to the managed OS version channel configuration
func (r *ManagedOSVersionChannelReconciler) createSyncerPod(ctx context.Context, ch *elementalv1.ManagedOSVersionChannel, sync syncer.Syncer) error {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Launching syncer", "pod", ch.Name)

	serviceAccount := false
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ch.Name,
			Namespace: ch.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: elementalv1.GroupVersion.String(),
					Kind:       "ManagedOSVersionChannel",
					Name:       ch.Name,
					UID:        ch.UID,
					Controller: pointer.Bool(true),
				},
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			AutomountServiceAccountToken: &serviceAccount,
			InitContainers:               sync.ToContainers(),
			Volumes: []corev1.Volume{{
				Name:         "output",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			}},
			Containers: []corev1.Container{{
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "output",
					MountPath: sync.GetMountPath(),
				}},
				Name:    displayContainer,
				Image:   r.OperatorImage,
				Command: []string{},
				Args:    []string{"display", "--file", sync.GetOutputFile()},
			}},
		},
	}

	err := r.Create(ctx, pod)
	if err != nil {
		logger.Error(err, "Failed creating pod", "pod", ch.Name)
		// Could fail due to previous leftovers
		_ = r.Delete(ctx, pod)
		meta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToCreatePodReason,
			Status:  metav1.ConditionFalse,
			Message: "failed creating synchronization pod",
		})
		return err
	}

	meta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
		Type:    elementalv1.ReadyCondition,
		Reason:  elementalv1.SyncingReason,
		Status:  metav1.ConditionFalse,
		Message: "started synchronization pod",
	})
	return nil
}

func filterChannelEvents() predicate.Funcs {
	return predicate.Funcs{
		// Process only new generation updates for channels and new phase for pods updates
		UpdateFunc: func(e event.UpdateEvent) bool {
			logger := ctrl.LoggerFrom(context.Background())

			if oldChannel, ok := e.ObjectOld.(*elementalv1.ManagedOSVersionChannel); ok {
				newChannel := e.ObjectNew.(*elementalv1.ManagedOSVersionChannel)
				update := newChannel.GetGeneration() != oldChannel.GetGeneration()
				logger.V(log.DebugDepth).Info("Channel update event", "new generation", update)
				return update
			}
			if oldPod, ok := e.ObjectOld.(*corev1.Pod); ok {
				newPod := e.ObjectNew.(*corev1.Pod)
				if newPod.Status.Phase != oldPod.Status.Phase {
					logger.V(log.DebugDepth).Info("Processing pod update", "Pod", newPod.Name, "Phase", newPod.Status.Phase)
					return true
				}
				logger.V(log.DebugDepth).Info("Ignoring pod update", "Pod", newPod.Name, "Phase", newPod.Status.Phase)
				return false
			}
			// Return true in case it watches other types
			logger.V(log.DebugDepth).Info("Processing update event", "Obj", e.ObjectNew.GetName())
			return true
		},
		// Ignore pods deletion
		DeleteFunc: func(e event.DeleteEvent) bool {
			logger := ctrl.LoggerFrom(context.Background())

			if _, ok := e.Object.(*corev1.Pod); ok {
				return false
			}
			// Return true in case it watches other types
			logger.V(log.DebugDepth).Info("Processing delete event", "Obj", e.Object.GetName())
			return true
		},
		// Ignore generic pod events
		GenericFunc: func(e event.GenericEvent) bool {
			logger := ctrl.LoggerFrom(context.Background())

			if _, ok := e.Object.(*corev1.Pod); ok {
				return false
			}
			// Return true in case it watches other types
			logger.V(log.DebugDepth).Info("Processing generic event", "Obj", e.Object.GetName())
			return true
		},
	}
}
