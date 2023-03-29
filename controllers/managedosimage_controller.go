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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/util"
	fleetv1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/rancher/wrangler/pkg/name"
	"gopkg.in/yaml.v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	rancherSystemNamespace = "cattle-system"
)

// ManagedOSImageReconciler reconciles a ManagedOSImage object.
type ManagedOSImageReconciler struct {
	client.Client
	DefaultRegistry string
	Scheme          *runtime.Scheme
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosimages/status,verbs=get;update;patch;list
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversions,verbs=get;list;watch
// +kubebuilder:rbac:groups="fleet.cattle.io",resources=bundles,verbs=create;get;list;watch

func (r *ManagedOSImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elementalv1.ManagedOSImage{}).
		Owns(&fleetv1.Bundle{}).
		Complete(r)
}

func (r *ManagedOSImageReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) { //nolint:dupl
	logger := ctrl.LoggerFrom(ctx)

	managedOSImage := &elementalv1.ManagedOSImage{}
	err := r.Get(ctx, req.NamespacedName, managedOSImage)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(log.DebugDepth).Info("Object was not found, not an error")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get managed OS image object: %w", err)
	}

	patchBase := client.MergeFrom(managedOSImage.DeepCopy())

	// We have to sanitize the conditions because old API definitions didn't have proper validation.
	managedOSImage.Status.Conditions = util.RemoveInvalidConditions(managedOSImage.Status.Conditions)

	// Collect errors as an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, managedOSImage)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reconciling managed OS image object: %w", err))
	}

	managedosimageStatusCopy := managedOSImage.Status.DeepCopy() // Patch call will erase the status

	if err := r.Patch(ctx, managedOSImage, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch status for managed OS image object: %w", err))
	}

	managedOSImage.Status = *managedosimageStatusCopy

	if err := r.Status().Patch(ctx, managedOSImage, patchBase); err != nil && !apierrors.IsNotFound(err) {
		errs = append(errs, fmt.Errorf("failed to patch status for managed OS image object: %w", err))
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errorutils.NewAggregate(errs)
	}

	return result, nil
}

func (r *ManagedOSImageReconciler) reconcile(ctx context.Context, managedOSImage *elementalv1.ManagedOSImage) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling managed OS image object")

	if managedOSImage.Namespace == "fleet-local" && len(managedOSImage.Spec.Targets) > 0 { // TODO: this should be a part of validation webhook
		return ctrl.Result{}, errors.New("spec.targets should be empty if in the fleet-local namespace")
	}

	bundleResources, err := r.newFleetBundleResources(ctx, managedOSImage)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create fleet bundle resources: %w", err)
	}

	if err := r.createFleetBundle(ctx, managedOSImage, bundleResources); err != nil {
		meta.SetStatusCondition(&managedOSImage.Status.Conditions, metav1.Condition{
			Type:    elementalv1.FleetBundleCreation,
			Reason:  elementalv1.FleetBundleCreateFailureReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to create fleet bundle: %w", err)
	}

	if err := r.updateManagedOSImageStatus(ctx, managedOSImage); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update managed OS image status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *ManagedOSImageReconciler) newFleetBundleResources(ctx context.Context, managedOSImage *elementalv1.ManagedOSImage) ([]fleetv1.BundleResource, error) {
	logger := ctrl.LoggerFrom(ctx)

	if meta.IsStatusConditionTrue(managedOSImage.Status.Conditions, elementalv1.FleetBundleCreation) {
		logger.Info("Fleet bundle already exists, skipping bundle resource creation")
		return nil, nil
	}

	cloudConfig, err := getCloudConfig(managedOSImage)
	if err != nil {
		return nil, err
	}

	concurrency := int64(1)
	if managedOSImage.Spec.Concurrency != nil { // TODO: this should be a part of defaulting webhook
		concurrency = *managedOSImage.Spec.Concurrency
	}

	cordon := true // TODO: we probably should not default boolean to true
	if managedOSImage.Spec.Cordon != nil {
		cordon = *managedOSImage.Spec.Cordon
	}

	var managedOSVersion *elementalv1.ManagedOSVersion

	m := make(map[string]runtime.RawExtension)

	// if a managedOS version was specified, we fetch it for later use and store the metadata
	if managedOSImage.Spec.ManagedOSVersionName != "" {
		managedOSVersion = &elementalv1.ManagedOSVersion{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: managedOSImage.Namespace,
			Name:      managedOSImage.Spec.ManagedOSVersionName,
		}, managedOSVersion); err != nil {
			return nil, fmt.Errorf("failed to get managedOSVersion: %w", err)
		}
		m = managedOSVersion.Spec.Metadata
	}

	// Entire logic from below is carried from the old code.
	// XXX Issues currently standing:
	// - minVersion is not respected:
	//	 gate minVersion that are not passing validation checks with the version reported
	// - Monitoring upgrade status from the fleet bundles (reconcile to update the status to report what is the current version )
	// - Enforce a ManagedOSImage "version" that is applied to a one node only. Or check out if either fleet is already doing that

	image, version, err := getImageVersion(managedOSImage, managedOSVersion)
	if err != nil {
		return nil, err
	}

	selector := managedOSImage.Spec.NodeSelector
	if selector == nil {
		selector = &metav1.LabelSelector{}
	}

	upgradeContainerSpec := managedOSImage.Spec.UpgradeContainer
	if managedOSVersion != nil && upgradeContainerSpec == nil {
		upgradeContainerSpec = managedOSVersion.Spec.UpgradeContainer
	}

	if upgradeContainerSpec == nil {
		upgradeContainerSpec = &upgradev1.ContainerSpec{
			Image: prefixPrivateRegistry(image[0], r.DefaultRegistry),
			Command: []string{
				"/usr/sbin/suc-upgrade",
			},
		}
	}

	// Encode metadata from the spec as environment in the upgrade spec pod
	metadataEnv := metadataEnv(m)

	// metadata envs overwrites any other specified
	keys := map[string]interface{}{}
	for _, e := range metadataEnv {
		keys[e.Name] = nil
	}

	for _, e := range upgradeContainerSpec.Env {
		if _, ok := keys[e.Name]; !ok {
			metadataEnv = append(metadataEnv, e)
		}
	}

	sort.Slice(metadataEnv, func(i, j int) bool {
		dat := []string{metadataEnv[i].Name, metadataEnv[j].Name}
		sort.Strings(dat)
		return dat[0] == metadataEnv[i].Name
	})

	upgradeContainerSpec.Env = metadataEnv

	uniqueName := name.SafeConcatName("os-upgrader", managedOSImage.Name)

	objs := []runtime.Object{
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: uniqueName,
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"update", "get", "list", "watch", "patch"},
					APIGroups: []string{""},
					Resources: []string{"nodes"},
				},
				{
					Verbs:     []string{"list"},
					APIGroups: []string{""},
					Resources: []string{"pods"},
				},
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: uniqueName,
			},
			Subjects: []rbacv1.Subject{{
				Kind:      "ServiceAccount",
				Name:      uniqueName,
				Namespace: rancherSystemNamespace,
			}},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     uniqueName,
			},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uniqueName,
				Namespace: rancherSystemNamespace,
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uniqueName,
				Namespace: rancherSystemNamespace,
			},
			Data: map[string][]byte{
				"cloud-config": cloudConfig,
			},
		},
		&upgradev1.Plan{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Plan",
				APIVersion: "upgrade.cattle.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      uniqueName,
				Namespace: rancherSystemNamespace,
			},
			Spec: upgradev1.PlanSpec{
				Concurrency: concurrency,
				Version:     version,
				Tolerations: []corev1.Toleration{{
					Operator: corev1.TolerationOpExists,
				}},
				ServiceAccountName: uniqueName,
				NodeSelector:       selector,
				Cordon:             cordon,
				Drain:              managedOSImage.Spec.Drain,
				Prepare:            managedOSImage.Spec.Prepare,
				Secrets: []upgradev1.SecretSpec{{
					Name: uniqueName,
					Path: "/run/data",
				}},
				Upgrade: upgradeContainerSpec,
			},
		},
	}

	return r.objToFleetBundleResources(objs)
}

func (r *ManagedOSImageReconciler) createFleetBundle(ctx context.Context, managedOSImage *elementalv1.ManagedOSImage, bundleResources []fleetv1.BundleResource) error {
	logger := ctrl.LoggerFrom(ctx)

	if meta.IsStatusConditionTrue(managedOSImage.Status.Conditions, elementalv1.FleetBundleCreation) {
		logger.Info("Fleet bundle already exists, skipping bundle resource creation")
		return nil
	}

	bundle := &fleetv1.Bundle{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeConcatName("mos", managedOSImage.Name),
			Namespace: managedOSImage.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: elementalv1.GroupVersion.String(),
					Kind:       "ManagedOSImage",
					Name:       managedOSImage.Name,
					UID:        managedOSImage.UID,
					Controller: pointer.Bool(true),
				},
			},
		},
		Spec: fleetv1.BundleSpec{
			Resources:               bundleResources,
			BundleDeploymentOptions: fleetv1.BundleDeploymentOptions{},
			RolloutStrategy:         managedOSImage.Spec.ClusterRolloutStrategy,
			Targets:                 convertBundleTargets(managedOSImage.Spec.Targets),
		},
	}

	if managedOSImage.Namespace == "fleet-local" {
		bundle.Spec.Targets = []fleetv1.BundleTarget{{ClusterName: "local"}}
	}

	if err := r.Create(ctx, bundle); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create fleet bundle: %w", err)
	}

	meta.SetStatusCondition(&managedOSImage.Status.Conditions, metav1.Condition{
		Type:   elementalv1.FleetBundleCreation,
		Reason: elementalv1.FleetBundleCreateSuccessReason,
		Status: metav1.ConditionTrue,
	})

	return nil
}

func (r *ManagedOSImageReconciler) updateManagedOSImageStatus(ctx context.Context, managedOSImage *elementalv1.ManagedOSImage) error {
	bundle := &fleetv1.Bundle{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: managedOSImage.Namespace,
		Name:      name.SafeConcatName("mos", managedOSImage.Name),
	}, bundle); err != nil {
		return err
	}

	// Convert Bundle status conditions to ManagedOSImage status conditions
	for _, cond := range bundle.Status.Conditions {
		newCondition := metav1.Condition{
			Status:  metav1.ConditionStatus(cond.Status),
			Message: cond.Message,
		}

		if cond.Type == "" {
			newCondition.Type = "UnknownType"
		} else {
			newCondition.Type = cond.Type
		}

		if cond.Reason == "" {
			newCondition.Reason = "UnknownReason"
		} else {
			newCondition.Reason = cond.Reason
		}

		if cond.Status == "" {
			newCondition.Status = metav1.ConditionUnknown
		} else {
			newCondition.Status = metav1.ConditionStatus(cond.Status)
		}

		meta.SetStatusCondition(&managedOSImage.Status.Conditions, newCondition)
	}

	return nil
}

func (r *ManagedOSImageReconciler) objToFleetBundleResources(objs []runtime.Object) ([]fleetv1.BundleResource, error) {
	result := []fleetv1.BundleResource{}
	for _, obj := range objs {
		obj = obj.DeepCopyObject()

		gvks, _, err := r.Scheme.ObjectKinds(obj)
		if err != nil {
			return nil, err
		}

		if len(gvks) == 0 {
			return nil, nil
		}

		kind := obj.GetObjectKind()
		kind.SetGroupVersionKind(gvks[0])

		gvk := obj.GetObjectKind().GroupVersionKind()
		if gvk.Kind == "" {
			return nil, errors.New("can't set object GVK")
		}

		typeMeta, err := meta.TypeAccessor(obj)
		if err != nil {
			return nil, err
		}

		meta, err := meta.Accessor(obj)
		if err != nil {
			return nil, err
		}

		data, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}

		digest := sha256.Sum256(data)
		filename := name.SafeConcatName(typeMeta.GetKind(), meta.GetNamespace(), meta.GetName(), hex.EncodeToString(digest[:])[:12]) + ".yaml"
		result = append(result, fleetv1.BundleResource{
			Name:    filename,
			Content: string(data),
		})
	}
	return result, nil
}

func getCloudConfig(managedOSImage *elementalv1.ManagedOSImage) ([]byte, error) {
	if managedOSImage.Spec.CloudConfig == nil || len(managedOSImage.Spec.CloudConfig.Data) == 0 {
		return []byte{}, nil
	}

	data, err := yaml.Marshal(managedOSImage.Spec.CloudConfig.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cloud config: %w", err)
	}

	return append([]byte("#cloud-config\n"), data...), nil
}

func getImageVersion(managedOSImage *elementalv1.ManagedOSImage, managedOSVersion *elementalv1.ManagedOSVersion) ([]string, string, error) {
	baseImage := managedOSImage.Spec.OSImage
	if baseImage == "" && managedOSVersion != nil {
		osMeta, err := managedOSVersion.Metadata()
		if err != nil {
			return []string{}, "", err
		}
		baseImage = osMeta.ImageURI
	}

	image := strings.SplitN(baseImage, ":", 2)
	version := "latest"
	if len(image) == 2 {
		version = image[1]
	}

	return image, version, nil
}

func prefixPrivateRegistry(image, prefix string) string {
	if prefix == "" {
		return image
	}
	return prefix + "/" + image
}

func metadataEnv(m map[string]runtime.RawExtension) []corev1.EnvVar {
	// Encode metadata as environment in a slice of envVar
	envs := []corev1.EnvVar{}
	for k, v := range m {
		value := strings.Trim(string(v.Raw), "\"")
		envs = append(envs, corev1.EnvVar{Name: strings.ToUpper(fmt.Sprintf("METADATA_%s", k)), Value: value})
	}
	return envs
}

func convertBundleTargets(elementalBundleTargets []elementalv1.BundleTarget) []fleetv1.BundleTarget {
	result := []fleetv1.BundleTarget{}

	for _, elementalBundleTarget := range elementalBundleTargets {
		result = append(result, fleetv1.BundleTarget{
			Name:                 elementalBundleTarget.Name,
			ClusterName:          elementalBundleTarget.ClusterName,
			ClusterSelector:      elementalBundleTarget.ClusterSelector,
			ClusterGroup:         elementalBundleTarget.ClusterGroup,
			ClusterGroupSelector: elementalBundleTarget.ClusterGroupSelector,
			BundleDeploymentOptions: fleetv1.BundleDeploymentOptions{
				DefaultNamespace:    elementalBundleTarget.DefaultNamespace,
				TargetNamespace:     elementalBundleTarget.TargetNamespace,
				Kustomize:           elementalBundleTarget.Kustomize,
				Helm:                elementalBundleTarget.Helm,
				ServiceAccount:      elementalBundleTarget.ServiceAccount,
				ForceSyncGeneration: elementalBundleTarget.ForceSyncGeneration,
				YAML:                elementalBundleTarget.YAML,
				Diff:                elementalBundleTarget.Diff,
			},
		})
	}

	return result
}
