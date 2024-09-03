/*
Copyright © 2022 - 2024 SUSE LLC

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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
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
	baseRateTime          = 4 * time.Second
	maxDelayTime          = 512 * time.Second
	pollingTime           = 20 * time.Second
	displayContainer      = "display"
	maxConscutiveFailures = 4
)

// ManagedOSVersionChannelReconciler reconciles a ManagedOSVersionChannel object.
type ManagedOSVersionChannelReconciler struct {
	client.Client
	kcl           *kubernetes.Clientset
	OperatorImage string
	// syncerProvider is mostly an interface to facilitate unit tests
	syncerProvider syncer.Provider
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

	if !managedOSVersionChannel.Spec.Enabled {
		logger.Info("Channel is disabled. Skipping sync.")
		curVersions := r.getAllOwnedManagedOSVersions(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		})
		for _, version := range curVersions {
			if err := r.deprecateVersion(ctx, *managedOSVersionChannel, version); err != nil {
				return ctrl.Result{}, fmt.Errorf("Deprecating ManagedOSVersion %s: %w", version.Name, err)
			}
		}
		if err := r.deleteSyncerPod(ctx, *managedOSVersionChannel); err != nil {
			return ctrl.Result{}, fmt.Errorf("deleting syncer pod: %w", err)
		}
		meta.SetStatusCondition(&managedOSVersionChannel.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.ChannelDisabledReason,
			Status:  metav1.ConditionTrue,
			Message: "Channel is disabled",
		})
		return ctrl.Result{}, nil
	}

	reachedNextInterval := false
	lastSync := managedOSVersionChannel.Status.LastSyncedTime
	if lastSync != nil && lastSync.Add(interval).Before(time.Now()) {
		reachedNextInterval = true
	}

	newGeneration := managedOSVersionChannel.Status.SyncedGeneration != managedOSVersionChannel.Generation
	readyCondition := meta.FindStatusCondition(managedOSVersionChannel.Status.Conditions, elementalv1.ReadyCondition)
	if readyCondition == nil || newGeneration || reachedNextInterval {
		// First reconcile loop for the given generation or reached the next interval
		managedOSVersionChannel.Status.FailedSynchronizationAttempts = 0
		managedOSVersionChannel.Status.SyncedGeneration = managedOSVersionChannel.Generation
		return ctrl.Result{}, r.createSyncerPod(ctx, managedOSVersionChannel, sync)
	}

	if readyCondition.Status == metav1.ConditionTrue {
		logger.Info("synchronization already done", "lastSync", lastSync)
		return ctrl.Result{RequeueAfter: time.Until(lastSync.Add(interval))}, nil
	}

	if managedOSVersionChannel.Status.FailedSynchronizationAttempts > maxConscutiveFailures {
		logger.Error(fmt.Errorf("stop retrying"), "sychronization failed consecutively too many times", "failed attempts", managedOSVersionChannel.Status.FailedSynchronizationAttempts)
		return ctrl.Result{RequeueAfter: time.Until(lastSync.Add(interval))}, nil
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
		managedOSVersionChannel.Status.FailedSynchronizationAttempts++
		return ctrl.Result{}, err
	}

	// Sometimes during upgrade the new elemental-channel will be created using
	// the old logic. This checks if this happened we recreate the syncer pod.
	if pod.Spec.Containers[0].Image != r.OperatorImage {
		_ = r.Delete(ctx, pod)
		return ctrl.Result{}, r.createSyncerPod(ctx, managedOSVersionChannel, sync)
	}

	return r.handleSyncPod(ctx, pod, managedOSVersionChannel, interval)
}

// handleSyncPod is the method responcible to manage the lifecycle of the channel synchronization pod
func (r *ManagedOSVersionChannelReconciler) handleSyncPod(ctx context.Context, pod *corev1.Pod, ch *elementalv1.ManagedOSVersionChannel, interval time.Duration) (ctrl.Result, error) {
	var data []byte
	var err error

	logger := ctrl.LoggerFrom(ctx)

	switch pod.Status.Phase {
	case corev1.PodPending, corev1.PodRunning:
		logger.Info("Waiting until the pod is on succeeded state ", "pod", pod.Name)
		meta.SetStatusCondition(&ch.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.SyncingReason,
			Status:  metav1.ConditionFalse,
			Message: "ongoing channel synchronization",
		})
		// Always requeue to prevent being stuck on this stage in case a pod phase change
		// is missed due to a race condition. This ensures we always check for the
		// synchronization finalization.
		return ctrl.Result{RequeueAfter: pollingTime}, nil
	case corev1.PodSucceeded:
		data, err = r.syncerProvider.ReadPodLogs(ctx, r.kcl, pod, displayContainer)
		if err != nil {
			return ctrl.Result{}, r.handleFailedSync(ctx, pod, ch, err)
		}
		now := metav1.Now()
		err = r.createManagedOSVersions(ctx, ch, data, now.Format(time.RFC3339))
		if err != nil {
			return ctrl.Result{}, r.handleFailedSync(ctx, pod, ch, err)
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
		ch.Status.FailedSynchronizationAttempts = 0
		ch.Status.LastSyncedTime = &now
		return ctrl.Result{RequeueAfter: interval}, nil
	default:
		// Any other phase (failed or unknown) is considered an error
		return ctrl.Result{}, r.handleFailedSync(ctx, pod, ch, fmt.Errorf("synchronization pod failed"))
	}
}

// handleFailedSync deletes the pod, produces error log traces and sets the failure state if error is not nil, otherwise sets a success state
func (r *ManagedOSVersionChannelReconciler) handleFailedSync(ctx context.Context, pod *corev1.Pod, ch *elementalv1.ManagedOSVersionChannel, err error) error {
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
	ch.Status.FailedSynchronizationAttempts++
	if dErr := r.Delete(ctx, pod); dErr != nil {
		logger.Error(dErr, "could not delete the pod", "pod", pod.Name)
	}
	return err
}

// createManagedOSVersions unmarshals managedOSVersions from a byte array and creates them.
func (r *ManagedOSVersionChannelReconciler) createManagedOSVersions(ctx context.Context, ch *elementalv1.ManagedOSVersionChannel, data []byte, syncTimestamp string) error {
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
				Controller: ptr.To(true),
			},
		}
		vcpy.ObjectMeta.Labels = map[string]string{
			elementalv1.ElementalManagedOSVersionChannelLabel: ch.Name,
		}
		vcpy.ObjectMeta.Annotations = map[string]string{
			elementalv1.ElementalManagedOSVersionChannelLastSyncAnnotation: syncTimestamp,
		}

		if ch.Spec.UpgradeContainer != nil {
			vcpy.Spec.UpgradeContainer = ch.Spec.UpgradeContainer
		}

		if len(ch.Spec.Registry) > 0 {
			registry := ch.Spec.Registry
			urlField := ""
			switch {
			case vcpy.IsContainerImage():
				urlField = "upgradeImage"
			case vcpy.IsISOImage():
				urlField = "uri"
			default:
				err := fmt.Errorf("unexpected ManagedOSVersion type %s", vcpy.Spec.Type)
				logger.Error(err, "failed to concatenate managedosversion with registry", "registry", ch.Spec.Registry)
				return err
			}

			val := strings.Trim(string(vcpy.Spec.Metadata[urlField].Raw), "\"")
			if !strings.HasSuffix(ch.Spec.Registry, "/") && !strings.HasPrefix(val, "/") {
				registry += "/"
			}
			vcpy.Spec.Metadata[urlField] = runtime.RawExtension{
				Raw: []byte(fmt.Sprintf("\"%s%s\"", registry, val)),
			}
		}

		if cv, ok := curVersions[v.Name]; ok {
			patchBase := client.MergeFrom(cv.DeepCopy())
			cv.Spec = vcpy.Spec
			cv.ObjectMeta.Labels = vcpy.ObjectMeta.Labels
			cv.ObjectMeta.Annotations = vcpy.ObjectMeta.Annotations
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

	if len(errs) > 0 {
		return errorutils.NewAggregate(errs)
	}

	// Flagging orphan versions
	for _, version := range curVersions {
		if lastSyncTime, found := version.Annotations[elementalv1.ElementalManagedOSVersionChannelLastSyncAnnotation]; !found || (lastSyncTime != syncTimestamp) {
			if err := r.deprecateVersion(ctx, *ch, version); err != nil {
				return fmt.Errorf("Deprecating ManagedOSVersion %s: %w", version.Name, err)
			}
		}
	}

	return nil
}

// deprecateVersion flags a ManagedOSVersion as orphan and if needed trigger its deletion.
func (r *ManagedOSVersionChannelReconciler) deprecateVersion(ctx context.Context, channel elementalv1.ManagedOSVersionChannel, version *elementalv1.ManagedOSVersion) error {
	logger := ctrl.LoggerFrom(ctx).WithValues("ManagedOSVersionChannel", channel.Name).WithValues("ManagedOSVersion", version.Name)
	logger.Info("ManagedOSVersion no longer synced through this channel")
	patchBase := client.MergeFrom(version.DeepCopy())
	if version.ObjectMeta.Annotations == nil {
		version.ObjectMeta.Annotations = map[string]string{}
	}
	version.ObjectMeta.Annotations[elementalv1.ElementalManagedOSVersionNoLongerSyncedAnnotation] = elementalv1.ElementalManagedOSVersionNoLongerSyncedValue
	if err := r.Patch(ctx, version, patchBase); err != nil {
		logger.Error(err, "Could not patch ManagedOSVersion as no longer in sync")
		return fmt.Errorf("deprecating ManagedOSVersion '%s': %w", version.Name, err)
	}
	if channel.Spec.DeleteNoLongerInSyncVersions {
		logger.Info("Auto-deleting no longer in sync ManagedOSVersion due to channel settings")
		if err := r.Delete(ctx, version); err != nil {
			logger.Error(err, "Could not auto-delete no longer in sync ManagedOSVersion")
			return fmt.Errorf("auto-deleting ManagedOSVersion '%s': %w", version.Name, err)
		}
	}
	return nil
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
					Controller: ptr.To(true),
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

	// If we don't update the LastSyncedTime on Pod creation, we will loop
	// creating/deleting the Pod after the ManagedOSVersionChannel.spec.syncInterval
	// see https://github.com/rancher/elemental-operator/issues/766
	now := metav1.Now()
	ch.Status.LastSyncedTime = &now

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
		ch.Status.FailedSynchronizationAttempts++
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

// deleteSyncerPod deletes the syncer pod if it exists
func (r *ManagedOSVersionChannelReconciler) deleteSyncerPod(ctx context.Context, channel elementalv1.ManagedOSVersionChannel) error {
	pod := &corev1.Pod{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: channel.Namespace,
		Name:      channel.Name,
	}, pod); apierrors.IsNotFound(err) {
		// Pod does not exist. Nothing to do.
		return nil
	} else if err != nil {
		return fmt.Errorf("getting pod: %w", err)
	}
	if err := r.Delete(ctx, pod); err != nil {
		return fmt.Errorf("deleting pod: %w", err)
	}
	return nil
}

// filterChannelEvents is a method that filters reconcile requests events for the channels reconciler.
// ManagedOSVersionChannelReconciler watches channels and owned pods. This filter ignores pod
// create/delete/generic events and only reacts on pod phase updates. Channel update events are
// only reconciled if the update includes a new generation of the resource, all other events are not
// filtered.
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
		// Ignore pods creation
		CreateFunc: func(e event.CreateEvent) bool {
			logger := ctrl.LoggerFrom(context.Background())

			if _, ok := e.Object.(*corev1.Pod); ok {
				return false
			}
			// Return true in case it watches other types
			logger.V(log.DebugDepth).Info("Processing create event", "Obj", e.Object.GetName())
			return true
		},
	}
}
