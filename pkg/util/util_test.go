/*
Copyright © 2022 - 2024 SUSE LLC

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

package util

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
)

var _ = Describe("removeInvalidConditions", func() {
	It("should remove invalid conditions", func() {
		cond := RemoveInvalidConditions([]metav1.Condition{
			{
				Type:               elementalv1.ReadyCondition,
				Reason:             elementalv1.WaitingForPlanReason,
				Message:            "Test message",
				LastTransitionTime: metav1.Now(),
				Status:             metav1.ConditionUnknown,
			},
			{
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             "Ready",
			},
			{
				Type:               "InvalidCondition",
				LastTransitionTime: metav1.Now(),
				Reason:             "Ready",
			},
			{
				Type:   "InvalidCondition",
				Status: metav1.ConditionFalse,
				Reason: "Ready",
			},
			{
				Type:               "InvalidCondition",
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
			},
		})

		Expect(cond).To(HaveLen(1))
		Expect(cond[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(cond[0].Status).To(Equal(metav1.ConditionUnknown))
		Expect(cond[0].Reason).To(Equal(elementalv1.WaitingForPlanReason))
		Expect(cond[0].LastTransitionTime).NotTo(BeZero())
	})
})

var _ = Describe("MarshalCloudConfig", func() {
	It("should marshal an empty config to an empty byte array", func() {
		data, err := MarshalCloudConfig(nil)
		Expect(err).To(BeNil())
		Expect(data).To(Equal([]byte{}))

		data, err = MarshalCloudConfig(map[string]runtime.RawExtension{})
		Expect(err).To(BeNil())
		Expect(data).To(Equal([]byte{}))
	})

	It("should marshal an example cloud-init file correctly", func() {
		data, err := MarshalCloudConfig(map[string]runtime.RawExtension{
			"write_files": {Raw: []byte(`{}`)},
		})
		Expect(err).To(BeNil())
		Expect(string(data)).To(Equal("#cloud-config\nwrite_files: {}\n"))
	})
	It("should marshal a yip file correctly", func() {
		data, err := MarshalCloudConfig(map[string]runtime.RawExtension{
			"stages": {Raw: []byte(`{"network":[{"name":"test","commands":["foo","bar"]}]}`)},
		})
		Expect(err).To(BeNil())
		Expect(string(data)).To(Equal("stages:\n  network:\n  - commands:\n    - foo\n    - bar\n    name: test\n"))
	})
})

var _ = Describe("IsObjectOwned", func() {
	obj := metav1.ObjectMeta{
		OwnerReferences: []metav1.OwnerReference{
			{
				Name: "owner1",
				UID:  "owner1UID",
			},
			{
				Name: "owner2",
				UID:  "owner2UID",
			},
		},
	}

	It("should return true when the passed owner UID is found", func() {
		Expect(IsObjectOwned(&obj, "owner1UID")).To(BeTrue())
		Expect(IsObjectOwned(&obj, "owner2UID")).To(BeTrue())
	})
	It("should return false when the passed owner UID is not found", func() {
		Expect(IsObjectOwned(&obj, "owner3UID")).To(BeFalse())
		Expect(IsObjectOwned(&metav1.ObjectMeta{}, "owner1UID")).To(BeFalse())
	})
})

var _ = Describe("IsHTTP", func() {
	It("should return true on a valid HTTP or HTTPS URL", func() {
		Expect(IsHTTP("https://example.com")).To(BeTrue())
		Expect(IsHTTP("http://insecure-url.org")).To(BeTrue())
	})
	It("should return false on a invalid URL or image reference", func() {
		Expect(IsHTTP("domain.org/container/reference:some-tag")).To(BeFalse())
		Expect(IsHTTP("!@#invalid$%&")).To(BeFalse())
	})
})

var _ = Describe("GetRancherCacert", func() {
	var setting *managementv3.Setting
	BeforeEach(func() {
		setting = &managementv3.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "internal-cacerts",
				Namespace: "default",
			},
			Value: "internalcertificate",
		}
		Expect(cl.Create(ctx, setting)).To(Succeed())
	})
	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, setting)).To(Succeed())
	})
	It("should return the internal certificate", func() {
		Expect(GetRancherCACert(ctx, cl)).To(Equal("internalcertificate"))
	})
	It("should return an empty string if internal certificate is not found", func() {
		Expect(cl.Delete(ctx, setting)).To(Succeed())
		Expect(GetRancherCACert(ctx, cl)).To(BeEmpty())
	})
	It("should returns the default cacerts if found", func() {
		cacertsSetting := &managementv3.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cacerts",
				Namespace: "default",
			},
			Value: "mycertificate",
		}
		Expect(cl.Create(ctx, cacertsSetting)).To(Succeed())
		Expect(GetRancherCACert(ctx, cl)).To(Equal("mycertificate"))
		Expect(cl.Delete(ctx, cacertsSetting)).To(Succeed())
	})
})
