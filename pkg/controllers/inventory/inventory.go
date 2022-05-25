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

package inventory

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	v1 "github.com/rancher-sandbox/elemental-operator/pkg/apis/elemental.cattle.io/v1"
	"github.com/rancher-sandbox/elemental-operator/pkg/clients"
	ranchercontrollers "github.com/rancher-sandbox/elemental-operator/pkg/generated/controllers/management.cattle.io/v3"
	provcontrollers "github.com/rancher-sandbox/elemental-operator/pkg/generated/controllers/provisioning.cattle.io/v1"
	v12 "github.com/rancher-sandbox/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/name"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

var controllerName = "machine-inventory"

type handler struct {
	clusterCache                   provcontrollers.ClusterCache
	clusterRegistrationTokenCache  ranchercontrollers.ClusterRegistrationTokenCache
	clusterRegistrationTokenClient ranchercontrollers.ClusterRegistrationTokenClient
	Recorder                       record.EventRecorder
}

func Register(ctx context.Context, clients *clients.Clients) {
	h := &handler{
		clusterCache:                   clients.Provisioning.Cluster().Cache(),
		clusterRegistrationTokenCache:  clients.Rancher.ClusterRegistrationToken().Cache(),
		clusterRegistrationTokenClient: clients.Rancher.ClusterRegistrationToken(),
		Recorder:                       clients.EventRecorder(controllerName),
	}

	clients.OS.MachineInventory().OnRemove(ctx, "machine-inventory-remove", h.OnMachineInventoryRemove)
	v12.RegisterMachineInventoryStatusHandler(ctx, clients.OS.MachineInventory(), "", controllerName, h.OnMachineInventory)
}

func (h *handler) OnMachineInventoryRemove(key string, machine *v1.MachineInventory) (*v1.MachineInventory, error) {
	if machine.Status.ClusterRegistrationTokenName != "" && machine.Status.ClusterRegistrationTokenNamespace != "" {
		err := h.clusterRegistrationTokenClient.Delete(machine.Status.ClusterRegistrationTokenNamespace, machine.Status.ClusterRegistrationTokenName, nil)
		if !apierrors.IsNotFound(err) && err != nil {
			h.Recorder.Event(machine, corev1.EventTypeWarning, "error", err.Error())
			return nil, err
		}
	}
	return machine, nil
}

func (h *handler) OnMachineInventory(machine *v1.MachineInventory, status v1.MachineInventoryStatus) (v1.MachineInventoryStatus, error) {
	if machine == nil || machine.Spec.ClusterName == "" {
		return status, nil
	}

	cluster, err := h.clusterCache.Get(machine.Namespace, machine.Spec.ClusterName)
	if err != nil {
		return status, err
	}

	if cluster.Status.ClusterName == "" {
		msg := fmt.Sprintf("waiting for mgmt cluster to be created for prov cluster %s/%s", machine.Namespace, machine.Spec.ClusterName)
		h.Recorder.Event(machine, corev1.EventTypeWarning, "warning", msg)
		return status, errors.New(msg)
	}

	crtName := name.SafeConcatName(cluster.Status.ClusterName, machine.Name, "token")
	_, err = h.clusterRegistrationTokenCache.Get(cluster.Status.ClusterName, crtName)
	if apierrors.IsNotFound(err) {
		_, err = h.clusterRegistrationTokenClient.Create(&v3.ClusterRegistrationToken{
			ObjectMeta: metav1.ObjectMeta{
				Name:      crtName,
				Namespace: cluster.Status.ClusterName,
			},
			Spec: v3.ClusterRegistrationTokenSpec{
				ClusterName: cluster.Status.ClusterName,
			},
		})
	}

	if err != nil {
		h.Recorder.Event(machine, corev1.EventTypeWarning, "error", err.Error())
		return status, err
	}

	status.ClusterRegistrationTokenName = crtName
	status.ClusterRegistrationTokenNamespace = cluster.Status.ClusterName
	return status, nil
}
