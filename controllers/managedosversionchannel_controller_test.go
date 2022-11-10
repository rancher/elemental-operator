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

package controllers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("reconcile managed os version channel", func() {
	var r *ManagedOSVersionChannelReconciler
	var managedOSVersionChannel *elementalv1.ManagedOSVersionChannel

	BeforeEach(func() {
		r = &ManagedOSVersionChannelReconciler{
			Client: cl,
		}

		managedOSVersionChannel = &elementalv1.ManagedOSVersionChannel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, managedOSVersionChannel)).To(Succeed())
	})

	It("should reconcile managed os version channel object", func() {
		managedOSVersionChannel.Spec.Type = "type"
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: managedOSVersionChannel.Namespace,
				Name:      managedOSVersionChannel.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions).To(HaveLen(1))
		Expect(managedOSVersionChannel.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.SuccefullyCreatedReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	})

	It("should reconcile managed os version channel object without a type", func() {
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: managedOSVersionChannel.Namespace,
				Name:      managedOSVersionChannel.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions).To(HaveLen(1))
		Expect(managedOSVersionChannel.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.InvalidConfigurationReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(managedOSVersionChannel.Status.Conditions[0].Message).To(ContainSubstring("spec.Type can't be empty"))
	})
})
