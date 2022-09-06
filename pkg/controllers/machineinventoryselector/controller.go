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
	"context"
	"time"

	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	capi "github.com/rancher/elemental-operator/pkg/generated/controllers/cluster.x-k8s.io/v1beta1"
	elm "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
	core "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1beta12 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	ControllerName             = "machine-inventory-selector"
	OpenInventorySelectorIndex = "OpenInventorySelector"
)

type handler struct {
	MachineInventoryEnqueue              func(string, string)
	MachineInventorySelectorEnqueueAfter func(string, string, time.Duration)
	MachineInventoryCache                elm.MachineInventoryCache
	MachineInventories                   elm.MachineInventoryClient
	MachineInventorySelectorCache        elm.MachineInventorySelectorCache
	MachineInventorySelectors            elm.MachineInventorySelectorClient
	MachineCache                         capi.MachineCache
	SecretCache                          core.SecretCache
	Secrets                              core.SecretClient
}

func Register(ctx context.Context, clients clients.ClientInterface) {
	h := &handler{
		MachineInventoryEnqueue:              clients.Elemental().MachineInventory().Enqueue,
		MachineInventorySelectorEnqueueAfter: clients.Elemental().MachineInventorySelector().EnqueueAfter,
		MachineInventoryCache:                clients.Elemental().MachineInventory().Cache(),
		MachineInventories:                   clients.Elemental().MachineInventory(),
		MachineInventorySelectorCache:        clients.Elemental().MachineInventorySelector().Cache(),
		MachineInventorySelectors:            clients.Elemental().MachineInventorySelector(),
		MachineCache:                         clients.CAPI().Machine().Cache(),
		SecretCache:                          clients.Core().Secret().Cache(),
		Secrets:                              clients.Core().Secret(),
	}

	// indexers
	clients.Elemental().MachineInventorySelector().Cache().AddIndexer(OpenInventorySelectorIndex, h.indexOpenInventorySelector)

	/// MachineInventorySelector status handlers
	elm.RegisterMachineInventorySelectorStatusHandler(ctx, clients.Elemental().MachineInventorySelector(), "", ControllerName+"-ready", h.readyHandler)
	elm.RegisterMachineInventorySelectorStatusHandler(ctx, clients.Elemental().MachineInventorySelector(), "", ControllerName+"-inventory-ready", h.inventoryReadyHandler)
	elm.RegisterMachineInventorySelectorStatusHandler(ctx, clients.Elemental().MachineInventorySelector(), "", ControllerName+"-bootstrap", h.bootstrapReadyHandler)

	// Inventory handlers
	clients.Elemental().MachineInventory().OnChange(ctx, ControllerName, h.onInventoryChange)
}

// readyHandler waits until an inventory has been adopted and bootstrapped. The provider id, addresses and status ready will only be set once.  Once a selector is ready
// the operator will not make any more changes and assumes the cluster provisioned will manage the machine from here out.
func (h *handler) readyHandler(obj *v1beta1.MachineInventorySelector, status v1beta1.MachineInventorySelectorStatus) (v1beta1.MachineInventorySelectorStatus, error) {
	if obj == nil || !obj.DeletionTimestamp.IsZero() {
		return status, generic.ErrSkip
	}

	if status.Ready {
		v1beta1.ReadyCondition.SetError(&status, "", nil)
		return status, nil
	}

	if !v1beta1.InventoryReadyCondition.IsTrue(obj) {
		v1beta1.ReadyCondition.False(&status)
		v1beta1.ReadyCondition.Message(&status, "waiting for machine inventory")
		return status, nil
	}

	if !v1beta1.BootstrapReadyCondition.IsTrue(obj) {
		v1beta1.ReadyCondition.False(&status)
		v1beta1.ReadyCondition.Message(&status, "waiting for bootstrap to be applied")
		return status, nil
	}

	inventory, err := h.MachineInventoryCache.Get(status.MachineInventoryRef.Namespace, status.MachineInventoryRef.Name)
	if err != nil {
		return status, err
	}

	obj, err = h.MachineInventorySelectors.Update(obj)
	if err != nil {
		return status, err
	}

	status = obj.Status

	status.Ready = true

	if val := inventory.Labels["elemental.cattle.io/ExternalIP"]; val != "" {
		status.Addresses = append(status.Addresses, v1beta12.MachineAddress{
			Type:    v1beta12.MachineExternalIP,
			Address: val,
		})
	}

	if val := inventory.Labels["elemental.cattle.io/InternalIP"]; val != "" {
		status.Addresses = append(status.Addresses, v1beta12.MachineAddress{
			Type:    v1beta12.MachineInternalIP,
			Address: val,
		})
	}

	if val := inventory.Labels["elemental.cattle.io/Hostname"]; val != "" {
		status.Addresses = append(status.Addresses, v1beta12.MachineAddress{
			Type:    v1beta12.MachineHostName,
			Address: val,
		})
	}

	v1beta1.ReadyCondition.SetError(&status, "", nil)
	return status, nil
}

// onInventoryChange will enqueue it's inventory selector if one is found, otherwise it will search for an inventory selector to adopt it.
func (h *handler) onInventoryChange(_ string, obj *v1beta1.MachineInventory) (*v1beta1.MachineInventory, error) {
	if obj == nil || !obj.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	if !v1beta1.ReadyCondition.IsTrue(obj) {
		return nil, generic.ErrSkip
	}

	selector, err := h.getInventorySelectorOwner(obj)
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, err
	}

	if selector != nil {
		h.MachineInventorySelectorEnqueueAfter(selector.Namespace, selector.Name, 0)
		return nil, nil
	}

	// find the first selector that matches this inventory
	var match *v1beta1.MachineInventorySelector
	for l := range obj.Labels {
		selectors, err := h.MachineInventorySelectorCache.GetByIndex(OpenInventorySelectorIndex, l)
		if err != nil {
			return nil, err
		}

		for _, selector := range selectors {
			labelSelectorMap, _ := metav1.LabelSelectorAsMap(&selector.Spec.Selector)
			if labels.SelectorFromSet(labelSelectorMap).Matches(labels.Set(obj.Labels)) {
				match = selector
				break
			}
		}

		if match != nil {
			break
		}
	}

	if match == nil {
		return nil, nil
	}

	match.Status.MachineInventoryRef = &corev1.ObjectReference{
		Namespace: obj.Namespace,
		Name:      obj.Name,
	}

	_, err = h.MachineInventorySelectors.UpdateStatus(match)

	return nil, err
}

func hasInventorySelectorOwner(obj metav1.Object) bool {
	for _, owner := range obj.GetOwnerReferences() {
		if owner.APIVersion == v1beta1.SchemeGroupVersion.String() && owner.Kind == "MachineInventorySelector" {
			return true
		}
	}

	return false
}

func (h *handler) getInventorySelectorOwner(obj metav1.Object) (*v1beta1.MachineInventorySelector, error) {
	for _, owner := range obj.GetOwnerReferences() {
		if owner.APIVersion == v1beta1.SchemeGroupVersion.String() && owner.Kind == "MachineInventorySelector" {
			return h.MachineInventorySelectorCache.Get(obj.GetNamespace(), owner.Name)
		}
	}

	return nil, nil
}

func (h *handler) getMachineOwner(obj metav1.Object) (*v1beta12.Machine, error) {
	for _, owner := range obj.GetOwnerReferences() {
		if owner.APIVersion == v1beta12.GroupVersion.String() && owner.Kind == "Machine" {
			return h.MachineCache.Get(obj.GetNamespace(), owner.Name)
		}
	}

	return nil, nil
}
