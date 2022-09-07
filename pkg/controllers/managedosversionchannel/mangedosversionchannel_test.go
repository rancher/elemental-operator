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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	helpers "github.com/rancher/elemental-operator/tests/controllerHelpers"
	corev1 "k8s.io/api/core/v1"
	"testing"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "elemental-operator unit tests")
}

type FakeRequeuer struct {
	c             chan interface{}
	RequeueCalled bool
}

func (f *FakeRequeuer) Requeue()                    { f.RequeueCalled = true }
func (f *FakeRequeuer) Dequeue() <-chan interface{} { return f.c }

var _ = Describe("Os Version Channel", func() {
	var hl handler
	var cl *helpers.TestClients
	var queuer FakeRequeuer

	BeforeEach(func() {
		cl = &helpers.TestClients{}
		queuer = FakeRequeuer{}
		hl = handler{
			requer:  &queuer,
			clients: cl,
		}

	})
	It("Fails if object is nil", func() {
		_, err := hl.onChange("test", nil)
		Expect(err).To(HaveOccurred())
		Expect(queuer.RequeueCalled).To(BeFalse())
	})
	It("Fails if object.spec.type is empty", func() {
		obj := &elm.ManagedOSVersionChannel{}
		_, _ = hl.onChange("test", obj)
		Expect(queuer.RequeueCalled).To(BeFalse())
		eventExpected := helpers.Event{
			Message:   "No ManagedOSVersionChannel type defined",
			EventType: corev1.EventTypeWarning,
			Reason:    "error"}
		Expect(cl.Recorder.Events).To(ContainElement(eventExpected))
	})
	It("Requeues correctly", func() {
		obj := &elm.ManagedOSVersionChannel{Spec: elm.ManagedOSVersionChannelSpec{Type: controllerAgentName}}
		_, _ = hl.onChange("test", obj)
		Expect(queuer.RequeueCalled).To(BeTrue())
	})
})
