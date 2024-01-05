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
	"fmt"
	"time"

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
	"github.com/rancher/elemental-operator/pkg/log"
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
		WithEventFilter(filterSelectorUpdateEvents()).
		Complete(r)
}

func (r *MachineInventorySelectorReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) { //nolint:dupl
	logger := ctrl.LoggerFrom(ctx)

	machineInventorySelector := &elementalv1.MachineInventorySelector{}
	err := r.Get(ctx, req.NamespacedName, machineInventorySelector)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(log.DebugDepth).Info("Object was not found, not an error")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get machine inventory selector object: %w", err)
	}

	// Ensure we patch the latest version otherwise we could erratically
	// overwrite the adopted reference when it was already set
	patchBase := client.MergeFromWithOptions(machineInventorySelector.DeepCopy(), client.MergeFromWithOptimisticLock{})

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
		errs = append(errs, fmt.Errorf("failed to patch machine inventory selector object: %w", err))
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
		return ctrl.Result{}, nil
	}

	var mInventory *elementalv1.MachineInventory

	if err := r.findAndAdoptInventory(ctx, miSelector, mInventory); err != nil {
		meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToAdoptInventoryReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to find and adopt machine inventory: %w", err)
	}

	requeue, err := r.updateAdoptionStatus(ctx, miSelector, mInventory)
	if err != nil {
		meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
			Type:    elementalv1.InventoryReadyCondition,
			Reason:  elementalv1.FailedToAdoptInventoryReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to set adoption status: %w", err)
	}

	if err := r.updatePlanSecretWithBootstrap(ctx, miSelector, mInventory); err != nil {
		meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToUpdatePlanReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to set bootstrap plan: %w", err)
	}

	if err := r.setInvetorySelectorAddresses(ctx, miSelector, mInventory); err != nil {
		meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.FailedToSetAdressesReason,
			Status:  metav1.ConditionFalse,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("failed to set inventory selector address: %w", err)
	}

	if requeue {
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *MachineInventorySelectorReconciler) findAndAdoptInventory(ctx context.Context, miSelector *elementalv1.MachineInventorySelector, mInventory *elementalv1.MachineInventory) error {
	logger := ctrl.LoggerFrom(ctx)

	if miSelector.Status.MachineInventoryRef != nil {
		logger.V(log.DebugDepth).Info("machine inventory reference already set", "machineInvetoryName", miSelector.Status.MachineInventoryRef.Name)
		return nil
	}

	logger.V(log.DebugDepth).Info("Trying to find matching machine inventory")
	meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
		Type:   elementalv1.ReadyCondition,
		Reason: elementalv1.WaitingForInventoryReason,
		Status: metav1.ConditionFalse,
	})

	labelSelector, err := metav1.LabelSelectorAsSelector(&miSelector.Spec.Selector)
	if err != nil {
		return fmt.Errorf("failed to convert spec to a label selector: %w", err)
	}

	machineInventories := &elementalv1.MachineInventoryList{}
	if err := r.List(ctx, machineInventories, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return err
	}

	for i := range machineInventories.Items {
		if !isAlreadyOwned(&machineInventories.Items[i]) {
			mInventory = &machineInventories.Items[i]
			break
		}
	}

	if mInventory == nil {
		logger.V(log.DebugDepth).Info("No matching machine inventories found")
		return nil
	}

	// Ensure we patch the latest version otherwise we could erratically
	// set the ownership of an already owned inventory
	patchBase := client.MergeFromWithOptions(mInventory.DeepCopy(), client.MergeFromWithOptimisticLock{})

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

	logger.Info("Inventory adoption started")
	meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
		Type:               elementalv1.InventoryReadyCondition,
		Reason:             elementalv1.WaitForInventoryCheckReason,
		Status:             metav1.ConditionUnknown,
		LastTransitionTime: metav1.NewTime(time.Now()),
	})

	return nil
}

func (r *MachineInventorySelectorReconciler) updateAdoptionStatus(ctx context.Context, miSelector *elementalv1.MachineInventorySelector, mInventory *elementalv1.MachineInventory) (bool, error) {
	logger := ctrl.LoggerFrom(ctx)

	if miSelector.Status.MachineInventoryRef == nil {
		logger.V(log.DebugDepth).Info("Waiting for a machine inventory match")
		return false, nil
	}

	inventoryReady := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.InventoryReadyCondition)
	if inventoryReady == nil {
		return false, fmt.Errorf("missing required InventoryReadyCondition it must be already set at this phase")
	}
	if inventoryReady.Status == metav1.ConditionTrue {
		logger.V(log.DebugDepth).Info("Machine inventory is successfully adopted already")
		return false, nil
	}

	if mInventory == nil {
		mInventory = &elementalv1.MachineInventory{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: miSelector.Namespace,
			Name:      miSelector.Status.MachineInventoryRef.Name,
		},
			mInventory,
		); err != nil {
			return false, fmt.Errorf("failed to get machine inventory: %w", err)
		}
	}

	// Check the machine inventory ownership is sane and it is successfully adopted
	adoptedCondition := meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.AdoptionReadyCondition)
	owner := getSelectorOwner(mInventory)
	orphanInventory := owner == nil || adoptedCondition == nil || adoptedCondition.Status != metav1.ConditionTrue
	deadLine := inventoryReady.LastTransitionTime.Add(adoptionTimeout * time.Second)
	now := time.Now()

	switch {
	case owner != nil && owner.Name != miSelector.Name:
		miSelector.Status.MachineInventoryRef = nil
		return false, fmt.Errorf("machine inventory ownership mismatch detected, restart adoption")
	case orphanInventory && now.After(deadLine):
		miSelector.Status.MachineInventoryRef = nil
		return false, fmt.Errorf("machine inventory adoption validation timeout, restart adoption. Deadline was: %v", deadLine)
	case orphanInventory:
		logger.V(log.DebugDepth).Info("Machine inventory adoption not completed")
		meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
			Type:   elementalv1.InventoryReadyCondition,
			Reason: elementalv1.WaitForInventoryCheckReason,
			Status: metav1.ConditionUnknown,
		})
		return true, nil
	default:
		logger.Info("Machine inventory adoption successfully completed")
		meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
			Type:   elementalv1.InventoryReadyCondition,
			Reason: elementalv1.SuccessfullyAdoptedInventoryReason,
			Status: metav1.ConditionTrue,
		})
		return false, nil
	}
}

func (r *MachineInventorySelectorReconciler) updatePlanSecretWithBootstrap(ctx context.Context, miSelector *elementalv1.MachineInventorySelector, mInventory *elementalv1.MachineInventory) error {
	logger := ctrl.LoggerFrom(ctx)

	inventoryReady := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.InventoryReadyCondition)
	if inventoryReady == nil || inventoryReady.Status != metav1.ConditionTrue {
		logger.V(log.DebugDepth).Info("Waiting for machine inventory to be adopted before updating plan secret")
		return nil
	}

	if miSelector.Status.BootstrapPlanChecksum != "" {
		logger.V(log.DebugDepth).Info("Secret plan already updated with bootstrap")
		return nil
	}

	if mInventory == nil {
		mInventory = &elementalv1.MachineInventory{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: miSelector.Namespace,
			Name:      miSelector.Status.MachineInventoryRef.Name,
		},
			mInventory,
		); err != nil {
			return fmt.Errorf("failed to get machine inventory: %w", err)
		}
	}

	if mInventory.Status.Plan == nil || mInventory.Status.Plan.PlanSecretRef == nil {
		logger.V(log.DebugDepth).Info("Machine inventory plan reference not set yet")
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
	planSecret.Annotations = map[string]string{elementalv1.PlanTypeAnnotation: elementalv1.PlanTypeBootstrap}

	if err := r.Patch(ctx, planSecret, patchBase); err != nil {
		return fmt.Errorf("failed to patch plan secret: %w", err)
	}

	miSelector.Status.BootstrapPlanChecksum = checksum

	logger.Info("Machine inventory plan updated")
	meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
		Type:   elementalv1.ReadyCondition,
		Reason: elementalv1.SuccessfullyUpdatedPlanReason,
		Status: metav1.ConditionFalse,
	})

	return nil
}

func (r *MachineInventorySelectorReconciler) newBootstrapPlan(ctx context.Context, miSelector *elementalv1.MachineInventorySelector, mInventory *elementalv1.MachineInventory) (string, []byte, error) {
	logger := ctrl.LoggerFrom(ctx)

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

	logger.V(log.DebugDepth).Info("setting a bootstrap plan for the selector")
	bootstrapSecret := &corev1.Secret{}

	if err := r.Get(ctx, types.NamespacedName{
		Namespace: machine.Namespace,
		Name:      *machine.Spec.Bootstrap.DataSecretName,
	}, bootstrapSecret); err != nil {
		return "", nil, fmt.Errorf("failed to get a boostrap plan for the machine: %w", err)
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
					Command: "bash",
					Args: []string{
						"-c", "[ -f /var/lib/rancher/bootstrap_done ] || /var/lib/rancher/bootstrap.sh && touch /var/lib/rancher/bootstrap_done",
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

	checksum := util.PlanChecksum(plan)

	return checksum, plan, nil
}

func (r *MachineInventorySelectorReconciler) getOwnerMachine(ctx context.Context, miSelector *elementalv1.MachineInventorySelector) (*clusterv1.Machine, error) {
	logger := ctrl.LoggerFrom(ctx)

	logger.V(log.DebugDepth).Info("Trying to find CAPI machine that owns machine inventory selector")
	for _, owner := range miSelector.GetOwnerReferences() {
		if owner.APIVersion == clusterv1.GroupVersion.String() && owner.Kind == "Machine" {
			logger.V(log.DebugDepth).Info("Found owner CAPI machine for machine inventory selector", "capiMachineName", owner.Name)
			machine := &clusterv1.Machine{}
			err := r.Client.Get(ctx, types.NamespacedName{Namespace: miSelector.Namespace, Name: owner.Name}, machine)
			return machine, err
		}
	}

	return nil, nil
}

func (r *MachineInventorySelectorReconciler) setInvetorySelectorAddresses(ctx context.Context, miSelector *elementalv1.MachineInventorySelector, mInventory *elementalv1.MachineInventory) error {
	logger := ctrl.LoggerFrom(ctx)

	if miSelector.Status.MachineInventoryRef == nil {
		logger.V(log.DebugDepth).Info("Waiting for machine inventory to be adopted before setting adresses")
		return nil
	}

	if miSelector.Status.BootstrapPlanChecksum == "" {
		logger.V(log.DebugDepth).Info("Waiting for the bootstrap plan to be created")
		return nil
	}

	if mInventory == nil {
		mInventory = &elementalv1.MachineInventory{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: miSelector.Namespace,
			Name:      miSelector.Status.MachineInventoryRef.Name,
		},
			mInventory,
		); err != nil {
			return fmt.Errorf("failed to get machine inventory: %w", err)
		}
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

	logger.Info("Inventory selector is ready")
	miSelector.Status.Ready = true
	meta.SetStatusCondition(&miSelector.Status.Conditions, metav1.Condition{
		Type:   elementalv1.ReadyCondition,
		Reason: elementalv1.SelectorReadyReason,
		Status: metav1.ConditionTrue,
	})

	return nil
}

func filterSelectorUpdateEvents() predicate.Funcs {
	return predicate.Funcs{

		UpdateFunc: func(e event.UpdateEvent) bool {
			logger := ctrl.LoggerFrom(context.Background())

			if oldS, ok := e.ObjectOld.(*elementalv1.MachineInventorySelector); ok {
				// Avoid reconciling if the event triggering the reconciliation is related to
				// incremental status updates for MachineInventorySelector resources only
				oldS = oldS.DeepCopy()
				newS := e.ObjectNew.(*elementalv1.MachineInventorySelector).DeepCopy()

				// Ignore all fields that might be updated on a status update
				oldS.Status = elementalv1.MachineInventorySelectorStatus{}
				newS.Status = elementalv1.MachineInventorySelectorStatus{}
				oldS.ObjectMeta.ResourceVersion = ""
				newS.ObjectMeta.ResourceVersion = ""
				oldS.ManagedFields = []metav1.ManagedFieldsEntry{}
				newS.ManagedFields = []metav1.ManagedFieldsEntry{}

				update := !cmp.Equal(oldS, newS)
				if !update {
					logger.V(log.DebugDepth).Info("Ignoring status update", "MISelector", oldS.Name)
				}
				return update
			}
			if oldI, ok := e.ObjectOld.(*elementalv1.MachineInventory); ok {
				// Avoid reconciling if the event triggering the reconciliation is related to
				// adopting an inventory resource
				newI := e.ObjectNew.(*elementalv1.MachineInventory)
				update := len(newI.ObjectMeta.OwnerReferences) <= len(oldI.ObjectMeta.OwnerReferences)
				if !update {
					logger.V(log.DebugDepth).Info("Ignoring new additional owner update", "MInventory", oldI.Name)
				}
				return update
			}
			// Return true in case it watches other types
			return true
		},
	}
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
	logger := ctrl.LoggerFrom(ctx, "MachineInventory", klog.KObj(mInventory))

	// If machine inventory is already owned reconcile its owner
	if owner := getSelectorOwner(mInventory); owner != nil {
		return append(result, reconcile.Request{
			NamespacedName: client.ObjectKey{
				Name:      owner.Name,
				Namespace: mInventory.Namespace,
			},
		})
	}

	// If machine inventory has no labels it can't be adopted.
	if mInventory.Labels == nil {
		return result
	}

	miSelectorList := &elementalv1.MachineInventorySelectorList{}
	err := r.List(ctx, miSelectorList, client.InNamespace(mInventory.Namespace))
	if err != nil {
		logger.Error(err, "failed to list machine inventories")
		return result
	}

	for i := range miSelectorList.Items {
		if hasMatchingLabels(ctx, &miSelectorList.Items[i], mInventory) {
			result = append(result, reconcile.Request{
				NamespacedName: client.ObjectKey{
					Namespace: miSelectorList.Items[i].Namespace, Name: miSelectorList.Items[i].Name,
				},
			})
		}
	}

	return result
}

func hasMatchingLabels(ctx context.Context, miSelector *elementalv1.MachineInventorySelector, mInventory *elementalv1.MachineInventory) bool {
	logger := ctrl.LoggerFrom(ctx)

	selector, err := metav1.LabelSelectorAsSelector(&miSelector.Spec.Selector)
	if err != nil {
		log.Error(err, "Unable to convert selector")
		return false
	}

	if selector.Empty() {
		logger.V(log.DebugDepth).Info("machine selector has empty selector", "mSelector.Name", miSelector.Name)
		return false
	}

	if !selector.Matches(labels.Set(mInventory.Labels)) {
		logger.V(log.DebugDepth).Info("machine inventory has mismatch labels", "mInventory.Name", mInventory.Name, "mSelector.Name", miSelector.Name)
		return false
	}

	return true
}
