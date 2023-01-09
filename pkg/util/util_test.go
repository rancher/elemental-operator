/*
Copyright © SUSE LLC

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
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
