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

	provv1 "github.com/rancher-sandbox/rancheros-operator/pkg/apis/rancheros.cattle.io/v1"
	"github.com/rancher-sandbox/rancheros-operator/pkg/clients"
	"github.com/sirupsen/logrus"
)

func Register(ctx context.Context, r chan interface{}, clients *clients.Clients) {
	h := &handler{
		requer:  r,
		clients: clients,
	}
	clients.OS.ManagedOSVersionChannel().OnChange(ctx, "mos-sync", h.OnChange)
}

type handler struct {
	requer  chan interface{}
	clients *clients.Clients
}

func (h *handler) OnChange(s string, moc *provv1.ManagedOSVersionChannel) (*provv1.ManagedOSVersionChannel, error) {

	if moc == nil {
		return nil, nil
	}

	if moc.Spec.Type == "" {
		copy := moc.DeepCopy()
		copy.Status.Status = "error"
		_, err := h.clients.OS.ManagedOSVersionChannel().UpdateStatus(copy)
		return nil, err
	}

	logrus.Info("Requeueing sync from controller")

	h.requer <- nil

	return nil, nil
}
