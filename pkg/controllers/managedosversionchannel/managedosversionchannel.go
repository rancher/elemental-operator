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
	rosscheme "github.com/rancher-sandbox/rancheros-operator/pkg/generated/clientset/versioned/scheme"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	corev1Typed "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
)

const controllerAgentName = "mos-sync"

func Register(ctx context.Context, r chan interface{}, c *clients.Clients) {
	h := &handler{
		requer:        r,
		clients:       c,
		eventRecorder: buildEventRecorder(c.Events),
	}
	c.OS.ManagedOSVersionChannel().OnChange(ctx, controllerAgentName, h.OnChange)
}

type handler struct {
	requer        chan interface{}
	clients       *clients.Clients
	eventRecorder record.EventRecorder
}

func buildEventRecorder(events corev1Typed.EventInterface) record.EventRecorder {
	// Create event broadcaster
	utilruntime.Must(rosscheme.AddToScheme(scheme.Scheme))
	logrus.Info("Creating event broadcaster for " + controllerAgentName)
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1Typed.EventSinkImpl{Interface: events})
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})
}

func (h *handler) OnChange(s string, moc *provv1.ManagedOSVersionChannel) (*provv1.ManagedOSVersionChannel, error) {

	if moc == nil {
		return nil, nil
	}

	if moc.Spec.Type == "" {
		copy := moc.DeepCopy()
		copy.Status.Status = "error"
		h.eventRecorder.Event(moc, corev1.EventTypeWarning, "error", "No ManagedOSVersionChannel type defined")
		_, err := h.clients.OS.ManagedOSVersionChannel().UpdateStatus(copy)
		return nil, err
	}

	logrus.Info("Requeueing sync from controller")

	h.requer <- nil

	return nil, nil
}
