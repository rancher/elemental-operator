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
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/v2/pkg/randomtoken"
	"github.com/rancher/yip/pkg/schema"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/util"
)

type SeedImageReconciler struct {
	client.Client
	SeedImageImage           string
	SeedImageImagePullPolicy corev1.PullPolicy
}

const (
	configMapKeyRegistration    = "registration"
	configMapKeyCloudConfig     = "cloud-config"
	configMapKeyResetConfig     = "reset-config"
	configMapKeyDevice          = "device"
	configMapKeyRegistrationURL = "registration-url"
	configMapKeyBaseImage       = "base-image"
	configMapKeyOutputName      = "output-name"
)

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=seedimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=seedimages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=machineregistrations,verbs=get;watch;list
// TODO: restrict access to resources to the required namespace only:
//   https://github.com/rancher/elemental-operator/issues/457
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

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
		errs = append(errs, fmt.Errorf("failed to patch seedimage object: %w", err))
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

	// RetriggerBuild resets the conditions and the buider pod: the controller
	// will reconcile anew all the resources in the following loop
	if seedImg.Spec.RetriggerBuild {
		seedImg.Status.Conditions = []metav1.Condition{}
		seedImg.Spec.RetriggerBuild = false
		return ctrl.Result{}, r.deleteChildResources(ctx, seedImg)
	}

	// Init the Ready condition as we want it to be the first one displayed
	if readyCond := meta.FindStatusCondition(seedImg.Status.Conditions, elementalv1.ReadyCondition); readyCond == nil {
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:   elementalv1.ReadyCondition,
			Reason: elementalv1.ResourcesNotCreatedYet,
			Status: metav1.ConditionUnknown,
		})
	}

	mRegistration := &elementalv1.MachineRegistration{}

	if err := r.reconcileSeedImageOwner(ctx, seedImg, mRegistration); err != nil {
		errMsg := fmt.Errorf("failed to set seedimage owner: %w", err)
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.SetOwnerFailureReason,
			Message: errMsg.Error(),
		})
		return ctrl.Result{}, errMsg
	}

	if err := r.reconcileBuildImagePod(ctx, seedImg, mRegistration); err != nil {
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.PodCreationFailureReason,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to reconcile pod: %w", err)
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
		Type:    elementalv1.ReadyCondition,
		Reason:  elementalv1.ResourcesSuccessfullyCreatedReason,
		Status:  metav1.ConditionTrue,
		Message: "resources created successfully",
	})

	return ctrl.Result{}, nil
}

func (r *SeedImageReconciler) reconcileSeedImageOwner(ctx context.Context, seedImg *elementalv1.SeedImage, mRegistration *elementalv1.MachineRegistration) error {
	if err := r.Get(ctx, types.NamespacedName{
		Name:      seedImg.Spec.MachineRegistrationRef.Name,
		Namespace: seedImg.Spec.MachineRegistrationRef.Namespace,
	}, mRegistration); err != nil {
		return err
	}

	for _, o := range seedImg.OwnerReferences {
		if o.UID == mRegistration.UID {
			return nil
		}
	}

	return controllerutil.SetOwnerReference(mRegistration, seedImg, r.Scheme())
}

func (r *SeedImageReconciler) reconcileBuildImagePod(ctx context.Context, seedImg *elementalv1.SeedImage, mRegistration *elementalv1.MachineRegistration) error {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling Pod resources")

	seedImgCondition := meta.FindStatusCondition(seedImg.Status.Conditions, elementalv1.SeedImageConditionReady)
	seedImgConditionMet := false
	if seedImgCondition != nil && seedImgCondition.Status == metav1.ConditionTrue {
		seedImgConditionMet = true
	}

	if seedImgConditionMet && seedImgCondition.Reason == elementalv1.SeedImageBuildDeadline {
		logger.V(5).Info("Seed image deadline passed, skip")
		return nil
	}

	if err := r.reconcileConfigMapObject(ctx, seedImg, mRegistration); err != nil {
		return err
	}

	podName := seedImg.Name
	podNamespace := seedImg.Namespace
	podBaseImg := seedImg.Spec.BaseImage

	foundPod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: podNamespace}, foundPod)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if err == nil {
		logger.V(5).Info("Pod already there")

		// ensure the pod was created by us
		if !util.IsObjectOwned(&foundPod.ObjectMeta, seedImg.UID) {
			return fmt.Errorf("pod already exists and was not created by this controller")
		}

		// pod is already there and with the right configuration
		if foundPod.Annotations["elemental.cattle.io/base-image"] == podBaseImg {
			return r.updateStatusFromPod(ctx, seedImg, foundPod)
		}

		// Pod is not up-to-date: delete and recreate it
		if err := r.Delete(ctx, foundPod); err != nil {
			return fmt.Errorf("failed to delete old pod: %w", err)
		}
	}

	if seedImgConditionMet {
		logger.V(5).Info("SeedImageReady condition is met: skip Pod creation")
		return nil
	}
	// apierrors.IsNotFound(err) OR we just deleted the old Pod

	logger.V(5).Info("Creating pod")

	pod := fillBuildImagePod(seedImg, r.SeedImageImage, r.SeedImageImagePullPolicy)
	if err := controllerutil.SetControllerReference(seedImg, pod, r.Scheme()); err != nil {
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.SeedImageConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.SeedImageBuildNotStartedReason,
			Message: err.Error(),
		})
		return err
	}

	if err := r.Create(ctx, pod); err != nil && !apierrors.IsAlreadyExists(err) {
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.SeedImageConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.SeedImageBuildNotStartedReason,
			Message: err.Error(),
		})
		return err
	}

	meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
		Type:    elementalv1.SeedImageConditionReady,
		Status:  metav1.ConditionFalse,
		Reason:  elementalv1.SeedImageBuildOngoingReason,
		Message: "seed image build started",
	})
	return nil
}

func (r *SeedImageReconciler) reconcileConfigMapObject(ctx context.Context, seedImg *elementalv1.SeedImage, mRegistration *elementalv1.MachineRegistration) error {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling ConfigMap resources")

	podConfigMap := &corev1.ConfigMap{}

	regClientConf, err := mRegistration.GetClientRegistrationConfig(util.GetRancherCACert(ctx, r))
	if err != nil {
		return fmt.Errorf("failed processing registration config: %w", err)
	}

	regData, err := yaml.Marshal(regClientConf)
	if err != nil {
		return fmt.Errorf("failed marshalling registration config: %w", err)
	}

	resetConfig, err := serializeResetRegistrationYaml(regClientConf)
	if err != nil {
		return fmt.Errorf("failed marshalling reset machine registration: %w", err)
	}

	cloudConfig, err := util.MarshalCloudConfig(seedImg.Spec.CloudConfig)
	if err != nil {
		return fmt.Errorf("failed marshalling cloud-config: %w", err)
	}

	outputName := fmt.Sprintf("elemental-%s-%s.%s", seedImg.Spec.MachineRegistrationRef.Name, time.Now().Format(time.RFC3339), seedImg.Spec.Type)

	if err := r.Get(ctx, types.NamespacedName{
		Name:      seedImg.Name,
		Namespace: seedImg.Namespace,
	}, podConfigMap); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed fetching config map: %w", err)
		}
	} else {
		// ConfigMap is already there...
		if bytes.Equal(regData, podConfigMap.BinaryData["registration"]) &&
			bytes.Equal(resetConfig, podConfigMap.BinaryData["reset-config"]) &&
			bytes.Equal(cloudConfig, podConfigMap.BinaryData["cloud-config"]) {
			logger.V(5).Info("ConfigMap is up-to-date")
			return nil
		}
		logger.V(5).Info("ConfigMap is out of date", "configmap", podConfigMap.Namespace+"/"+podConfigMap.Name)

		outputName = podConfigMap.Data[configMapKeyOutputName]

		// ...but values are not up-to-date
		if err := r.Delete(ctx, podConfigMap); err != nil {
			return fmt.Errorf("failed to delete old config map: %w", err)
		}
	}

	// (re)create the configmap
	logger.V(5).Info("Create ConfigMap", "seedimage", seedImg.Namespace+"/"+seedImg.Name)

	binaryData := map[string][]byte{
		configMapKeyRegistration: regData,
		configMapKeyCloudConfig:  cloudConfig,
		configMapKeyResetConfig:  resetConfig,
	}

	data := map[string]string{
		configMapKeyRegistrationURL: mRegistration.Status.RegistrationURL,
		configMapKeyDevice:          mRegistration.Spec.Config.Elemental.Install.Device,
		configMapKeyBaseImage:       seedImg.Spec.BaseImage,
		configMapKeyOutputName:      outputName,
	}
	conf := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		},
		BinaryData: binaryData,
		Data:       data,
	}

	if err := controllerutil.SetControllerReference(seedImg, conf, r.Scheme()); err != nil {
		return fmt.Errorf("failed setting configmap ownership: %w", err)
	}

	if err := r.Create(ctx, conf); err != nil {
		return fmt.Errorf("failed to create registration config map: %w", err)
	}

	return nil
}

func (r *SeedImageReconciler) updateStatusFromPod(ctx context.Context, seedImg *elementalv1.SeedImage, foundPod *corev1.Pod) error {
	podName := seedImg.Name
	podNamespace := seedImg.Namespace

	logger := ctrl.LoggerFrom(ctx)

	if !foundPod.DeletionTimestamp.IsZero() {
		logger.V(5).Info("Wait the builder Pod to terminate")
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.SeedImageConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.SeedImageBuildNotStartedReason,
			Message: "wait old builder Pod termination",
		})
		return nil
	}

	// no need to reconcile
	if meta.IsStatusConditionTrue(seedImg.Status.Conditions, elementalv1.SeedImageConditionReady) &&
		foundPod.Status.Phase != corev1.PodSucceeded {
		return nil
	}

	logger.V(5).Info("Sync SeedImage Status from builder Pod", "pod-phase", foundPod.Status.Phase)

	switch foundPod.Status.Phase {
	case corev1.PodPending:
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.SeedImageConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.SeedImageBuildOngoingReason,
			Message: "seed image build ongoing",
		})
		return nil
	case corev1.PodRunning:
		rancherURL, err := r.getRancherServerAddress(ctx)
		if err != nil {
			errMsg := fmt.Errorf("failed to get Rancher Server Address: %w", err)
			meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
				Type:    elementalv1.SeedImageConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  elementalv1.SeedImageExposeFailureReason,
				Message: errMsg.Error(),
			})
			return errMsg
		}
		// Let's check here we have an associated Service, so the iso could be downloaded
		if err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: podNamespace}, &corev1.Service{}); err != nil {
			errMsg := fmt.Errorf("failed to get associated service: %w", err)
			meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
				Type:    elementalv1.SeedImageConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  elementalv1.SeedImageExposeFailureReason,
				Message: errMsg.Error(),
			})
			return errMsg
		}
		token, err := randomtoken.Generate()
		if err != nil {
			errMsg := fmt.Errorf("failed to generate registration token: %s", err.Error())
			meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
				Type:    elementalv1.SeedImageConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  elementalv1.SeedImageExposeFailureReason,
				Message: errMsg.Error(),
			})
			return errMsg
		}

		// Use the registration name or else default to "elemental" for the image file name
		imageName := "elemental"
		if seedImg.Spec.MachineRegistrationRef != nil && len(seedImg.Spec.MachineRegistrationRef.Name) > 0 {
			imageName = seedImg.Spec.MachineRegistrationRef.Name
		}
		seedImg.Status.DownloadToken = token
		seedImg.Status.DownloadURL = fmt.Sprintf("https://%s/elemental/seedimage/%s/%s.%s", rancherURL, token, imageName, seedImg.Spec.Type)
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.SeedImageConditionReady,
			Status:  metav1.ConditionTrue,
			Reason:  elementalv1.SeedImageBuildSuccessReason,
			Message: "seed image iso available",
		})
		return nil
	case corev1.PodFailed:
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.SeedImageConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  elementalv1.SeedImageBuildFailureReason,
			Message: "pod failed",
		})
		return nil
	case corev1.PodSucceeded:
		if err := r.Delete(ctx, foundPod); err != nil {
			errMsg := fmt.Errorf("failed to delete builder pod: %w", err)
			meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
				Type:    elementalv1.SeedImageConditionReady,
				Status:  metav1.ConditionFalse,
				Reason:  elementalv1.SeedImageBuildDeadline,
				Message: errMsg.Error(),
			})
			return errMsg
		}
		seedImg.Status.DownloadURL = ""
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.SeedImageConditionReady,
			Status:  metav1.ConditionTrue,
			Reason:  elementalv1.SeedImageBuildDeadline,
			Message: "seed image deadline elapsed",
		})
		return nil
	default:
		meta.SetStatusCondition(&seedImg.Status.Conditions, metav1.Condition{
			Type:    elementalv1.SeedImageConditionReady,
			Status:  metav1.ConditionUnknown,
			Reason:  elementalv1.SeedImageBuildUnknown,
			Message: fmt.Sprintf("pod phase %s", foundPod.Status.Phase),
		})
		return nil
	}
}

func (r *SeedImageReconciler) deleteChildResources(ctx context.Context, seedImg *elementalv1.SeedImage) error {
	foundPod := &corev1.Pod{}
	podName := seedImg.Name
	podNamespace := seedImg.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: podNamespace}, foundPod); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		// Pod found, delete it
		if !util.IsObjectOwned(&foundPod.ObjectMeta, seedImg.UID) {
			return fmt.Errorf("pod %s/%s doesn't belong to seedimage %s/%s", podNamespace, podName, seedImg.Namespace, seedImg.Name)
		}

		if err := r.Delete(ctx, foundPod); err != nil {
			return err
		}
	}
	seedImg.Status.DownloadURL = ""

	foundSvc := &corev1.Service{}
	svcName := seedImg.Name
	svcNamespace := seedImg.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: svcNamespace}, foundSvc); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		if !util.IsObjectOwned(&foundSvc.ObjectMeta, seedImg.UID) {
			return fmt.Errorf("service %s/%s doesn't belong to seedimage %s/%s", svcNamespace, svcName, seedImg.Namespace, seedImg.Name)
		}

		if err := r.Delete(ctx, foundSvc); err != nil {
			return err
		}
	}

	return nil
}

func (r *SeedImageReconciler) getRancherServerAddress(ctx context.Context) (string, error) {
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

func fillBuildImagePod(seedImg *elementalv1.SeedImage, buildImg string, pullPolicy corev1.PullPolicy) *corev1.Pod {
	name := seedImg.Name
	namespace := seedImg.Namespace
	baseImg := seedImg.Spec.BaseImage
	deadline := seedImg.Spec.LifetimeMinutes
	configMap := name

	var initContainers []corev1.Container
	if seedImg.Spec.BuildContainer == nil {
		initContainers = defaultInitContainers(seedImg, buildImg, pullPolicy)
	} else {
		initContainers = userDefinedInitContainers(seedImg)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app.kubernetes.io/name": name},
			Annotations: map[string]string{
				"elemental.cattle.io/base-image": baseImg,
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: initContainers,
			Containers: []corev1.Container{
				{
					Name:            "serve",
					Image:           buildImg,
					ImagePullPolicy: pullPolicy,
					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 80,
						},
					},
					Args: []string{"-d", "$(ELEMENTAL_OUTPUT_NAME)", "-t", fmt.Sprintf("%d", deadline*60)},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "iso-storage",
							MountPath: "/srv",
						},
					},
					WorkingDir: "/srv",
					Env: []corev1.EnvVar{
						{
							Name: "ELEMENTAL_OUTPUT_NAME",
							ValueFrom: &corev1.EnvVarSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: seedImg.Name,
									},
									Key: configMapKeyOutputName,
								},
							},
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
							SizeLimit: &seedImg.Spec.Size,
						},
					},
				}, {
					Name: "config-map",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: configMap},
							Items: []corev1.KeyToPath{
								{Key: configMapKeyRegistration, Path: "reg/livecd-cloud-config.yaml"},
								{Key: configMapKeyCloudConfig, Path: "iso-config/cloud-config.yaml"},
								{Key: configMapKeyResetConfig, Path: "reg/reset-config.yaml"},
							},
						},
					},
				},
			},
		},
	}
	return pod
}

func defaultInitContainers(seedImg *elementalv1.SeedImage, buildImg string, pullPolicy corev1.PullPolicy) []corev1.Container {
	if seedImg.Spec.Type == elementalv1.TypeRaw {
		return defaultRawInitContainers(seedImg, buildImg, pullPolicy)
	}

	return defaultIsoInitContainers(seedImg, buildImg, pullPolicy)
}

func defaultRawInitContainers(seedImg *elementalv1.SeedImage, buildImg string, pullPolicy corev1.PullPolicy) []corev1.Container {
	platformArg := ""
	if seedImg.Spec.TargetPlatform != "" {
		platformArg = fmt.Sprintf("--platform %s", seedImg.Spec.TargetPlatform)
	}

	buildCommands := []string{
		fmt.Sprintf(`/usr/bin/elemental \
        --debug \
        build-disk \
        %s \
        --expandable \
        --deploy-command systemctl,start,elemental-register-reset.service \
        --squash-no-compression \
        --cloud-init /overlay/reg/reset-config.yaml,/overlay/iso-config/cloud-config.yaml \
        -n elemental \
        -o /iso \
        --system $(ELEMENTAL_BASE_IMAGE)`, platformArg),

		"mv /iso/elemental.raw /iso/$(ELEMENTAL_OUTPUT_NAME)",
	}

	return []corev1.Container{
		{
			Name:            "build",
			Image:           buildImg,
			ImagePullPolicy: pullPolicy,
			Command:         []string{"/bin/bash", "-c"},
			Args:            []string{strings.Join(buildCommands, " && ")},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "iso-storage",
					MountPath: "/iso",
				}, {
					Name:      "config-map",
					MountPath: "/overlay",
				},
			},
			Env: defaultEnvVars(seedImg.Name),
		},
	}
}

func defaultIsoInitContainers(seedImg *elementalv1.SeedImage, buildImg string, pullPolicy corev1.PullPolicy) []corev1.Container {
	const baseIsoPath = "/iso/base.iso"

	containers := []corev1.Container{}
	buildCommands := []string{
		fmt.Sprintf("xorriso -indev %s -outdev /iso/$(ELEMENTAL_OUTPUT_NAME) -map /overlay/reg/livecd-cloud-config.yaml /livecd-cloud-config.yaml -map /overlay/iso-config/cloud-config.yaml /iso-config/cloud-config.yaml -boot_image any replay", baseIsoPath),
		fmt.Sprintf("rm -rf %s", baseIsoPath),
	}

	if util.IsHTTP(seedImg.Spec.BaseImage) {
		buildCommands = append([]string{fmt.Sprintf("curl -Lo %s %s", baseIsoPath, seedImg.Spec.BaseImage)}, buildCommands...)
	} else {
		command := []string{"busybox", "sh", "-c"}
		args := []string{fmt.Sprintf("busybox cp /elemental-iso/*.iso %s", baseIsoPath)}
		image := seedImg.Spec.BaseImage

		if seedImg.Spec.TargetPlatform != "" {
			image = buildImg
			command = []string{"/bin/bash", "-c"}
			args = []string{fmt.Sprintf("mkdir /work && elemental pull-image --platform=%s %s /work && cp /work/elemental-iso/*.iso %s", seedImg.Spec.TargetPlatform, seedImg.Spec.BaseImage, baseIsoPath)}
		}

		// If baseImg is not an HTTP url assume it is an image reference
		containers = append(
			containers, corev1.Container{
				Name:            "baseiso",
				Image:           image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         command,
				Args:            args,
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "iso-storage",
						MountPath: "/iso",
					},
				},
			},
		)
	}

	containers = append(
		containers, corev1.Container{
			Name:            "build",
			Image:           buildImg,
			ImagePullPolicy: pullPolicy,
			Command:         []string{"/bin/bash", "-c"},
			Args:            []string{strings.Join(buildCommands, " && ")},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "iso-storage",
					MountPath: "/iso",
				}, {
					Name:      "config-map",
					MountPath: "/overlay",
				},
			},
			Env: defaultEnvVars(seedImg.Name),
		},
	)

	// Make sure the pod starts starts having enough disk for the maximum volume size
	// volume size just represents a maximum it is not computed to schedule pods.
	// To ensure there is enough local disk space we add a storage requirement to the first
	// init container of the pod.
	containers[0].Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceEphemeralStorage: seedImg.Spec.Size,
		},
	}

	return containers
}

func userDefinedInitContainers(seedImg *elementalv1.SeedImage) []corev1.Container {
	c := seedImg.Spec.BuildContainer

	name := "build"
	if c.Name != "" {
		name = c.Name
	}

	return []corev1.Container{
		{
			Name:            name,
			Image:           c.Image,
			ImagePullPolicy: c.ImagePullPolicy,
			Command:         c.Command,
			Args:            c.Args,
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "iso-storage",
					MountPath: "/iso",
				}, {
					Name:      "config-map",
					MountPath: "/overlay",
				},
			},
			Env: defaultEnvVars(seedImg.Name),
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: seedImg.Spec.Size,
				},
			},
		},
	}
}

func defaultEnvVars(seedImgName string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "ELEMENTAL_DEVICE",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: seedImgName,
					},
					Key: configMapKeyDevice,
				},
			},
		},
		{
			Name: "ELEMENTAL_REGISTRATION_URL",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: seedImgName,
					},
					Key: configMapKeyRegistrationURL,
				},
			},
		},
		{
			Name: "ELEMENTAL_BASE_IMAGE",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: seedImgName,
					},
					Key: configMapKeyBaseImage,
				},
			},
		},
		{
			Name: "ELEMENTAL_OUTPUT_NAME",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: seedImgName,
					},
					Key: configMapKeyOutputName,
				},
			},
		},
	}
}

func (r *SeedImageReconciler) createBuildImageService(ctx context.Context, seedImg *elementalv1.SeedImage) error {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling Service resources")

	svcName := seedImg.Name
	svcNamespace := seedImg.Namespace

	foundSvc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: svcNamespace}, foundSvc)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	seedImageDeadlinePassed := false
	if seedImgCondition := meta.FindStatusCondition(seedImg.Status.Conditions, elementalv1.SeedImageConditionReady); seedImgCondition != nil &&
		seedImgCondition.Status == metav1.ConditionTrue &&
		seedImgCondition.Reason == elementalv1.SeedImageBuildDeadline {
		seedImageDeadlinePassed = true
	}

	if err == nil {
		logger.V(5).Info("Service already there")

		// ensure the service was created by us
		for _, owner := range foundSvc.GetOwnerReferences() {
			if owner.UID == seedImg.UID {
				if seedImageDeadlinePassed {
					logger.V(5).Info("Seed image deadline passed, delete associated service", "service", foundSvc.Name)
					if err := r.Delete(ctx, foundSvc); err != nil {
						return fmt.Errorf("failed to delete service %s: %w", foundSvc.Name, err)
					}
				}
				return nil
			}
		}

		return fmt.Errorf("service already exists and was not created by this controller")
	}

	if seedImageDeadlinePassed {
		return nil
	}

	logger.V(5).Info("Creating service")

	service := fillBuildImageService(seedImg.Name, seedImg.Namespace)

	if err := controllerutil.SetControllerReference(seedImg, service, r.Scheme()); err != nil {
		return err
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
		},
	}

	return service
}

func serializeResetRegistrationYaml(config *elementalv1.Config) ([]byte, error) {
	regConf, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}

	conf := &schema.YipConfig{
		Name: "Write registration config",
		Stages: map[string][]schema.Stage{
			"initramfs": {
				schema.Stage{
					If:   `[ -f "/run/cos/recovery_mode" ] || [ -f "/run/elemental/recovery_mode" ]`,
					Name: "Save registration config",
					Directories: []schema.Directory{
						{
							Path:        filepath.Dir("/oem/registration"),
							Permissions: 0700,
						},
					},
					Files: []schema.File{
						{
							Path:        "/oem/registration/config.yaml",
							Content:     base64.StdEncoding.EncodeToString(regConf),
							Encoding:    "b64",
							Permissions: 0666,
						},
					},
				},
			},
		},
	}

	return yaml.Marshal(conf)
}
