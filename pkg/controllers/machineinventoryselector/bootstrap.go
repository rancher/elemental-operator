package machineinventoryselector

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/controllers/machineinventory"
	"github.com/rancher/system-agent/pkg/applyinator"
	"github.com/rancher/wrangler/pkg/generic"
)

// bootstrapReadyHandler once the `InventoryReady` condition is true the bootstrap will set the bootstrap plan and record the checksum.  Once the bootstrap checksum is applied to the machine inventory bootstrap is considered complete.
func (h *handler) bootstrapReadyHandler(obj *v1beta1.MachineInventorySelector, status v1beta1.MachineInventorySelectorStatus) (v1beta1.MachineInventorySelectorStatus, error) {
	if obj == nil || !obj.DeletionTimestamp.IsZero() {
		return status, generic.ErrSkip
	}

	if status.Ready {
		v1beta1.BootstrapReadyCondition.SetError(&status, "", nil)
		return status, nil
	}

	// we must own a machine inventory before we can bootstrap it
	if !v1beta1.InventoryReadyCondition.IsTrue(obj) {
		v1beta1.BootstrapReadyCondition.False(&status)
		v1beta1.BootstrapReadyCondition.Message(&status, "waiting for machine inventory")
		return status, nil
	}

	inventory, err := h.MachineInventoryCache.Get(status.MachineInventoryRef.Namespace, status.MachineInventoryRef.Name)
	if err != nil {
		return status, err
	}

	// if the bootstrap plan is not set, set it
	if status.BootstrapPlanChecksum == "" {
		checksum, plan, err := h.getBootstrapPlan(obj, inventory)
		if err != nil {
			return status, err
		}

		planSecret, err := h.SecretCache.Get(inventory.Status.Plan.SecretRef.Namespace, inventory.Status.Plan.SecretRef.Name)
		if err != nil {
			return status, err
		}

		planSecret.Data["plan"] = plan

		if _, err = h.Secrets.Update(planSecret); err != nil {
			return status, err
		}

		status.BootstrapPlanChecksum = checksum
	}

	// if the bootstrap plan is applied bootstrap is ready
	if inventory.Status.Plan.AppliedChecksum == status.BootstrapPlanChecksum {
		v1beta1.BootstrapReadyCondition.SetError(&status, "", nil)
		return status, nil
	}

	v1beta1.BootstrapReadyCondition.False(&status)
	v1beta1.BootstrapReadyCondition.Message(&status, "waiting for bootstrap plan to be applied")
	return status, nil
}

// getBootstrapPlan the bootstrap plan will determine the machine's ip addresses, set the hostname to the inventory name and run the bootstrap provider's script
func (h *handler) getBootstrapPlan(selector *v1beta1.MachineInventorySelector, inventory *v1beta1.MachineInventory) (string, []byte, error) {
	machine, err := h.getMachineOwner(selector)
	if err != nil {
		return "", nil, err
	}

	if machine == nil || machine.Spec.Bootstrap.DataSecretName == nil {
		return "", nil, errors.New("cannot find machine")
	}

	bootstrap, err := h.SecretCache.Get(selector.Namespace, *machine.Spec.Bootstrap.DataSecretName)
	if err != nil {
		return "", nil, err
	}

	p := applyinator.Plan{
		Files: []applyinator.File{
			{
				Content:     base64.StdEncoding.EncodeToString(bootstrap.Data["value"]),
				Path:        "/var/lib/rancher/bootstrap.sh",
				Permissions: "0700",
			},
			{
				Path:        "/etc/rancher/rke2/config.yaml.d/40-elemental.yaml",
				Permissions: "0600",
				Content:     base64.StdEncoding.EncodeToString([]byte("kubelet-arg: provider-id=" + selector.Spec.ProviderID)),
			},
		},
		OneTimeInstructions: []applyinator.OneTimeInstruction{
			{
				CommonInstruction: applyinator.CommonInstruction{
					Name:    "configure hostname",
					Command: "hostnamectl",
					Args: []string{
						"set-hostname",
						inventory.Name,
					},
				},
			},
			{
				CommonInstruction: applyinator.CommonInstruction{
					Command: "bash",
					Args: []string{
						"-c",
						"elemental-operator register --label \"elemental.cattle.io/ExternalIP=$(hostname -I | awk '{print $1}')\" --label \"elemental.cattle.io/InternalIP=$(hostname -I | awk '{print $2}')\"",
					},
				},
			},
			{
				CommonInstruction: applyinator.CommonInstruction{
					Command: "/var/lib/rancher/bootstrap.sh",
				},
			},
		},
	}

	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(p)
	plan := buf.Bytes()

	checksum := machineinventory.PlanChecksum(plan)

	return checksum, plan, nil
}
