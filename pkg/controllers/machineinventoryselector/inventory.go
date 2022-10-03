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
	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/wrangler/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// indexOpenInventorySelector indexes selectors by all their inventory selector labels
func (h *handler) indexOpenInventorySelector(obj *v1beta1.MachineInventorySelector) ([]string, error) {
	if v1beta1.InventoryReadyCondition.IsTrue(obj) {
		return nil, nil
	}

	labelMap, err := v1.LabelSelectorAsMap(&obj.Spec.Selector)
	if err != nil {
		return nil, err
	}

	var rval []string
	for k := range labelMap {
		rval = append(rval, k)
	}

	return rval, nil
}

// inventoryReadyHandler if an machine inventory  is set on the status the controller will attempt to set an owner reference.  if this fails the inventory reference will be removed.
// if no inventory reference is found any inventory matching the selector criteria will be enqueued.
func (h *handler) inventoryReadyHandler(obj *v1beta1.MachineInventorySelector, status v1beta1.MachineInventorySelectorStatus) (v1beta1.MachineInventorySelectorStatus, error) {
	if obj == nil || !obj.DeletionTimestamp.IsZero() {
		return status, generic.ErrSkip
	}

	if status.Ready {
		v1beta1.InventoryReadyCondition.SetError(&status, v1beta1.MachineInventorySelectorReadyReason, nil)
		return status, nil
	}

	// if no inventory has been suggested try to find one
	if status.MachineInventoryRef == nil {
		labelSelectorMap, _ := v1.LabelSelectorAsMap(&obj.Spec.Selector)
		ls := labels.SelectorFromSet(labelSelectorMap)
		inventories, err := h.MachineInventoryCache.List(obj.Namespace, ls)
		if err != nil {
			return status, err
		}

		for _, inventory := range inventories {
			if hasInventorySelectorOwner(inventory) {
				continue
			}

			status.MachineInventoryRef = &corev1.ObjectReference{
				Name:      inventory.Name,
				Namespace: inventory.Namespace,
			}
		}

		v1beta1.InventoryReadyCondition.False(&status)
		v1beta1.InventoryReadyCondition.Message(&status, "waiting for machine inventory")
		v1beta1.InventoryReadyCondition.Reason(&status, v1beta1.WaitingForMachineInventoryReason)

		return status, nil
	}

	// if an inventory has been suggested attempt to adopt it
	inventory, err := h.MachineInventoryCache.Get(status.MachineInventoryRef.Namespace, status.MachineInventoryRef.Name)
	if err != nil {
		return status, err
	}

	err = h.adoptMachineInventory(obj, inventory)

	// if the inventory is already adopted abandon it
	if errors2.IsInvalid(err) {
		status.MachineInventoryRef = nil
		v1beta1.InventoryReadyCondition.False(&status)
		v1beta1.InventoryReadyCondition.Message(&status, "waiting for machine inventory")
		v1beta1.InventoryReadyCondition.Reason(&status, v1beta1.WaitingForMachineInventoryReason)

		return status, nil
	}

	// if an unexpected error occurs retry
	if err != nil {
		return status, err
	}

	// if the adoption succeeded the inventory is ready
	v1beta1.InventoryReadyCondition.SetError(&status, v1beta1.MachineInventorySelectorReadyReason, nil)
	return status, nil
}

// adoptMachineInventory attempts to set a controller owner on the inventory
func (h *handler) adoptMachineInventory(selector *v1beta1.MachineInventorySelector, inventory *v1beta1.MachineInventory) error {
	var err error

	if v1.IsControlledBy(inventory, selector) {
		return nil
	}

	newRef := *v1.NewControllerRef(selector, selector.GroupVersionKind())
	inventory.OwnerReferences = append(inventory.OwnerReferences, newRef)
	_, err = h.MachineInventories.Update(inventory)
	if err != nil {
		return err
	}

	return err
}
