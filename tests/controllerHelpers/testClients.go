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

package controllerHelpers

import (
	"context"
	"fmt"

	capi "github.com/rancher/elemental-operator/pkg/generated/controllers/cluster.x-k8s.io/v1beta1"
	elmcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/elemental.cattle.io/v1beta1"
	fleetcontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/fleet.cattle.io/v1alpha1"
	ranchercontrollers "github.com/rancher/elemental-operator/pkg/generated/controllers/management.cattle.io/v3"
	"github.com/rancher/wrangler/pkg/apply"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	rbaccontrollers "github.com/rancher/wrangler/pkg/generated/controllers/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
)

// TestClients is a test client that fills the clients.ClientInterface
type TestClients struct {
	Recorder *FakeRecorder
}

func (c *TestClients) Start(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (c *TestClients) K8s() kubernetes.Interface {
	//TODO implement me
	panic("implement me")
}

func (c *TestClients) CAPI() capi.Interface {
	//TODO implement me
	panic("implement me")
}

func (c *TestClients) Apply() apply.Apply {
	//TODO implement me
	panic("implement me")
}

func (c *TestClients) Fleet() fleetcontrollers.Interface {
	//TODO implement me
	panic("implement me")
}

func (c *TestClients) Rancher() ranchercontrollers.Interface {
	return FakeRancher{}
}

func (c *TestClients) Elemental() elmcontrollers.Interface {
	return &FakeElementalV1beta1{}
}

func (c *TestClients) EventRecorder(name string) record.EventRecorder {
	if c.Recorder == nil {
		c.Recorder = &FakeRecorder{}
	}
	return c.Recorder
}

func (c *TestClients) RBAC() rbaccontrollers.Interface {
	return &FakeRBAC{}
}

func (c *TestClients) Core() corecontrollers.Interface {
	return &FakeCore{}
}

type FakeRecorder struct {
	Events []Event
}

type Event struct {
	Message   string
	EventType string
	Reason    string
}

// Event will record the event with the object name in the FakeRecorder.Events
func (f *FakeRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	f.Eventf(object, eventtype, reason, message)
}
func (f *FakeRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	f.Events = append(f.Events, Event{
		Message:   fmt.Sprintf(messageFmt, args...),
		EventType: eventtype,
		Reason:    reason,
	})
}
func (f *FakeRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
}
