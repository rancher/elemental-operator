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
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/rancher/system-agent/pkg/applyinator"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/controllers/machineinventory"
)

// bootstrapReadyHandler once the `InventoryReady` condition is true the bootstrap will set the bootstrap plan and record the checksum.  Once the bootstrap checksum is applied to the machine inventory bootstrap is considered complete.
func (h *handler) bootstrapReadyHandler(obj *v1beta1.MachineInventorySelector, status v1beta1.MachineInventorySelectorStatus) (v1beta1.MachineInventorySelectorStatus, error) {
	if obj == nil || !obj.DeletionTimestamp.IsZero() {
		return status, generic.ErrSkip
	}

	if status.Ready {
		v1beta1.BootstrapReadyCondition.SetError(&status, v1beta1.BootstrapReadyReason, nil)
		return status, nil
	}

	// we must own a machine inventory before we can bootstrap it
	if !v1beta1.InventoryReadyCondition.IsTrue(obj) {
		v1beta1.BootstrapReadyCondition.False(&status)
		v1beta1.BootstrapReadyCondition.Message(&status, "waiting for machine inventory")
		v1beta1.BootstrapReadyCondition.Reason(&status, v1beta1.WaitingForMachineInventoryReason)
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
		v1beta1.BootstrapReadyCondition.SetError(&status, v1beta1.BootstrapReadyReason, nil)

		if inventory.Status.Plan.FailedChecksum == status.BootstrapPlanChecksum {
			logrus.Warn("boostrap plan failed...")
			return status, nil
		}

		logrus.Info("bootstrap plan succeeded, setting plan to stop elemental-system-agent")
		plan, err := getStopElementalAgentPlan()
		if err != nil {
			logrus.Warn("failed creating stop elemental-system-agent plan")
			return status, nil
		}

		planSecret, err := h.SecretCache.Get(inventory.Status.Plan.SecretRef.Namespace, inventory.Status.Plan.SecretRef.Name)
		if err != nil {
			logrus.Warn("failed to get plan secret, stop elemental-system-agent plan is not set")
			return status, nil
		}

		planSecret.Data["plan"] = plan

		if _, err := h.Secrets.Update(planSecret); err != nil {
			logrus.Warn("failed to update plan secret, stop elemental-system-agent plan is not set")
		}
		return status, nil
	}

	v1beta1.BootstrapReadyCondition.False(&status)
	v1beta1.BootstrapReadyCondition.Message(&status, "waiting for bootstrap plan to be applied")
	v1beta1.BootstrapReadyCondition.Reason(&status, v1beta1.WaitingForBootstrapReason)
	return status, nil
}

// getStopElementalAgentPlan returns the local plan to stop elemental-system-agent service in json format
func getStopElementalAgentPlan() ([]byte, error) {
	stopElementalAgent := applyinator.Plan{
		OneTimeInstructions: []applyinator.OneTimeInstruction{
			{
				CommonInstruction: applyinator.CommonInstruction{
					Command: "systemctl",
					Args: []string{
						"--no-block",
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

	type LabelsFromInventory struct {
		// NOTE: The '+' is not a typo and is needed when adding labels to k3s/rke
		// instead of replacing them.
		NodeLabels []string `yaml:"node-label+"`
	}

	nodeLabelsFromInventory := LabelsFromInventory{NodeLabels: []string{}}
	for label, value := range inventory.Labels {
		nodeLabelsFromInventory.NodeLabels = append(nodeLabelsFromInventory.NodeLabels, fmt.Sprintf("%s=%s", label, value))
	}

	nodeLabelsFromInventoryInYaml, err := yaml.Marshal(nodeLabelsFromInventory)
	if err != nil {
		logrus.Warnf("Could not decode the inventory labels to add them to the node: %s", err)
		nodeLabelsFromInventoryInYaml = []byte("")
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
				Content:     base64.StdEncoding.EncodeToString(nodeLabelsFromInventoryInYaml),
				Path:        "/etc/rancher/rke2/config.yaml.d/99-elemental-inventory-labels.yaml",
				Permissions: "0600",
			},
			{
				Content:     base64.StdEncoding.EncodeToString([]byte("node-name: " + inventory.Name)),
				Path:        "/etc/rancher/k3s/config.yaml.d/99-elemental-name.yaml",
				Permissions: "0600",
			},
			{
				Content:     base64.StdEncoding.EncodeToString(nodeLabelsFromInventoryInYaml),
				Path:        "/etc/rancher/k3s/config.yaml.d/99-elemental-inventory-labels.yaml",
				Permissions: "0600",
			},
			{
				Content:     base64.StdEncoding.EncodeToString([]byte(inventory.Name)),
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
						inventory.Name,
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

	checksum := planChecksum(plan)

	return checksum, plan, nil
}

func planChecksum(input []byte) string {
	h := sha256.New()
	h.Write(input)

	return fmt.Sprintf("%x", h.Sum(nil))
}
