/*
Copyright © 2022 SUSE LLC

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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// bootstrapReadyHandler once the `InventoryReady` condition is true the bootstrap will set the bootstrap plan and record the checksum.  Once the bootstrap checksum is applied to the machine inventory bootstrap is considered complete.
func (h *handler) bootstrapReadyHandler(obj *v1beta1.MachineInventorySelector, status v1beta1.MachineInventorySelectorStatus) (v1beta1.MachineInventorySelectorStatus, error) {
	if obj == nil || !obj.DeletionTimestamp.IsZero() {
		return status, generic.ErrSkip
	}

	if status.Ready {
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type:   v1beta1.BootstrapReadyCondition,
			Reason: "BootstrapReady",
			Status: metav1.ConditionTrue,
		})
		return status, nil
	}

	// we must own a machine inventory before we can bootstrap it
	if !meta.IsStatusConditionTrue(obj.Status.Conditions, v1beta1.InventoryReadyCondition) {
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type:    v1beta1.BootstrapReadyCondition,
			Reason:  "Waiting for the machine inventory",
			Status:  metav1.ConditionFalse,
			Message: "waiting for machine inventory",
		})

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
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type:   v1beta1.BootstrapReadyCondition,
			Reason: "BootstrapReady",
			Status: metav1.ConditionTrue,
		})
		return status, nil
	}

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    v1beta1.BootstrapReadyCondition,
		Reason:  "WaitingForBootstrap",
		Status:  metav1.ConditionFalse,
		Message: "waiting for bootstrap plan to be applied",
	})

	return status, nil
}

// getStopElementalAgentLocalPlan returns the local plan to stop elemental-system-agent service in json format
func getStopElementalAgentLocalPlan() ([]byte, error) {
	stopElementalAgent := applyinator.Plan{
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
	return json.Marshal(stopElementalAgent)
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

	stopAgentPlan, err := getStopElementalAgentLocalPlan()
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
				Content:     base64.StdEncoding.EncodeToString([]byte("node-name: " + inventory.Name)),
				Path:        "/etc/rancher/rke2/config.yaml.d/99-elemental-name.yaml",
				Permissions: "0600",
			},
			{
				Content:     base64.StdEncoding.EncodeToString([]byte("node-name: " + inventory.Name)),
				Path:        "/etc/rancher/k3s/config.yaml.d/99-elemental-name.yaml",
				Permissions: "0600",
			},
			{
				Content:     base64.StdEncoding.EncodeToString([]byte(inventory.Name)),
				Path:        "/usr/local/etc/hostname",
				Permissions: "0644",
			},
			{
				Content:     base64.StdEncoding.EncodeToString(stopAgentPlan),
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
						inventory.Name,
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
	_ = json.NewEncoder(&buf).Encode(p)
	plan := buf.Bytes()

	checksum := machineinventory.PlanChecksum(plan)

	return checksum, plan, nil
}
