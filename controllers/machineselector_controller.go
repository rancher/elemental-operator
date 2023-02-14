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
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"

	"encoding/base64"
	"encoding/json"

	"github.com/google/go-cmp/cmp"
	"github.com/rancher/system-agent/pkg/applyinator"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/util"
)

// MachineInventorySelectorReconciler reconciles a MachineInventorySelector object.
type MachineInventorySelectorReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=elemental.cattle.io,resources=machineinventoryselectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elemental.cattle.io,resources=machineinventoryselectors/status,verbs=get;update;patch;list
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;patch;list;watch

func (r *MachineInventorySelectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elementalv1.MachineInventorySelector{}).
		Watches(
			&source.Kind{
				Type: &elementalv1.MachineInventory{},
			},
			handler.EnqueueRequestsFromMapFunc(r.MachineInventoryToSelector),
		).
		WithEventFilter(r.ignoreIncrementalStatusUpdate()).
		Complete(r)
}

func (r *MachineInventorySelectorReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) { //nolint:dupl
	logger := ctrl.LoggerFrom(ctx)

	machineInventorySelector := &elementalv1.MachineInventorySelector{}
	err := r.Get(ctx, req.NamespacedName, machineInventorySelector)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Object was not found")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get machine inventory selector object: %w", err)
	}

	patchBase := client.MergeFrom(machineInventorySelector.DeepCopy())

	// We have to sanitize the conditions because old API definitions didn't have proper validation.
	machineInventorySelector.Status.Conditions = util.RemoveInvalidConditions(machineInventorySelector.Status.Conditions)

	// Collect errors as an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, machineInventorySelector)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reconciling machine inventory selector object: %w", err))
	}

	machineInventorySelectorStatusCopy := machineInventorySelector.Status.DeepCopy() // Patch call will erase the status

	if err := r.Patch(ctx, machineInventorySelector, patchBase); err != nil {
		errs = append(errs, fmt.Errorf("failed to patch status for machine inventory selector object: %w", err))
	}

	machineInventorySelector.Status = *machineInventorySelectorStatusCopy

	if err := r.Status().Patch(ctx, machineInventorySelector, patchBase); err != nil {
		errs = append(errs, fmt.Errorf("failed to patch status for machine inventory selector object: %w", err))
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errorutils.NewAggregate(errs)
	}

	return result, nil
}

func (r *MachineInventorySelectorReconciler) reconcile(ctx context.Context, miSelector *elementalv1.MachineInventorySelector) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Reconciling machine inventory selector object")

	if miSelector.GetDeletionTimestamp() != nil {
		// return r.reconcileDelete(ctx, machineRegistration) // TODO: add deletion logic if needed
		return ctrl.Result{}, nil
	}

	if err := r.findAndAdoptInventory(ctx, miSelector); err != nil {
		meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToAdoptInventoryReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to find and adopt machine inventory: %w", err)
	}

	if err := r.updatePlanSecretWithBootstrap(ctx, miSelector); err != nil {
		meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToUpdatePlanReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to set bootstrap plan: %w", err)
	}

	if err := r.setInvetorySelectorAddresses(ctx, miSelector); err != nil {
		meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToSetAdressesReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to set inventory selector address: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *MachineInventorySelectorReconciler) findAndAdoptInventory(ctx context.Context, miSelector *elementalv1.MachineInventorySelector) error {
	logger := ctrl.LoggerFrom(ctx)

	if miSelector.Status.MachineInventoryRef != nil {
		logger.V(5).Info("machine inventory reference already set", "machineInvetoryName", miSelector.Status.MachineInventoryRef.Name)
		return nil
	}

	logger.Info("Trying to find matching machine inventory")

	labelSelector, err := metav1.LabelSelectorAsSelector(&miSelector.Spec.Selector)
	if err != nil {
		return fmt.Errorf("failed to convert spec to a label selector: %w", err)
	}

	machineInventories := &elementalv1.MachineInventoryList{}
	if err := r.List(ctx, machineInventories, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return err
	}

	var mInventory *elementalv1.MachineInventory

	if len(machineInventories.Items) > 0 {
		for i := range machineInventories.Items {
			if isAlreadyOwned(machineInventories.Items[i]) {
				continue
			}
			mInventory = &machineInventories.Items[i]
			break
		}
	}

	if mInventory == nil {
		logger.Info("No matching machine inventories found")
		meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
			Type:   elementalv1.ReadyCondition,
			Reason: elementalv1.WaitingForInventoryReason,
			Status: metav1.ConditionFalse,
		})

		return nil
	}

	patchBase := client.MergeFrom(mInventory.DeepCopy())

	mInventory.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: elementalv1.GroupVersion.String(),
			Kind:       "MachineInventorySelector",
			Name:       miSelector.Name,
			UID:        miSelector.UID,
			Controller: pointer.Bool(true),
		},
	}

	if err := r.Patch(ctx, mInventory, patchBase); err != nil {
		return fmt.Errorf("failed to patch machine inventory: %w", err)
	}

	miSelector.Status.MachineInventoryRef = &corev1.LocalObjectReference{
		Name: mInventory.Name,
	}

	meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
		Type:   elementalv1.ReadyCondition,
		Reason: elementalv1.SuccefullyAdoptedInventoryReason,
		Status: metav1.ConditionFalse,
	})

	return nil
}

func (r *MachineInventorySelectorReconciler) updatePlanSecretWithBootstrap(ctx context.Context, miSelector *elementalv1.MachineInventorySelector) error {
	logger := ctrl.LoggerFrom(ctx)

	if miSelector.Status.MachineInventoryRef == nil {
		logger.V(5).Info("Waiting for machine inventory to be adopted before updating plan secret")
		return nil
	}

	if miSelector.Status.BootstrapPlanChecksum != "" {
		logger.V(5).Info("Secret plan already updated with bootstrap")
		return nil
	}

	mInventory := &elementalv1.MachineInventory{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: miSelector.Namespace,
		Name:      miSelector.Status.MachineInventoryRef.Name,
	},
		mInventory,
	); err != nil {
		return fmt.Errorf("failed to get machine inventory: %w", err)
	}

	if mInventory.Status.Plan == nil || mInventory.Status.Plan.PlanSecretRef == nil {
		logger.V(5).Info("Machine inventory plan reference not set yet")
		return nil
	}

	checksum, plan, err := r.newBootstrapPlan(ctx, miSelector, mInventory)
	if err != nil {
		return fmt.Errorf("failed to get bootstrap plan: %w", err)
	}

	planSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: mInventory.Status.Plan.PlanSecretRef.Namespace,
		Name:      mInventory.Status.Plan.PlanSecretRef.Name,
	}, planSecret); err != nil {
		return fmt.Errorf("failed to get plan secret: %w", err)
	}

	patchBase := client.MergeFrom(planSecret.DeepCopy())

	planSecret.Data["plan"] = plan

	if err := r.Patch(ctx, planSecret, patchBase); err != nil {
		return fmt.Errorf("failed to patch plan secret: %w", err)
	}

	miSelector.Status.BootstrapPlanChecksum = checksum

	meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
		Type:   elementalv1.ReadyCondition,
		Reason: elementalv1.SuccefullyUpdatedPlanReason,
		Status: metav1.ConditionFalse,
	})

	return nil
}

func (r *MachineInventorySelectorReconciler) newBootstrapPlan(ctx context.Context, miSelector *elementalv1.MachineInventorySelector, mInventory *elementalv1.MachineInventory) (string, []byte, error) {
	machine, err := r.getOwnerMachine(ctx, miSelector)
	if err != nil {
		return "", nil, fmt.Errorf("failed to find an owner machine for inventory selector: %w", err)
	}

	if machine == nil {
		return "", nil, fmt.Errorf("machine for machine inventory selector doesn't exist")
	}

	if machine.Spec.Bootstrap.DataSecretName == nil {
		return "", nil, fmt.Errorf("machine %s/%s missing bootstrap data secret name", machine.Namespace, machine.Name)
	}

	bootstrapSecret := &corev1.Secret{}

	if err := r.Get(ctx, types.NamespacedName{
		Namespace: machine.Namespace,
		Name:      *machine.Spec.Bootstrap.DataSecretName,
	}, bootstrapSecret); err != nil {
		return "", nil, fmt.Errorf("failed to get a boostrap plan for the machine: %w", err)
	}

	stopAgentPlan := applyinator.Plan{
		OneTimeInstructions: []applyinator.OneTimeInstruction{
			{
				CommonInstruction: applyinator.CommonInstruction{
					Command: "systemctl",
					Args: []string{
						"stop",
						"elemental-system-agent",
					},
				},
			},
		},
	}

	stopAgentPlanJSON, err := json.Marshal(stopAgentPlan)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create new stop elemental agent plan: %w", err)
	}

	type LabelsFromInventory struct {
		// NOTE: The '+' is not a typo and is needed when adding labels to k3s/rke
		// instead of replacing them.
		NodeLabels []string `yaml:"node-label+"`
	}

	nodeLabelsFromInventory := LabelsFromInventory{NodeLabels: []string{}}
	for label, value := range mInventory.Labels {
		nodeLabelsFromInventory.NodeLabels = append(nodeLabelsFromInventory.NodeLabels, fmt.Sprintf("%s=%s", label, value))
	}

	nodeLabelsFromInventoryInYaml, err := yaml.Marshal(nodeLabelsFromInventory)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal node labels from inventory: %w", err)
	}

	p := applyinator.Plan{
		Files: []applyinator.File{
			{
				Content:     base64.StdEncoding.EncodeToString(bootstrapSecret.Data["value"]),
				Path:        "/var/lib/rancher/bootstrap.sh",
				Permissions: "0700",
			},
			{
				Content:     base64.StdEncoding.EncodeToString([]byte("node-name: " + mInventory.Name)),
				Path:        "/etc/rancher/rke2/config.yaml.d/99-elemental-name.yaml",
				Permissions: "0600",
			},
			{
				Content:     base64.StdEncoding.EncodeToString(nodeLabelsFromInventoryInYaml),
				Path:        "/etc/rancher/rke2/config.yaml.d/99-elemental-inventory-labels.yaml",
				Permissions: "0600",
			},
			{
				Content:     base64.StdEncoding.EncodeToString([]byte("node-name: " + mInventory.Name)),
				Path:        "/etc/rancher/k3s/config.yaml.d/99-elemental-name.yaml",
				Permissions: "0600",
			},
			{
				Content:     base64.StdEncoding.EncodeToString(nodeLabelsFromInventoryInYaml),
				Path:        "/etc/rancher/k3s/config.yaml.d/99-elemental-inventory-labels.yaml",
				Permissions: "0600",
			},
			{
				Content:     base64.StdEncoding.EncodeToString([]byte(mInventory.Name)),
				Path:        "/usr/local/etc/hostname",
				Permissions: "0644",
			},
			{
				Content:     base64.StdEncoding.EncodeToString(stopAgentPlanJSON),
				Path:        "/var/lib/rancher/agent/plans/elemental-agent-stop.plan.skip",
				Permissions: "0644",
			},
		},
		OneTimeInstructions: []applyinator.OneTimeInstruction{
			{
				CommonInstruction: applyinator.CommonInstruction{
					Name:    "configure hostname",
					Command: "hostnamectl",
					Args: []string{
						"set-hostname",
						"--transient",
						mInventory.Name,
					},
				},
			},
			{
				CommonInstruction: applyinator.CommonInstruction{
					Command: "/var/lib/rancher/bootstrap.sh",
					// Ensure local plans will be enabled, this is required to ensure the local
					// plan stopping elemental-system-agent is executed
					Env: []string{
						"CATTLE_LOCAL_ENABLED=true",
					},
				},
			},
			{
				// Ensure the local plan can only be executed after bootstrapping script is done
				CommonInstruction: applyinator.CommonInstruction{
					Command: "bash",
					Args: []string{
						"-c",
						"mv /var/lib/rancher/agent/plans/elemental-agent-stop.plan.skip /var/lib/rancher/agent/plans/elemental-agent-stop.plan",
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(p); err != nil {
		return "", nil, fmt.Errorf("failed to encode plan: %w", err)
	}

	plan := buf.Bytes()

	checksum := planChecksum(plan)

	return checksum, plan, nil
}

func (r *MachineInventorySelectorReconciler) getOwnerMachine(ctx context.Context, miSelector *elementalv1.MachineInventorySelector) (*clusterv1.Machine, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.V(5).Info("Trying to find CAPI machine that owns machine inventory selector")
	for _, owner := range miSelector.GetOwnerReferences() {
		if owner.APIVersion == clusterv1.GroupVersion.String() && owner.Kind == "Machine" {
			logger.V(5).Info("Found owner CAPI machine for machine inventory selector", "capiMachineName", owner.Name)
			machine := &clusterv1.Machine{}
			err := r.Client.Get(ctx, types.NamespacedName{Namespace: miSelector.Namespace, Name: owner.Name}, machine)
			return machine, err
		}
	}

	return nil, nil
}

func (r *MachineInventorySelectorReconciler) setInvetorySelectorAddresses(ctx context.Context, miSelector *elementalv1.MachineInventorySelector) error {
	logger := ctrl.LoggerFrom(ctx)

	if miSelector.Status.MachineInventoryRef == nil {
		logger.V(5).Info("Waiting for machine inventory to be adopted before setting adresses")
		return nil
	}

	mInventory := &elementalv1.MachineInventory{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: miSelector.Namespace,
		Name:      miSelector.Status.MachineInventoryRef.Name,
	},
		mInventory,
	); err != nil {
		return fmt.Errorf("failed to get machine inventory: %w", err)
	}

	if val := mInventory.Labels["elemental.cattle.io/ExternalIP"]; val != "" {
		miSelector.Status.Addresses = append(miSelector.Status.Addresses, clusterv1.MachineAddress{
			Type:    clusterv1.MachineExternalIP,
			Address: val,
		})
	}

	if val := mInventory.Labels["elemental.cattle.io/InternalIP"]; val != "" {
		miSelector.Status.Addresses = append(miSelector.Status.Addresses, clusterv1.MachineAddress{
			Type:    clusterv1.MachineInternalIP,
			Address: val,
		})
	}

	if val := mInventory.Labels["elemental.cattle.io/Hostname"]; val != "" {
		miSelector.Status.Addresses = append(miSelector.Status.Addresses, clusterv1.MachineAddress{
			Type:    clusterv1.MachineHostName,
			Address: val,
		})
	}

	miSelector.Status.Ready = true

	meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
		Type:   elementalv1.ReadyCondition,
		Reason: elementalv1.SelectorReadyReason,
		Status: metav1.ConditionTrue,
	})

	return nil
}

func (r *MachineInventorySelectorReconciler) ignoreIncrementalStatusUpdate() predicate.Funcs {
	return predicate.Funcs{
		// Avoid reconciling if the event triggering the reconciliation is related to incremental status updates
		// for MachineInventorySelector resources only
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetObjectKind().GroupVersionKind().Kind != "MachineInventorySelector" {
				return true
			}

			oldMISelector := e.ObjectOld.(*elementalv1.MachineInventorySelector).DeepCopy()
			newMISelector := e.ObjectNew.(*elementalv1.MachineInventorySelector).DeepCopy()

			oldMISelector.Status = elementalv1.MachineInventorySelectorStatus{}
			newMISelector.Status = elementalv1.MachineInventorySelectorStatus{}

			oldMISelector.ObjectMeta.ResourceVersion = ""
			newMISelector.ObjectMeta.ResourceVersion = ""

			return !cmp.Equal(oldMISelector, newMISelector)
		},
	}
}

func isAlreadyOwned(machineInventory elementalv1.MachineInventory) bool {
	for _, owner := range machineInventory.GetOwnerReferences() {
		if owner.APIVersion == elementalv1.GroupVersion.String() && owner.Kind == "MachineInventorySelector" {
			return true
		}
	}

	return false
}

func planChecksum(input []byte) string {
	h := sha256.New()
	h.Write(input)

	return fmt.Sprintf("%x", h.Sum(nil))
}

// MachineInventoryToSelector is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for MachineInventoryToSelector that might adopt a MachineInventory.
func (r *MachineInventorySelectorReconciler) MachineInventoryToSelector(o client.Object) []reconcile.Request {
	result := []reconcile.Request{}

	mInventory, ok := o.(*elementalv1.MachineInventory)
	if !ok {
		panic(fmt.Sprintf("Expected a MachineInventory but got a %T", o))
	}

	// This won't log unless the global logger is set
	ctx := context.Background()
	log := ctrl.LoggerFrom(ctx, "MachineInventory", klog.KObj(mInventory))

	// If machine inventory has no labels it can't be adopted.
	if mInventory.Labels == nil {
		return nil
	}

	miSelectorList := &elementalv1.MachineInventorySelectorList{}
	err := r.List(context.Background(), miSelectorList, client.InNamespace(mInventory.Namespace))
	if err != nil {
		log.Error(err, "Failed to list machine inventories")
		return nil
	}

	var miSelectors []*elementalv1.MachineInventorySelector
	for i := range miSelectorList.Items {
		miSelector := &miSelectorList.Items[i]
		if hasMatchingLabels(ctx, miSelector, mInventory) {
			miSelectors = append(miSelectors, miSelector)
		}
	}

	result = append(result, reconcile.Request{})

	for _, miSelector := range miSelectors {
		name := client.ObjectKey{Namespace: miSelector.Namespace, Name: miSelector.Name}
		result = append(result, reconcile.Request{NamespacedName: name})
	}

	return result
}

func hasMatchingLabels(ctx context.Context, miSelector *elementalv1.MachineInventorySelector, mInventory *elementalv1.MachineInventory) bool {
	log := ctrl.LoggerFrom(ctx)

	selector, err := metav1.LabelSelectorAsSelector(&miSelector.Spec.Selector)
	if err != nil {
		log.Error(err, "Unable to convert selector")
		return false
	}

	if selector.Empty() {
		log.V(5).Info("machine selector has empty selector", "mSelector.Name", miSelector.Name)
		return false
	}

	if !selector.Matches(labels.Set(mInventory.Labels)) {
		log.V(5).Info("machine inventory has mismatch labels", "mInventory.Name", mInventory.Name, "mSelector.Name", miSelector.Name)
		return false
	}

	return true
}
