/*
Copyright © 2022 - 2023 SUSE LLC

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
	"k8s.io/apimachinery/pkg/runtime/schema"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
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
			"write_files": {Object: WriteFiles{}},
		})
		Expect(err).To(BeNil())
		Expect(data).To(Equal([]byte("#cloud-config\nwrite_files:\n{}\n")))
	})
})

type WriteFiles struct {
}

func (w WriteFiles) GetObjectKind() schema.ObjectKind {
	return nil
}
func (w WriteFiles) DeepCopyObject() runtime.Object {
	return nil
}

var _ runtime.Object = WriteFiles{}
