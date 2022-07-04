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

package managedosversionchannel

import (
	"context"

	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	"github.com/rancher/elemental-operator/pkg/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const controllerAgentName = "mos-sync"

// Register registers the ManagedOSVersionChannel controller to the clients with the given options
func Register(ctx context.Context, r types.Requeuer, c *clients.Clients) {
	h := &handler{
		requer:  r,
		clients: c,
	}
	c.Elemental.ManagedOSVersionChannel().OnChange(ctx, controllerAgentName, h.onChange)
}

type handler struct {
	requer  types.Requeuer
	clients *clients.Clients
}

func (h *handler) onChange(s string, moc *elm.ManagedOSVersionChannel) (*elm.ManagedOSVersionChannel, error) {

	if moc == nil {
		return nil, nil
	}

	recorder := h.clients.EventRecorder(controllerAgentName)

	if moc.Spec.Type == "" {
		copy := moc.DeepCopy()
		copy.Status.Status = "error"
		recorder.Event(moc, corev1.EventTypeWarning, "error", "No ManagedOSVersionChannel type defined")
		_, err := h.clients.Elemental.ManagedOSVersionChannel().UpdateStatus(copy)
		return nil, err
	}

	logrus.Info("Requeueing sync from controller")

	h.requer.Requeue()

	return nil, nil
}
