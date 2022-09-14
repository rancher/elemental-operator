package controllers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"

	"encoding/base64"
	"encoding/json"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/system-agent/pkg/applyinator"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MachineInventorySelectorReconciler reconciles a MachineInventorySelector object.
type MachineInventorySelectorReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups="management.cattle.io",resources=setting,verbs=get

func (r *MachineInventorySelectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&elementalv1.MachineInventorySelector{}).
		Complete(r)
}

func (r *MachineInventorySelectorReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
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

	// Collect errors as an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, machineInventorySelector)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reconciling machine inventory selector object: %w", err))
	}

	machineInventorySelectorStatusCopy := machineInventorySelector.Status.DeepCopy() // Patch call will erase the status

	if err := r.Patch(ctx, machineInventorySelector, patchBase); err != nil {
		errs = append(errs, fmt.Errorf("failed to patch status for machine inventory object: %w", err))
	}

	machineInventorySelector.Status = *machineInventorySelectorStatusCopy

	if err := r.Status().Patch(ctx, machineInventorySelector, patchBase); err != nil {
		errs = append(errs, fmt.Errorf("failed to patch status for machine inventory object: %w", err))
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
		// return r.reconcileDelete(ctx, machineRegistration)
		return ctrl.Result{}, nil
	}

	if err := r.findAndAdoptInventory(ctx, miSelector); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to find and adopt machine inventory: %w", err)
	}

	if err := r.updatePlanSecretWithBootstrap(ctx, miSelector); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set bootstrap plan: %w", err)
	}

	if err := r.setInvetorySelectorAddresses(ctx, miSelector); err != nil {
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

	labelSelectorMap, err := metav1.LabelSelectorAsMap(&miSelector.Spec.Selector)
	if err != nil {
		return fmt.Errorf("failed to convert label selector to map: %w", err)
	}

	machineInventories := &elementalv1.MachineInventoryList{}
	if err := r.List(ctx, machineInventories, client.MatchingLabels(labelSelectorMap)); err != nil {
		return err
	}

	if len(machineInventories.Items) == 0 {
		logger.Info("No matching machine inventories found")
		return nil
	}

	machineInventory := &elementalv1.MachineInventory{}
	for _, mi := range machineInventories.Items {
		if isAlreadyOwned(mi) {
			continue
		}
		machineInventory = &mi
		break
	}

	patchBase := client.MergeFrom(machineInventory.DeepCopy())
	machineInventoryStatusCopy := machineInventory.Status.DeepCopy() // Patch call will erase the status

	machineInventory.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: elementalv1.GroupVersion.String(),
			Kind:       "MachineInventorySelector",
			Name:       miSelector.Name,
			UID:        miSelector.UID,
			Controller: pointer.Bool(true),
		},
	}

	if err := r.Status().Patch(ctx, machineInventory, patchBase); err != nil {
		return fmt.Errorf("failed to patch machine inventory status: %w", err)
	}

	machineInventory.Status = *machineInventoryStatusCopy

	miSelector.Status.MachineInventoryRef = &corev1.ObjectReference{
		Name:      machineInventory.Name,
		Namespace: machineInventory.Namespace,
	}

	return nil
}

func (r *MachineInventorySelectorReconciler) updatePlanSecretWithBootstrap(ctx context.Context, miSelector *elementalv1.MachineInventorySelector) error {
	logger := ctrl.LoggerFrom(ctx)

	if miSelector.Status.MachineInventoryRef == nil {
		logger.V(5).Info("Waiting for machine inventory to be adopted")
		return nil
	}

	if miSelector.Status.BootstrapPlanChecksum != "" {
		logger.V(5).Info("Secret plan already update with bootstrap")
		return nil
	}

	mInventory := &elementalv1.MachineInventory{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: miSelector.Status.MachineInventoryRef.Namespace,
		Name:      miSelector.Status.MachineInventoryRef.Name,
	},
		mInventory,
	); err != nil {
		return fmt.Errorf("failed to get machine inventory: %w", err)
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
		return fmt.Errorf("failed to get plan secret %w", err)
	}

	patchBase := client.MergeFrom(planSecret.DeepCopy())

	planSecret.Data["plan"] = plan

	if err := r.Patch(ctx, planSecret, patchBase); err != nil {
		return fmt.Errorf("failed to patch plan secret: %w", err)
	}

	miSelector.Status.BootstrapPlanChecksum = checksum

	return nil
}

func (r *MachineInventorySelectorReconciler) newBootstrapPlan(ctx context.Context, miSelector *elementalv1.MachineInventorySelector, mInventory *elementalv1.MachineInventory) (string, []byte, error) {
	machine, err := r.getOwnerMachine(ctx, miSelector)
	if err != nil {
		return "", nil, fmt.Errorf("failed to find an owner machine for inventory selector: %w", err)
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

	stopAgentPlanJson, err := json.Marshal(stopAgentPlan)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create new stop elemental agent plan: %w", err)
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
				Content:     base64.StdEncoding.EncodeToString([]byte("node-name: " + mInventory.Name)),
				Path:        "/etc/rancher/k3s/config.yaml.d/99-elemental-name.yaml",
				Permissions: "0600",
			},
			{
				Content:     base64.StdEncoding.EncodeToString([]byte(mInventory.Name)),
				Path:        "/usr/local/etc/hostname",
				Permissions: "0644",
			},
			{
				Content:     base64.StdEncoding.EncodeToString(stopAgentPlanJson),
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
		logger.V(5).Info("Waiting for machine inventory to be adopted")
		return nil
	}

	mInventory := &elementalv1.MachineInventory{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: miSelector.Status.MachineInventoryRef.Namespace,
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

	return nil
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
