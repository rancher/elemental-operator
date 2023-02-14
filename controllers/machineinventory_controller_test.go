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

package controllers

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"
)

var _ = Describe("reconcile machine inventory", func() {
	var r *MachineInventoryReconciler
	var mInventory *elementalv1.MachineInventory
	var planSecret *corev1.Secret

	BeforeEach(func() {
		r = &MachineInventoryReconciler{
			Client: cl,
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: "default",
			},
		}

		planSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
			Data: map[string][]byte{
				"applied-checksum": []byte("appliedchecksum"),
			},
		}

		Expect(cl.Create(ctx, mInventory)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mInventory, planSecret)).To(Succeed())
	})

	It("should reconcile machine inventory object when plan secret doesn't exist", func() {
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, mInventory)).To(Succeed())

		Expect(mInventory.Finalizers).To(HaveLen(1))
		Expect(mInventory.Finalizers[0]).To(Equal(elementalv1.MachineInventoryFinalizer))

		Expect(mInventory.Status.Conditions).To(HaveLen(1))

		Expect(mInventory.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(mInventory.Status.Conditions[0].Reason).To(Equal(elementalv1.WaitingForPlanReason))
		Expect(mInventory.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		Expect(mInventory.Status.Conditions[0].Message).To(Equal("waiting for plan to be applied"))
	})

	It("should reconcile machine inventory object when plan secret already exists", func() {
		Expect(cl.Create(ctx, planSecret)).To(Succeed())

		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, mInventory)).To(Succeed())

		Expect(mInventory.Finalizers).To(HaveLen(1))
		Expect(mInventory.Finalizers[0]).To(Equal(elementalv1.MachineInventoryFinalizer))

		Expect(mInventory.Status.Plan.Checksum).To(Equal(string(planSecret.Data["applied-checksum"])))
		Expect(mInventory.Status.Plan.State).To(Equal(elementalv1.PlanApplied))

		Expect(mInventory.Status.Conditions).To(HaveLen(1))

		Expect(mInventory.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(mInventory.Status.Conditions[0].Reason).To(Equal(elementalv1.PlanSuccessfullyAppliedReason))
		Expect(mInventory.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(mInventory.Status.Conditions[0].Message).To(Equal("plan successfully applied"))
	})
})

var _ = Describe("createPlanSecret", func() {
	var r *MachineInventoryReconciler
	var mInventory *elementalv1.MachineInventory
	var planSecret *corev1.Secret

	BeforeEach(func() {
		r = &MachineInventoryReconciler{
			Client: cl,
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: "default",
			},
		}

		planSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		}

		Expect(cl.Create(ctx, mInventory)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mInventory, planSecret)).To(Succeed())
	})

	It("should succesfully create plan secret", func() {
		Expect(r.createPlanSecret(ctx, mInventory)).To(Succeed())

		Expect(r.Get(ctx, types.NamespacedName{Namespace: mInventory.Namespace, Name: mInventory.Name}, planSecret)).To(Succeed())

		Expect(planSecret.OwnerReferences).To(HaveLen(1))
		Expect(planSecret.OwnerReferences[0].APIVersion).To(Equal(elementalv1.GroupVersion.String()))
		Expect(planSecret.OwnerReferences[0].Kind).To(Equal("MachineInventory"))
		Expect(planSecret.OwnerReferences[0].Name).To(Equal(mInventory.Name))
		Expect(planSecret.OwnerReferences[0].UID).To(Equal(mInventory.UID))
		Expect(planSecret.OwnerReferences[0].Controller).To(Equal(pointer.Bool(true)))
		Expect(planSecret.Labels).To(HaveKey(elementalv1.ElementalManagedLabel))
		Expect(planSecret.Type).To(Equal(elementalv1.PlanSecretType))
		Expect(planSecret.Data).To(HaveKey("plan"))

		Expect(mInventory.Status.Plan.PlanSecretRef.Name).To(Equal(mInventory.Name))
		Expect(mInventory.Status.Plan.PlanSecretRef.Namespace).To(Equal(mInventory.Namespace))

		Expect(mInventory.Status.Conditions).To(HaveLen(1))

		Expect(mInventory.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(mInventory.Status.Conditions[0].Reason).To(Equal(elementalv1.WaitingForPlanReason))
		Expect(mInventory.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		Expect(mInventory.Status.Conditions[0].Message).To(Equal("waiting for plan to be applied"))
	})

	It("shouldn't return error is secret already exists", func() {
		Expect(r.createPlanSecret(ctx, mInventory)).To(Succeed())
		Expect(r.createPlanSecret(ctx, mInventory)).To(Succeed())
	})

	It("should return error is secret fails to be created", func() {
		r.Client = machineInventoryFailingClient{}
		err := r.createPlanSecret(ctx, mInventory)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create"))
	})

	It("should do nothing if ready condition is present", func() {
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:    elementalv1.ReadyCondition,
			Reason:  elementalv1.PlanSuccessfullyAppliedReason,
			Status:  metav1.ConditionTrue,
			Message: "plan successfully applied",
		})
		Expect(r.createPlanSecret(ctx, mInventory)).To(Succeed())
	})
})

var _ = Describe("updateInventoryWithPlanStatus", func() {
	var r *MachineInventoryReconciler
	var mInventory *elementalv1.MachineInventory
	var planSecret *corev1.Secret

	BeforeEach(func() {
		r = &MachineInventoryReconciler{
			Client: cl,
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: "default",
			},
		}

		planSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		}

		Expect(cl.Create(ctx, mInventory)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mInventory, planSecret)).To(Succeed())
	})

	It("should succesfully update when plan is applied", func() {
		planSecret.Data = map[string][]byte{
			"applied-checksum": []byte("applied-checksum"),
		}
		Expect(cl.Create(ctx, planSecret)).To(Succeed())

		mInventory.Status.Plan = &elementalv1.PlanStatus{}
		Expect(r.updateInventoryWithPlanStatus(ctx, mInventory)).To(Succeed())

		Expect(mInventory.Status.Plan.State).To(Equal(elementalv1.PlanApplied))

		Expect(mInventory.Status.Conditions).To(HaveLen(1))
		Expect(mInventory.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(mInventory.Status.Conditions[0].Reason).To(Equal(elementalv1.PlanSuccessfullyAppliedReason))
		Expect(mInventory.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(mInventory.Status.Conditions[0].Message).To(Equal("plan successfully applied"))
	})

	It("should return error when plan failed to be applied", func() {
		planSecret.Data = map[string][]byte{
			"failed-checksum": []byte("failed-checksum"),
		}
		Expect(cl.Create(ctx, planSecret)).To(Succeed())

		mInventory.Status.Plan = &elementalv1.PlanStatus{}
		err := r.updateInventoryWithPlanStatus(ctx, mInventory)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to apply plan"))
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mInventory, planSecret)).To(Succeed())
	})

})

type machineInventoryFailingClient struct {
	client.Client
}

func (cl machineInventoryFailingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return errors.New("failed to create")
}
