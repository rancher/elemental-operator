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

package registration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	helpers "github.com/rancher/elemental-operator/tests/controllerHelpers"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "elemental-operator unit tests")
}

var _ = Describe("Registration", func() {
	var hl handler
	var cl *helpers.TestClients
	var sc *helpers.FakeSettingCache

	BeforeEach(func() {
		cl = &helpers.TestClients{}
		sc = &helpers.FakeSettingCache{}
		hl = handler{
			Recorder:     nil,
			clients:      cl,
			settingCache: sc,
		}
	})
	Describe("OnChange", func() {
		It("Fails if server-url is not set", func() {
			mr := &v1beta1.MachineRegistration{}
			mrStatus := v1beta1.MachineRegistrationStatus{}
			_, err := hl.OnChange(mr, mrStatus)
			Expect(err).To(HaveOccurred())
		})
		It("Fills the proper status", func() {
			serverUrl := "https://test.url.here"
			testName := "testObject"
			testNamespace := "testNamespace"
			sc.AddToIndex("server-url", &v3.Setting{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Value:      serverUrl,
				Default:    "",
				Customized: false,
				Source:     "",
			})
			mr := &v1beta1.MachineRegistration{ObjectMeta: metav1.ObjectMeta{Name: testName, Namespace: testNamespace}}
			mrStatus := v1beta1.MachineRegistrationStatus{}
			newStatus, err := hl.OnChange(mr, mrStatus)
			Expect(err).ToNot(HaveOccurred())
			Expect(newStatus.RegistrationURL).ToNot(BeEmpty())
			Expect(newStatus.RegistrationURL).To(ContainSubstring(serverUrl))
			Expect(newStatus.RegistrationToken).ToNot(BeEmpty())
			Expect(newStatus.ServiceAccountRef.Name).ToNot(BeEmpty())
			Expect(newStatus.ServiceAccountRef.Name).To(Equal(testName))
			Expect(newStatus.ServiceAccountRef.Namespace).ToNot(BeEmpty())
			Expect(newStatus.ServiceAccountRef.Namespace).To(Equal(testNamespace))
		})
		It("Fails if it cant CREATE Role", func() {
			Skip("Requires a more advanced RBAC/ServiceAccount mock")
		})
		It("Fails if it cant CREATE ServiceAccount", func() {
			Skip("Requires a more advanced RBAC/ServiceAccount mock")
		})
		It("Fails if it cant CREATE Rolebinding", func() {
			Skip("Requires a more advanced RBAC/ServiceAccount mock")
		})
	})
	Describe("OnRemove", func() {
		It("Doesnt error out", func() {
			mr := &v1beta1.MachineRegistration{}
			rm, err := hl.OnRemove("", mr)
			Expect(rm).To(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})
		It("Fails if it cant DELETE Role", func() {
			Skip("Requires a more advanced RBAC/ServiceAccount mock")
		})
		It("Fails if it cant DELETE ServiceAccount", func() {
			Skip("Requires a more advanced RBAC/ServiceAccount mock")
		})
		It("Fails if it cant DELETE Rolebinding", func() {
			Skip("Requires a more advanced RBAC/ServiceAccount mock")
		})
	})
})
