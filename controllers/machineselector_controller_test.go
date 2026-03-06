/*
Copyright © 2022 - 2026 SUSE LLC

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"
)

var _ = Describe("reconcile machine inventory selector", func() {
	var r *MachineInventorySelectorReconciler
	var miSelector *elementalv1.MachineInventorySelector
	var planSecret *corev1.Secret
	var mInventory *elementalv1.MachineInventory
	var boostrapSecret *corev1.Secret
	var machine *clusterv1.Machine

	BeforeEach(func() {
		r = &MachineInventorySelectorReconciler{
			Client: cl,
		}

		miSelector = &elementalv1.MachineInventorySelector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Machine",
						Name:       "machine-inventory-suite",
						UID:        "test",
					},
				},
			},
			Spec: elementalv1.MachineInventorySelectorSpec{
				Selector: metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "location",
							Operator: "In",
							Values:   []string{"testregion", "bajoran"},
						},
					},
				},
			},
		}

		planSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
			},
			Data: map[string][]byte{"plan": []byte("test")},
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
				Labels: map[string]string{
					"location":                       "testregion",
					"elemental.cattle.io/ExternalIP": "1.1.1.1",
					"elemental.cattle.io/InternalIP": "2.2.2.2",
					"elemental.cattle.io/Hostname":   "host.name",
				},
			},
		}

		boostrapSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite-boostrap",
				Namespace: miSelector.Namespace,
			},
			Data: map[string][]byte{"value": []byte("test")},
		}

		machine = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: ptr.To(boostrapSecret.Name),
				},
			},
		}

		Expect(cl.Create(ctx, miSelector)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, miSelector, mInventory, planSecret, boostrapSecret, machine)).To(Succeed())
	})

	It("should reconcile machine selector when matching inventory exists", func() {
		Expect(r.Create(ctx, planSecret)).To(Succeed())
		Expect(r.Create(ctx, mInventory)).To(Succeed())
		Expect(r.Get(ctx, types.NamespacedName{Name: mInventory.Name, Namespace: mInventory.Namespace}, mInventory)).To(Succeed())
		mInventory.Status = elementalv1.MachineInventoryStatus{
			Plan: &elementalv1.PlanStatus{
				PlanSecretRef: &corev1.ObjectReference{
					Name:      planSecret.Name,
					Namespace: miSelector.Namespace,
				},
			},
		}
		Expect(r.Status().Update(ctx, mInventory)).To(Succeed())
		Expect(r.Create(ctx, boostrapSecret)).To(Succeed())

		Expect(r.Create(ctx, machine)).To(Succeed())

		_, err := r.reconcile(ctx, miSelector)
		Expect(err).ToNot(HaveOccurred())

		Expect(miSelector.Status.Ready).To(BeFalse())
		Expect(miSelector.Status.BootstrapPlanChecksum).To(BeEmpty())

		Expect(miSelector.Status.MachineInventoryRef).ToNot(BeNil())
		Expect(miSelector.Status.MachineInventoryRef.Name).To(Equal(mInventory.Name))

		cond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.ReadyCondition)
		Expect(cond).NotTo(BeNil())

		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(elementalv1.WaitingForInventoryReason))
	})

	It("should reconcile when matching inventory doesn't exist", func() {
		_, err := r.reconcile(ctx, miSelector)
		Expect(err).ToNot(HaveOccurred())

		Expect(miSelector.Status.Ready).To(BeFalse())
		Expect(miSelector.Status.BootstrapPlanChecksum).To(BeEmpty())
		Expect(miSelector.Status.MachineInventoryRef).To(BeNil())

		Expect(miSelector.Status.Conditions).To(HaveLen(1))
		Expect(miSelector.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(miSelector.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		Expect(miSelector.Status.Conditions[0].Reason).To(Equal(elementalv1.WaitingForInventoryReason))
	})
})

var _ = Describe("findAndAdoptInventory", func() {
	var r *MachineInventorySelectorReconciler
	var miSelector *elementalv1.MachineInventorySelector
	var mInventory *elementalv1.MachineInventory

	BeforeEach(func() {
		r = &MachineInventorySelectorReconciler{
			Client: cl,
		}

		miSelector = &elementalv1.MachineInventorySelector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: "default",
				UID:       "test",
			},
			Spec: elementalv1.MachineInventorySelectorSpec{
				Selector: metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "location",
							Operator: "In",
							Values:   []string{"testregion"},
						},
					},
				},
			},
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
			},
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, miSelector, mInventory)).To(Succeed())
	})

	It("successfully adopt matching machine inventory", func() {
		mInventory.Labels = map[string]string{
			"location": "testregion",
		}
		Expect(r.Create(ctx, mInventory)).To(Succeed())

		Expect(r.findAndAdoptInventory(ctx, miSelector, &elementalv1.MachineInventory{})).To(Succeed())

		rCond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.ReadyCondition)
		Expect(rCond).NotTo(BeNil())

		Expect(rCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(rCond.Reason).To(Equal(elementalv1.WaitingForInventoryReason))

		aCond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.InventoryReadyCondition)
		Expect(aCond).NotTo(BeNil())

		Expect(aCond.Status).To(Equal(metav1.ConditionUnknown))
		Expect(aCond.Reason).To(Equal(elementalv1.WaitForInventoryCheckReason))

		Expect(miSelector.Status.MachineInventoryRef).ToNot(BeNil())
		Expect(miSelector.Status.MachineInventoryRef.Name).To(Equal(mInventory.Name))

		Expect(r.Get(ctx, types.NamespacedName{Name: mInventory.Name, Namespace: mInventory.Namespace}, mInventory)).To(Succeed())
		Expect(mInventory.OwnerReferences).To(HaveLen(1))
		Expect(mInventory.OwnerReferences[0].APIVersion).To(Equal(elementalv1.GroupVersion.String()))
		Expect(mInventory.OwnerReferences[0].Kind).To(Equal("MachineInventorySelector"))
		Expect(mInventory.OwnerReferences[0].Name).To(Equal(miSelector.Name))
		Expect(mInventory.OwnerReferences[0].UID).To(Equal(miSelector.UID))
		Expect(mInventory.OwnerReferences[0].Controller).To(Equal(ptr.To(true)))
	})

	It("return early if no matching inventories found", func() {
		mInventory.Labels = map[string]string{
			"location": "badtestregion",
		}

		Expect(r.Create(ctx, mInventory)).To(Succeed())

		Expect(r.findAndAdoptInventory(ctx, miSelector, nil)).To(Succeed())

		Expect(miSelector.Status.Conditions).To(HaveLen(1))
		Expect(miSelector.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(miSelector.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		Expect(miSelector.Status.Conditions[0].Reason).To(Equal(elementalv1.WaitingForInventoryReason))
	})

	It("should do nothing is machine inventory refernce is already set", func() {
		miSelector.Status.MachineInventoryRef = &corev1.LocalObjectReference{
			Name: "test",
		}

		Expect(r.findAndAdoptInventory(ctx, miSelector, &elementalv1.MachineInventory{})).To(Succeed())
	})
})

var _ = Describe("updateAdoptionStatus", func() {
	var r *MachineInventorySelectorReconciler
	var miSelector *elementalv1.MachineInventorySelector
	var mInventory *elementalv1.MachineInventory

	BeforeEach(func() {
		r = &MachineInventorySelectorReconciler{
			Client: cl,
		}

		miSelector = &elementalv1.MachineInventorySelector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: "default",
				UID:       "test",
			},
			Spec: elementalv1.MachineInventorySelectorSpec{
				Selector: metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "location",
							Operator: "In",
							Values:   []string{"testregion"},
						},
					},
				},
			},
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
				Labels: map[string]string{
					"location": "testregion",
				},
			},
		}
		Expect(r.Create(ctx, miSelector)).To(Succeed())
		Expect(r.Create(ctx, mInventory)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, miSelector, mInventory)).To(Succeed())
	})

	It("successfully checks ongoing adoption", func() {
		// Start adoption check process
		Expect(r.findAndAdoptInventory(ctx, miSelector, mInventory)).To(Succeed())
		Expect(r.Get(ctx, types.NamespacedName{
			Namespace: miSelector.Namespace,
			Name:      miSelector.Status.MachineInventoryRef.Name,
		}, mInventory)).To(Succeed())

		rCond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.ReadyCondition)
		Expect(rCond).NotTo(BeNil())
		Expect(rCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(rCond.Reason).To(Equal(elementalv1.WaitingForInventoryReason))

		aCond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.InventoryReadyCondition)
		Expect(aCond).NotTo(BeNil())
		Expect(aCond.Status).To(Equal(metav1.ConditionUnknown))
		Expect(aCond.Reason).To(Equal(elementalv1.WaitForInventoryCheckReason))

		Expect(miSelector.Status.MachineInventoryRef).ToNot(BeNil())
		Expect(miSelector.Status.MachineInventoryRef.Name).To(Equal(mInventory.Name))

		// Set machine inventory adopted status
		meta.SetStatusCondition(&mInventory.Status.Conditions, metav1.Condition{
			Type:   elementalv1.AdoptionReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: elementalv1.SuccessfullyAdoptedReason,
		})

		// Successfully check adoption
		requeue, err := r.updateAdoptionStatus(ctx, miSelector, mInventory)
		Expect(err).To(BeNil())
		Expect(requeue).To(BeFalse())

		aCond = meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.InventoryReadyCondition)
		Expect(aCond).NotTo(BeNil())
		Expect(aCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(aCond.Reason).To(Equal(elementalv1.SuccessfullyAdoptedInventoryReason))
	})

	It("reaches adoption validation timeout", func() {
		// Start adoption check process
		Expect(r.findAndAdoptInventory(ctx, miSelector, mInventory)).To(Succeed())
		Expect(r.Get(ctx, types.NamespacedName{
			Namespace: miSelector.Namespace,
			Name:      miSelector.Status.MachineInventoryRef.Name,
		}, mInventory)).To(Succeed())

		rCond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.ReadyCondition)
		Expect(rCond).NotTo(BeNil())
		Expect(rCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(rCond.Reason).To(Equal(elementalv1.WaitingForInventoryReason))

		aCond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.InventoryReadyCondition)
		Expect(aCond).NotTo(BeNil())
		Expect(aCond.Status).To(Equal(metav1.ConditionUnknown))
		Expect(aCond.Reason).To(Equal(elementalv1.WaitForInventoryCheckReason))

		Expect(miSelector.Status.MachineInventoryRef).ToNot(BeNil())
		Expect(miSelector.Status.MachineInventoryRef.Name).To(Equal(mInventory.Name))

		// Successfully check adoption
		testTimeout := time.Now().Add(adoptionTimeout * time.Second * 2)
		requeue, err := r.updateAdoptionStatus(ctx, miSelector, mInventory)
		for err == nil {
			Expect(time.Now().Before(testTimeout)).To(BeTrue())
			Expect(requeue).To(BeTrue())
			time.Sleep(time.Second)
			requeue, err = r.updateAdoptionStatus(ctx, miSelector, mInventory)
		}
		Expect(err.Error()).To(ContainSubstring("adoption validation timeout"))
	})

	It("detects an adoption owneship mismatch", func() {
		// Start adoption check process
		Expect(r.findAndAdoptInventory(ctx, miSelector, mInventory)).To(Succeed())
		Expect(r.Get(ctx, types.NamespacedName{
			Namespace: miSelector.Namespace,
			Name:      miSelector.Status.MachineInventoryRef.Name,
		}, mInventory)).To(Succeed())

		rCond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.ReadyCondition)
		Expect(rCond).NotTo(BeNil())
		Expect(rCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(rCond.Reason).To(Equal(elementalv1.WaitingForInventoryReason))

		aCond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.InventoryReadyCondition)
		Expect(aCond).NotTo(BeNil())
		Expect(aCond.Status).To(Equal(metav1.ConditionUnknown))
		Expect(aCond.Reason).To(Equal(elementalv1.WaitForInventoryCheckReason))

		Expect(miSelector.Status.MachineInventoryRef).ToNot(BeNil())
		Expect(miSelector.Status.MachineInventoryRef.Name).To(Equal(mInventory.Name))

		// Swap owner to an invalid name
		mInventory.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: elementalv1.GroupVersion.String(),
				Kind:       "MachineInventorySelector",
				Name:       "differentName",
			},
		}

		// Detect adoption mismatch
		_, err := r.updateAdoptionStatus(ctx, miSelector, mInventory)
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("inventory ownership mismatch detected"))
	})

	It("does nothing if adoption has not started", func() {
		requeue, err := r.updateAdoptionStatus(ctx, miSelector, mInventory)
		Expect(requeue).To(BeFalse())
		Expect(err).To(BeNil())
		Expect(meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.InventoryReadyCondition)).To(BeNil())
	})

})

var _ = Describe("updatePlanSecretWithBootstrap", func() {
	var r *MachineInventorySelectorReconciler
	var miSelector *elementalv1.MachineInventorySelector
	var mInventory *elementalv1.MachineInventory
	var boostrapSecret *corev1.Secret
	var planSecret *corev1.Secret
	var machine *clusterv1.Machine

	BeforeEach(func() {
		r = &MachineInventorySelectorReconciler{
			Client: cl,
		}

		miSelector = &elementalv1.MachineInventorySelector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: "default",
				UID:       "test",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Machine",
						Name:       "machine-inventory-suite",
						UID:        "test",
					},
				},
			},
			Spec: elementalv1.MachineInventorySelectorSpec{
				Selector: metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "location",
							Operator: "In",
							Values:   []string{"testregion"},
						},
					},
				},
			},
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
			},
		}

		boostrapSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite-boostrap",
				Namespace: miSelector.Namespace,
			},
			Data: map[string][]byte{"value": []byte("test")},
		}

		machine = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: ptr.To(boostrapSecret.Name),
				},
			},
		}

		planSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
			},
			Data: map[string][]byte{"plan": []byte("test")},
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, miSelector, mInventory, machine, boostrapSecret, planSecret)).To(Succeed())
	})

	It("should do nothing if machine inventory ref is missing", func() {
		Expect(r.updatePlanSecretWithBootstrap(ctx, miSelector, mInventory)).To(Succeed())
	})

	It("should do nothing if bootstrap plan checksum is already set", func() {
		miSelector.Status.BootstrapPlanChecksum = "test"
		Expect(r.updatePlanSecretWithBootstrap(ctx, miSelector, mInventory)).To(Succeed())
	})

	It("do nothing if machine inventory plan not ready yet", func() {
		Expect(r.Create(ctx, mInventory)).To(Succeed())

		miSelector.Status.MachineInventoryRef = &corev1.LocalObjectReference{
			Name: mInventory.Name,
		}

		Expect(r.updatePlanSecretWithBootstrap(ctx, miSelector, mInventory)).To(Succeed())
	})

	It("return error if failed to create new bootstrap plan", func() {
		// emulate inventory adoption first
		Expect(r.Create(ctx, mInventory)).To(Succeed())
		Expect(r.findAndAdoptInventory(ctx, miSelector, mInventory)).To(Succeed())
		mInventory.Status = elementalv1.MachineInventoryStatus{
			Plan: &elementalv1.PlanStatus{
				PlanSecretRef: &corev1.ObjectReference{
					Name:      "test",
					Namespace: "test",
				},
			},
			Conditions: []metav1.Condition{{
				Type:               elementalv1.AdoptionReadyCondition,
				Status:             metav1.ConditionTrue,
				Reason:             elementalv1.SuccessfullyAdoptedReason,
				LastTransitionTime: metav1.Now(),
			}},
		}
		update, err := r.updateAdoptionStatus(ctx, miSelector, mInventory)
		Expect(update).To(BeFalse())
		Expect(err).To(BeNil())
		Expect(r.Status().Update(ctx, mInventory)).To(Succeed())

		// fails to create the bootstrap plan
		err = r.updatePlanSecretWithBootstrap(ctx, miSelector, mInventory)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get bootstrap plan"))
	})

	It("return error if plan secret doesn't exist", func() {
		// emulate inventory adoption first
		Expect(r.Create(ctx, mInventory)).To(Succeed())
		Expect(r.Create(ctx, machine)).To(Succeed())
		Expect(r.Create(ctx, boostrapSecret)).To(Succeed())
		Expect(r.findAndAdoptInventory(ctx, miSelector, mInventory)).To(Succeed())
		mInventory.Status = elementalv1.MachineInventoryStatus{
			Plan: &elementalv1.PlanStatus{
				PlanSecretRef: &corev1.ObjectReference{
					Name: "invalidname",
				},
			},
			Conditions: []metav1.Condition{{
				Type:               elementalv1.AdoptionReadyCondition,
				Status:             metav1.ConditionTrue,
				Reason:             elementalv1.SuccessfullyAdoptedReason,
				LastTransitionTime: metav1.Now(),
			}},
		}
		update, err := r.updateAdoptionStatus(ctx, miSelector, mInventory)
		Expect(update).To(BeFalse())
		Expect(err).To(BeNil())
		Expect(r.Status().Update(ctx, mInventory)).To(Succeed())

		// fails to create the bootstrap plan
		err = r.updatePlanSecretWithBootstrap(ctx, miSelector, mInventory)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get plan secret"))
	})

	It("succefully update plan secret with bootstrap", func() {
		// emulate inventory adoption first
		Expect(r.Create(ctx, mInventory)).To(Succeed())
		Expect(r.Create(ctx, machine)).To(Succeed())
		Expect(r.Create(ctx, boostrapSecret)).To(Succeed())
		Expect(r.Create(ctx, planSecret)).To(Succeed())
		Expect(r.findAndAdoptInventory(ctx, miSelector, mInventory)).To(Succeed())
		mInventory.Status = elementalv1.MachineInventoryStatus{
			Plan: &elementalv1.PlanStatus{
				PlanSecretRef: &corev1.ObjectReference{
					Name:      planSecret.Name,
					Namespace: planSecret.Namespace,
				},
			},
			Conditions: []metav1.Condition{{
				Type:               elementalv1.AdoptionReadyCondition,
				Status:             metav1.ConditionTrue,
				Reason:             elementalv1.SuccessfullyAdoptedReason,
				LastTransitionTime: metav1.Now(),
			}},
		}
		update, err := r.updateAdoptionStatus(ctx, miSelector, mInventory)
		Expect(update).To(BeFalse())
		Expect(err).To(BeNil())
		Expect(r.Status().Update(ctx, mInventory)).To(Succeed())

		Expect(r.updatePlanSecretWithBootstrap(ctx, miSelector, mInventory)).To(Succeed())

		Expect(miSelector.Status.BootstrapPlanChecksum).ToNot(BeEmpty())

		rCond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.ReadyCondition)
		Expect(rCond).NotTo(BeNil())

		Expect(rCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(rCond.Reason).To(Equal(elementalv1.SuccessfullyUpdatedPlanReason))
	})

})

var _ = Describe("newBootstrapPlan", func() {
	var r *MachineInventorySelectorReconciler
	var miSelector *elementalv1.MachineInventorySelector
	var mInventory *elementalv1.MachineInventory
	var boostrapSecret *corev1.Secret
	var machine *clusterv1.Machine

	BeforeEach(func() {
		r = &MachineInventorySelectorReconciler{
			Client: cl,
		}

		miSelector = &elementalv1.MachineInventorySelector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: "default",
				UID:       "test",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Machine",
						Name:       "machine-inventory-suite",
						UID:        "test",
					},
				},
			},
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
			},
		}

		boostrapSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite-boostrap",
				Namespace: miSelector.Namespace,
			},
			Data: map[string][]byte{"value": []byte("test")},
		}

		machine = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
			},
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, miSelector, mInventory, machine, boostrapSecret)).To(Succeed())
	})

	It("should return an error if no owner machine found", func() {
		_, _, err := r.newBootstrapPlan(ctx, miSelector, mInventory)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to find an owner machine for inventory selector"))
	})

	It("should return an error if machine doesn't have a boostrap secret ref", func() {
		Expect(r.Create(ctx, machine)).To(Succeed())

		_, _, err := r.newBootstrapPlan(ctx, miSelector, mInventory)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing bootstrap data secret name"))
	})

	It("should return an error if failed to get bootstrap secret", func() {
		machine.Spec = clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: ptr.To(boostrapSecret.Name),
			},
		}
		Expect(r.Create(ctx, machine)).To(Succeed())

		_, _, err := r.newBootstrapPlan(ctx, miSelector, mInventory)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get a boostrap plan for the machine"))
	})

	It("should succesfully return new boostrap plan", func() {
		machine.Spec = clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: ptr.To(boostrapSecret.Name),
			},
		}
		Expect(r.Create(ctx, machine)).To(Succeed())
		Expect(r.Create(ctx, boostrapSecret)).To(Succeed())

		checksum, plan, err := r.newBootstrapPlan(ctx, miSelector, mInventory)
		Expect(err).ToNot(HaveOccurred())

		Expect(checksum).ToNot(BeEmpty())
		Expect(plan).ToNot(BeEmpty())
	})
})

var _ = Describe("setInvetorySelectorAddresses", func() {
	var r *MachineInventorySelectorReconciler
	var miSelector *elementalv1.MachineInventorySelector
	var mInventory *elementalv1.MachineInventory

	BeforeEach(func() {
		r = &MachineInventorySelectorReconciler{
			Client: cl,
		}

		miSelector = &elementalv1.MachineInventorySelector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: "default",
				UID:       "test",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Machine",
						Name:       "machine-inventory-suite",
						UID:        "test",
					},
				},
			},
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: miSelector.Namespace,
			},
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, miSelector, mInventory)).To(Succeed())
	})

	It("should return early if machine inventory reference is missing", func() {
		Expect(r.setInvetorySelectorAddresses(ctx, miSelector, mInventory)).To(Succeed())
	})

	It("should return error if machine inventory is missing", func() {
		miSelector.Status.MachineInventoryRef = &corev1.LocalObjectReference{
			Name: mInventory.Name,
		}
		miSelector.Status.BootstrapPlanChecksum = "thisIsAChecksum"

		err := r.setInvetorySelectorAddresses(ctx, miSelector, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get machine inventory"))
	})

	It("should succesfully set adresses", func() {
		miSelector.Status.MachineInventoryRef = &corev1.LocalObjectReference{
			Name: mInventory.Name,
		}
		miSelector.Status.BootstrapPlanChecksum = "thisIsAChecksum"

		mInventory.Labels = map[string]string{
			"location":                       "testregion",
			"elemental.cattle.io/ExternalIP": "1.1.1.1",
			"elemental.cattle.io/InternalIP": "2.2.2.2",
			"elemental.cattle.io/Hostname":   "host.name",
		}
		Expect(r.Create(ctx, mInventory)).To(Succeed())

		err := r.setInvetorySelectorAddresses(ctx, miSelector, mInventory)
		Expect(err).ToNot(HaveOccurred())

		Expect(miSelector.Status.Ready).To(BeTrue())

		rCond := meta.FindStatusCondition(miSelector.Status.Conditions, elementalv1.ReadyCondition)
		Expect(rCond).NotTo(BeNil())

		Expect(rCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(rCond.Reason).To(Equal(elementalv1.SelectorReadyReason))

		Expect(miSelector.Status.Addresses).To(HaveLen(3))

		Expect(miSelector.Status.Addresses[0].Type).To(Equal(clusterv1.MachineExternalIP))
		Expect(miSelector.Status.Addresses[0].Address).To(Equal("1.1.1.1"))

		Expect(miSelector.Status.Addresses[1].Type).To(Equal(clusterv1.MachineInternalIP))
		Expect(miSelector.Status.Addresses[1].Address).To(Equal("2.2.2.2"))

		Expect(miSelector.Status.Addresses[2].Type).To(Equal(clusterv1.MachineHostName))
		Expect(miSelector.Status.Addresses[2].Address).To(Equal("host.name"))
	})
})
