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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/util"
	fleetv1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/rancher/wrangler/v2/pkg/name"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	rancherSystemNamespace = "cattle-system"
	fleetLocalNamespace    = "fleet-local"
)

var dnsLabelRegex = regexp.MustCompile("[^a-zA-Z0-9- ]+")

// ManagedOSImageReconciler reconciles a ManagedOSImage object.
type ManagedOSImageReconciler struct {
	client.Client
	DefaultRegistry string
	Scheme          *runtime.Scheme
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosimages/status,verbs=get;update;patch;list
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=managedosversions,verbs=get;list;watch
// +kubebuilder:rbac:groups="fleet.cattle.io",resources=bundles,verbs=create;get;update;list;watch

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

	// Ensure we patch the latest version otherwise we could erratically overlap with other controllers (e.g. backup and restore)
	patchBase := client.MergeFromWithOptions(managedOSImage.DeepCopy(), client.MergeFromWithOptimisticLock{})

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
		errs = append(errs, fmt.Errorf("failed to patch managed OS image object: %w", err))
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

	if managedOSImage.Namespace == fleetLocalNamespace && len(managedOSImage.Spec.Targets) > 0 { // TODO: this should be a part of validation webhook
		return ctrl.Result{}, errors.New("spec.targets should be empty if in the fleet-local namespace")
	}

	bundleResources, err := r.newFleetBundleResources(ctx, managedOSImage)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create fleet bundle resources: %w", err)
	}

	// Create a new Fleet bundle if we didn't do it before. Otherwise update it.
	if meta.IsStatusConditionTrue(managedOSImage.Status.Conditions, elementalv1.FleetBundleCreation) {
		logger.Info("Fleet bundle already exists")
		if err := r.updateFleetBundle(ctx, managedOSImage, bundleResources); err != nil {
			return ctrl.Result{}, fmt.Errorf("updating Bundle from ManagedOSImage: %w", err)
		}
	} else {
		if err := r.createFleetBundle(ctx, managedOSImage, bundleResources); err != nil {
			meta.SetStatusCondition(&managedOSImage.Status.Conditions, metav1.Condition{
				Type:    elementalv1.FleetBundleCreation,
				Reason:  elementalv1.FleetBundleCreateFailureReason,
				Status:  metav1.ConditionFalse,
				Message: err.Error(),
			})
			return ctrl.Result{}, fmt.Errorf("creating Bundle from ManagedOSImage: %w", err)
		}
	}

	if err := r.updateManagedOSImageStatus(ctx, managedOSImage); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update managed OS image status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *ManagedOSImageReconciler) newFleetBundleResources(ctx context.Context, managedOSImage *elementalv1.ManagedOSImage) ([]fleetv1.BundleResource, error) {
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

		// Add a label that can be used to List all ManagedOSImages referencing a certain ManagedOSVersion.
		if managedOSImage.ObjectMeta.Labels == nil {
			managedOSImage.ObjectMeta.Labels = map[string]string{}
		}
		managedOSImage.ObjectMeta.Labels[elementalv1.ElementalManagedOSImageVersionNameLabel] = managedOSImage.Spec.ManagedOSVersionName
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
		upgradeContainerSpec = &upgradev1.ContainerSpec{}
	}

	if upgradeContainerSpec.Image == "" {
		upgradeContainerSpec.Image = prefixPrivateRegistry(image, r.DefaultRegistry)
	}

	if len(upgradeContainerSpec.Command) == 0 {
		upgradeContainerSpec.Command = []string{"/usr/sbin/suc-upgrade"}
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

	// The system-upgrade-controller will use the Plan name to eventually create a Volume.
	// To keep things smooth and not bother the end user with excessive name validation,
	// we just do a safe name conversion here.
	uniqueName = toDNSLabel(uniqueName)

	upgradeContainerSpecCopy := *upgradeContainerSpec.DeepCopy()
	correlationID, err := applyCorrelationLabels(managedOSImage, version, &upgradeContainerSpecCopy)
	if err != nil {
		return nil, fmt.Errorf("applying upgrade environment variables: %w", err)
	}

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
				{
					Verbs:     []string{"get"},
					APIGroups: []string{"apps"},
					Resources: []string{"daemonsets"},
				},
				{
					Verbs:     []string{"create"},
					APIGroups: []string{""},
					Resources: []string{"pods/eviction"},
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
				Labels:    map[string]string{elementalv1.ElementalUpgradeCorrelationIDLabel: correlationID},
			},
			Spec: upgradev1.PlanSpec{
				Concurrency: concurrency,
				Version:     version,
				Tolerations: []corev1.Toleration{{
					Operator: corev1.TolerationOpExists,
				}},
				Exclusive:          true,
				ServiceAccountName: uniqueName,
				NodeSelector:       selector,
				Cordon:             cordon,
				Drain:              managedOSImage.Spec.Drain,
				Prepare:            managedOSImage.Spec.Prepare,
				Secrets: []upgradev1.SecretSpec{{
					Name: uniqueName,
					Path: "/run/data",
				}},
				Upgrade: &upgradeContainerSpecCopy,
			},
		},
	}

	return r.objToFleetBundleResources(objs)
}

func (r *ManagedOSImageReconciler) createFleetBundle(ctx context.Context, managedOSImage *elementalv1.ManagedOSImage, bundleResources []fleetv1.BundleResource) error {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Creating new fleet bundle")

	bundle := &fleetv1.Bundle{}
	r.mapImageToBundle(*managedOSImage, bundleResources, bundle)

	if managedOSImage.Namespace == fleetLocalNamespace {
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

func (r *ManagedOSImageReconciler) updateFleetBundle(ctx context.Context, managedOSImage *elementalv1.ManagedOSImage, bundleResources []fleetv1.BundleResource) error {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Updating existing bundle")

	bundleName := r.formatBundleName(*managedOSImage)
	bundleNamespace := managedOSImage.Namespace

	bundle := &fleetv1.Bundle{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: bundleNamespace,
		Name:      bundleName,
	}, bundle); err != nil {
		logger.Error(err, "Could not get expected Bundle")
		return fmt.Errorf("getting bundle '%s/%s': %w", bundleNamespace, bundleName, err)
	}
	r.mapImageToBundle(*managedOSImage, bundleResources, bundle)

	if managedOSImage.Namespace == fleetLocalNamespace {
		bundle.Spec.Targets = []fleetv1.BundleTarget{{ClusterName: "local"}}
	}

	if err := r.Update(ctx, bundle); err != nil {
		logger.Error(err, "Could not update Bundle")
		return fmt.Errorf("updating bundle: %w", err)
	}

	return nil
}

func (r *ManagedOSImageReconciler) formatBundleName(managedOSImage elementalv1.ManagedOSImage) string {
	return name.SafeConcatName("mos", managedOSImage.Name)
}

func (r *ManagedOSImageReconciler) mapImageToBundle(managedOSImage elementalv1.ManagedOSImage, bundleResources []fleetv1.BundleResource, bundle *fleetv1.Bundle) {
	bundle.ObjectMeta.Name = r.formatBundleName(managedOSImage)
	bundle.ObjectMeta.Namespace = managedOSImage.Namespace
	bundle.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: elementalv1.GroupVersion.String(),
			Kind:       "ManagedOSImage",
			Name:       managedOSImage.Name,
			UID:        managedOSImage.UID,
			Controller: ptr.To(true),
		},
	}

	bundle.Spec.Resources = bundleResources
	bundle.Spec.RolloutStrategy = managedOSImage.Spec.ClusterRolloutStrategy

	if managedOSImage.Namespace == fleetLocalNamespace {
		bundle.Spec.Targets = []fleetv1.BundleTarget{{ClusterName: "local"}}
	} else {
		bundle.Spec.Targets = managedOSImage.Spec.Targets
	}
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
	if len(managedOSImage.Spec.CloudConfig) == 0 {
		return []byte{}, nil
	}

	data, err := util.MarshalCloudConfig(managedOSImage.Spec.CloudConfig)
	if err != nil {
		return nil, fmt.Errorf("mashalling cloud config: %w", err)
	}

	return data, nil
}

func getImageVersion(managedOSImage *elementalv1.ManagedOSImage, managedOSVersion *elementalv1.ManagedOSVersion) (string, string, error) {
	baseImage := managedOSImage.Spec.OSImage
	if baseImage == "" && managedOSVersion != nil {
		osImg, err := managedOSVersion.ContainerImage()
		if err != nil {
			return "", "", err
		}
		baseImage = osImg.ImageURI
	}

	// Get the registry prefix
	parts := strings.Split(baseImage, "/")
	registry := ""
	if len(parts) > 1 {
		registry = parts[0]
		parts = parts[1:]
	}

	// Now get the version
	remainder := strings.Join(parts, "/")
	imageParts := strings.SplitN(remainder, ":", 2)
	version := "latest"
	if len(imageParts) == 2 {
		version = imageParts[1]
	}
	image := imageParts[0]

	// Add the registry back if needed
	if len(registry) > 0 {
		image = fmt.Sprintf("%s/%s", registry, imageParts[0])
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

func applyCorrelationLabels(managedOSImage *elementalv1.ManagedOSImage, imageVersion string, upgradeContainerSpec *upgradev1.ContainerSpec) (string, error) {
	// Include additional snapshot labels
	upgradeContainerSpec.Env = append(upgradeContainerSpec.Env, corev1.EnvVar{
		Name:  "ELEMENTAL_REGISTER_UPGRADE_SNAPSHOT_LABELS",
		Value: formatSnapshotLabels(*managedOSImage, imageVersion, *upgradeContainerSpec),
	})

	// Calculate the managedOSImage.Spec after all changes
	correlationID, err := managedOSImageHash(managedOSImage.Spec)
	if err != nil {
		return "", fmt.Errorf("calculating ManagedOSImage hash: %w", err)
	}
	// Tag this ManagedOSImage with the correlation ID label
	if managedOSImage.Labels == nil {
		managedOSImage.Labels = map[string]string{}
	}
	managedOSImage.Labels[elementalv1.ElementalUpgradeCorrelationIDLabel] = correlationID
	// Use the hash as correlation ID that will be applied as snapshot label on the machine.
	upgradeContainerSpec.Env = append(upgradeContainerSpec.Env, corev1.EnvVar{
		Name:  "ELEMENTAL_REGISTER_UPGRADE_CORRELATION_ID",
		Value: correlationID,
	})

	return correlationID, nil
}

// This converts any string to RFC 1123 DNS label standard by replacing invalid characters with "-"
func toDNSLabel(input string) string {
	output := dnsLabelRegex.ReplaceAllString(input, "-")
	output = strings.TrimPrefix(output, "-")
	output = strings.TrimSuffix(output, "-")
	return output
}

func managedOSImageHash(spec elementalv1.ManagedOSImageSpec) (string, error) {
	// Do not calculate a new hash if target changes.
	spec.Targets = []fleetv1.BundleTarget{}

	specData, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("unmarshalling ManagedOSImage.Spec: %w", err)
	}
	hash := sha256.New224() //sha256 produces 64 chars (max label value is 63). Use sha224 instead.
	if _, err := hash.Write(specData); err != nil {
		return "", fmt.Errorf("writing hash: %w", err)
	}

	result := hash.Sum(nil)
	return fmt.Sprintf("%x", result), nil
}

// formatSnapshotLabels formats the *_SNAPSHOT_LABELS environment variable: "managedOSImage=foo,image=bar:v1.2.3"
// It is required for this function to always generate the same value in a predictable way, in order to keep the computed Plan hash the same at every loop.
func formatSnapshotLabels(managedOSImage elementalv1.ManagedOSImage, imageVersion string, upgradeContainerSpec upgradev1.ContainerSpec) string {
	imageWithVersion := fmt.Sprintf("%s:%s", upgradeContainerSpec.Image, imageVersion)

	formattedLabels := fmt.Sprintf("managedOSImage=%s,image=%s", managedOSImage.Name, imageWithVersion)
	if len(managedOSImage.Spec.ManagedOSVersionName) > 0 {
		formattedLabels = fmt.Sprintf("%s,managedOSVersion=%s", formattedLabels, managedOSImage.Spec.ManagedOSVersionName)
	}
	return formattedLabels
}
