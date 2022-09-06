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

package machineinventory

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	elemental "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
	v1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	controllerName            = "machine-inventory"
	planMachineInventoryIndex = "plan-machine-inventory-index"
)

type handler struct {
	ctx                   context.Context
	machineInventories    elemental.MachineInventoryClient
	MachineInventoryCache elemental.MachineInventoryCache
	Secrets               v1.SecretClient
	SecretCache           v1.SecretCache
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := handler{
		ctx:                   ctx,
		machineInventories:    clients.Elemental.MachineInventory(),
		MachineInventoryCache: clients.Elemental.MachineInventory().Cache(),
		Secrets:               clients.Core.Secret(),
		SecretCache:           clients.Core.Secret().Cache(),
	}

	elemental.RegisterMachineInventoryGeneratingHandler(
		ctx,
		clients.Elemental.MachineInventory(),
		clients.Apply.WithNoDelete().WithCacheTypes(clients.Core.Secret()).WithSetOwnerReference(true, true),
		"",
		controllerName+"-generate",
		h.initializeHandler,
		nil)

	elemental.RegisterMachineInventoryStatusHandler(
		ctx,
		clients.Elemental.MachineInventory(),
		"",
		controllerName+"-ready",
		h.readyHandler)

	elemental.RegisterMachineInventoryStatusHandler(
		ctx,
		clients.Elemental.MachineInventory(),
		"",
		controllerName+"-plan-applied",
		h.planReadyHandler)

	clients.Elemental.MachineInventory().Cache().AddIndexer(planMachineInventoryIndex, h.planMachineInventoryIndexer)

	clients.Core.Secret().OnChange(ctx, controllerName, h.onInventoryPlanChange)
}

// planMachineInventoryIndexer index machine inventories by their plan reference
func (h *handler) planMachineInventoryIndexer(obj *v1beta1.MachineInventory) ([]string, error) {
	if !v1beta1.InitializedCondition.IsTrue(obj) {
		return nil, nil
	}

	return []string{obj.Status.Plan.SecretRef.Namespace + ":" + obj.Status.Plan.SecretRef.Name}, nil
}

// initializeHandler creates the machine inventory plan secret and sets the initial plan checksum
func (h *handler) initializeHandler(obj *v1beta1.MachineInventory, status v1beta1.MachineInventoryStatus) ([]runtime.Object, v1beta1.MachineInventoryStatus, error) {
	if obj == nil || obj.DeletionTimestamp != nil {
		return nil, status, generic.ErrSkip
	}

	// ensure we only initialize the machine once
	if v1beta1.InitializedCondition.IsTrue(obj) {
		return nil, status, generic.ErrSkip
	}

	// create the plan secret
	plan := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: obj.Namespace,
			Name:      obj.Name,
		},
		Type:       v1beta1.PlanSecretType,
		StringData: map[string]string{"plan": "{}"},
	}

	status.Plan = &v1beta1.PlanStatus{
		SecretRef: &corev1.ObjectReference{
			Namespace: plan.Namespace,
			Name:      plan.Name,
		},
		Checksum: PlanChecksum([]byte(plan.StringData["plan"])),
	}

	v1beta1.InitializedCondition.SetError(&status, "", nil)

	return []runtime.Object{plan}, status, nil
}

// readyHandler inventories are considered ready when they are initialized and do not have any pending plans
func (h *handler) readyHandler(obj *v1beta1.MachineInventory, status v1beta1.MachineInventoryStatus) (v1beta1.MachineInventoryStatus, error) {
	if !v1beta1.InitializedCondition.IsTrue(obj) {
		v1beta1.ReadyCondition.False(&status)
		v1beta1.ReadyCondition.Message(&status, "waiting for initialization")
		return status, nil
	}

	if !v1beta1.PlanReadyCondition.IsTrue(obj) {
		v1beta1.ReadyCondition.False(&status)
		v1beta1.ReadyCondition.Message(&status, "waiting for plan to be applied")
		return status, nil
	}

	v1beta1.ReadyCondition.SetError(&status, "", nil)
	return status, nil
}

// planReadyHandler a plan is ready if it fails or is applied.  It is up to the user to verfiy the plan was successful.
func (h *handler) planReadyHandler(obj *v1beta1.MachineInventory, status v1beta1.MachineInventoryStatus) (v1beta1.MachineInventoryStatus, error) {
	if obj == nil || obj.DeletionTimestamp != nil {
		return status, generic.ErrSkip
	}

	if status.Plan == nil {
		return status, generic.ErrSkip
	}

	switch status.Plan.Checksum {
	case status.Plan.FailedChecksum:
		v1beta1.PlanReadyCondition.SetError(&status, "", nil)
		return status, nil
	case status.Plan.AppliedChecksum:
		v1beta1.PlanReadyCondition.SetError(&status, "", nil)
		status.Plan.FailedChecksum = ""
		return status, nil
	default:
		v1beta1.PlanReadyCondition.False(&status)
		v1beta1.PlanReadyCondition.Message(&status, "waiting for plan to be applied")
		return status, nil
	}
}

// onInventoryPlanChange watch for plan changes and copy the checksums to the machine inventory statys
func (h *handler) onInventoryPlanChange(_ string, plan *corev1.Secret) (*corev1.Secret, error) {
	if plan == nil || plan.DeletionTimestamp != nil {
		return nil, nil
	}

	if plan.Type != v1beta1.PlanSecretType {
		return nil, nil
	}

	inventories, err := h.MachineInventoryCache.GetByIndex(planMachineInventoryIndex, plan.Namespace+":"+plan.Name)
	if err != nil {
		return nil, err
	}

	if len(inventories) != 1 {
		return nil, errors.New("failed to find machine inventory for plan")
	}

	inventory := inventories[0]
	inventory.Status.Plan.Checksum = PlanChecksum(plan.Data["plan"])
	inventory.Status.Plan.AppliedChecksum = string(plan.Data["applied-checksum"])
	inventory.Status.Plan.FailedChecksum = string(plan.Data["failed-checksum"])

	_, err = h.machineInventories.UpdateStatus(inventory)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func PlanChecksum(input []byte) string {
	h := sha256.New()
	h.Write(input)

	return fmt.Sprintf("%x", h.Sum(nil))
}
