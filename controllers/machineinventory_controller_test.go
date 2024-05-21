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

package controllers

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
				Finalizers: []string{elementalv1.MachineInventoryFinalizer},
				Name:       "machine-inventory-suite",
				Namespace:  "default",
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

		cond := meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.ReadyCondition)
		Expect(cond).NotTo(BeNil())

		Expect(cond.Reason).To(Equal(elementalv1.WaitingForPlanReason))
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Message).To(Equal("waiting for plan to be applied"))
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

		Expect(mInventory.Status.Plan.Checksum).To(Equal(string(planSecret.Data["applied-checksum"])))
		Expect(mInventory.Status.Plan.State).To(Equal(elementalv1.PlanApplied))

		cond := meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.ReadyCondition)
		Expect(cond).NotTo(BeNil())

		Expect(cond.Reason).To(Equal(elementalv1.PlanSuccessfullyAppliedReason))
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Message).To(Equal("plan successfully applied"))
	})

	It("should add finalizer if not exist", func() {
		noFinalizerMI := &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-no-finalizer",
				Namespace: "default",
			},
		}
		Expect(cl.Create(ctx, noFinalizerMI)).To(Succeed())

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
		}, noFinalizerMI)).To(Succeed())

		Expect(controllerutil.ContainsFinalizer(noFinalizerMI, elementalv1.MachineInventoryFinalizer)).To(BeTrue())
		Expect(test.CleanupAndWait(ctx, cl, noFinalizerMI)).To(Succeed())
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
		Expect(planSecret.OwnerReferences[0].Controller).To(Equal(ptr.To(true)))
		Expect(planSecret.Labels).To(HaveKey(elementalv1.ElementalManagedLabel))
		Expect(planSecret.Type).To(Equal(elementalv1.PlanSecretType))
		Expect(planSecret.Data).To(HaveKey("plan"))

		Expect(mInventory.Status.Plan.PlanSecretRef.Name).To(Equal(mInventory.Name))
		Expect(mInventory.Status.Plan.PlanSecretRef.Namespace).To(Equal(mInventory.Namespace))

		cond := meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.ReadyCondition)
		Expect(cond).NotTo(BeNil())

		Expect(cond.Reason).To(Equal(elementalv1.WaitingForPlanReason))
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Message).To(Equal("waiting for plan to be applied"))
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

var _ = Describe("updateInventoryWithAdoptionStatus", func() {
	var r *MachineInventoryReconciler
	var mInventory *elementalv1.MachineInventory
	var miSelector *elementalv1.MachineInventorySelector

	BeforeEach(func() {
		r = &MachineInventoryReconciler{
			Client: cl,
		}

		miSelector = &elementalv1.MachineInventorySelector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-selector",
				Namespace: "default",
				UID:       "test",
			},
		}
		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-inventory-suite",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: elementalv1.GroupVersion.String(),
						Kind:       "MachineInventorySelector",
						Name:       miSelector.ObjectMeta.Name,
						UID:        miSelector.ObjectMeta.UID,
					},
				},
			},
		}

		Expect(cl.Create(ctx, mInventory)).To(Succeed())
		Expect(cl.Create(ctx, miSelector)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mInventory, miSelector)).To(Succeed())
	})

	It("successfully verifies owner reference in status", func() {
		// Set selector reference
		Expect(cl.Get(ctx, types.NamespacedName{
			Namespace: miSelector.Namespace,
			Name:      miSelector.Name,
		}, miSelector)).To(Succeed())
		miSelector.Status.MachineInventoryRef = &corev1.LocalObjectReference{
			Name: mInventory.Name,
		}
		Expect(cl.Status().Update(ctx, miSelector)).To(Succeed())

		requeue, err := r.updateInventoryWithAdoptionStatus(ctx, mInventory)
		Expect(err).To(Succeed())
		Expect(requeue).To(BeFalse())

		cond := meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.AdoptionReadyCondition)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(elementalv1.SuccessfullyAdoptedReason))
	})

	It("reaches adoption validation timeout", func() {

		testTimeout := time.Now().Add(adoptionTimeout * 2 * time.Second)
		requeue, err := r.updateInventoryWithAdoptionStatus(ctx, mInventory)
		for err == nil {
			Expect(err).To(Succeed())
			Expect(requeue).To(BeTrue())
			Expect(time.Now().Before(testTimeout)).To(BeTrue())
			time.Sleep(time.Second)
			requeue, err = r.updateInventoryWithAdoptionStatus(ctx, mInventory)
		}
		Expect(err.Error()).To(ContainSubstring("Adoption timeout"))

		cond := meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.AdoptionReadyCondition)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionUnknown))
		Expect(cond.Reason).To(Equal(elementalv1.ValidatingAdoptionReason))
	})

	It("detects an adoption mismatch", func() {
		// Set selector reference
		Expect(cl.Get(ctx, types.NamespacedName{
			Namespace: miSelector.Namespace,
			Name:      miSelector.Name,
		}, miSelector)).To(Succeed())
		miSelector.Status.MachineInventoryRef = &corev1.LocalObjectReference{
			Name: "unexpectedName",
		}
		Expect(cl.Status().Update(ctx, miSelector)).To(Succeed())

		requeue, err := r.updateInventoryWithAdoptionStatus(ctx, mInventory)
		Expect(err).NotTo(Succeed())
		Expect(err.Error()).To(ContainSubstring("Ownership mismatch"))
		Expect(requeue).To(BeFalse())
	})

	It("has nothing if no owner is set", func() {
		mInventory.ObjectMeta.OwnerReferences = []metav1.OwnerReference{}

		requeue, err := r.updateInventoryWithAdoptionStatus(ctx, mInventory)
		Expect(err).To(Succeed())
		Expect(requeue).To(BeFalse())

		cond := meta.FindStatusCondition(mInventory.Status.Conditions, elementalv1.AdoptionReadyCondition)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(elementalv1.WaitingToBeAdoptedReason))
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

var _ = Describe("handle finalizer", func() {
	var r *MachineInventoryReconciler
	var mInventory *elementalv1.MachineInventory
	var planSecret *corev1.Secret

	BeforeEach(func() {
		r = &MachineInventoryReconciler{
			Client: cl,
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
				Annotations:       map[string]string{elementalv1.MachineInventoryResettableAnnotation: "true"},
				Finalizers:        []string{elementalv1.MachineInventoryFinalizer},
				Name:              "machine-inventory-suite-finalizer",
				Namespace:         "default",
			},
		}

		planSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mInventory.Name,
				Namespace: mInventory.Namespace,
			},
		}

		// 1. Create initial MachineInventory
		Expect(cl.Create(ctx, mInventory)).To(Succeed())
		// 2. Create initial plan Secret
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		// 3. Update meta.DeletionTimestamp
		Expect(cl.Delete(ctx, mInventory)).To(Succeed())
		// 4. Update secret with reset plan
		_, err = r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		// 5. Update MachineInventory plan status
		_, err = r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
	})

	It("should update secret with reset plan", func() {
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      planSecret.Name,
			Namespace: planSecret.Namespace,
		}, planSecret)).To(Succeed())
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, mInventory)).To(Succeed())

		wantChecksum, wantPlan, err := r.newResetPlan(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Check Plan status
		Expect(mInventory.Status.Plan.Checksum).To(Equal(wantChecksum))
		Expect(mInventory.Status.Plan.PlanSecretRef.Name).To(Equal(planSecret.Name))
		Expect(mInventory.Status.Plan.PlanSecretRef.Namespace).To(Equal(planSecret.Namespace))
		Expect(mInventory.Status.Plan.State).To(Equal(elementalv1.PlanState("")))

		// Check MachineInventory status
		Expect(mInventory.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(mInventory.Status.Conditions[0].Reason).To(Equal(elementalv1.WaitingForPlanReason))
		Expect(mInventory.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		Expect(mInventory.Status.Conditions[0].Message).To(Equal("waiting for plan to be applied"))

		// Check plan secret was updated
		Expect(planSecret.Annotations[elementalv1.PlanTypeAnnotation]).To(Equal(elementalv1.PlanTypeReset))
		Expect(planSecret.Data["plan"]).To(Equal(wantPlan))
		Expect(planSecret.Data["applied-checksum"]).To(Equal([]byte("")))
		Expect(planSecret.Data["failed-checksum"]).To(Equal([]byte("")))

		// Check we are holding on the MachineInventory (preventing actual deletion)
		_, err = r.Reconcile(ctx, reconcile.Request{
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
	})

	It("should remove finalizer on reset plan applied", func() {
		// 6. Mark the reset plan as applied
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, planSecret)).To(Succeed())
		planSecret.Data["applied-checksum"] = []byte("applied")
		Expect(cl.Update(ctx, planSecret)).To(Succeed())

		// 7. Trigger finalizer removal
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		// Check MachineInventory was actually deleted
		err = cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, mInventory)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("should delete by removing finalizer when resettable annotation is false", func() {
		// 6. Manually disable resettable annotation
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, mInventory)).To(Succeed())
		mInventory.Annotations[elementalv1.MachineInventoryResettableAnnotation] = "false"
		Expect(cl.Update(ctx, mInventory)).To(Succeed())

		// 7. Trigger finalizer removal
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		// Check MachineInventory was actually deleted
		err = cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, mInventory)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("should delete by removing finalizer when up for deletion", func() {
		// 6. Manually remove finalizer
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, mInventory)).To(Succeed())
		controllerutil.RemoveFinalizer(mInventory, elementalv1.MachineInventoryFinalizer)
		Expect(cl.Update(ctx, mInventory)).To(Succeed())

		// Check MachineInventory was actually deleted
		err := cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, mInventory)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mInventory, planSecret)).To(Succeed())
	})
})

var _ = Describe("handle unmanaged finalizer", func() {
	var r *MachineInventoryReconciler
	var mInventory *elementalv1.MachineInventory
	var planSecret *corev1.Secret

	BeforeEach(func() {
		r = &MachineInventoryReconciler{
			Client: cl,
		}

		mInventory = &elementalv1.MachineInventory{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
				Annotations:       map[string]string{elementalv1.MachineInventoryResettableAnnotation: "true", elementalv1.MachineInventoryOSUnmanagedAnnotation: "true"},
				Finalizers:        []string{elementalv1.MachineInventoryFinalizer},
				Name:              "machine-inventory-suite-finalizer",
				Namespace:         "default",
			},
		}

		planSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mInventory.Name,
				Namespace: mInventory.Namespace,
			},
		}

		// 1. Create initial MachineInventory
		Expect(cl.Create(ctx, mInventory)).To(Succeed())
		// 2. Create initial plan Secret
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		// 3. Update meta.DeletionTimestamp
		Expect(cl.Delete(ctx, mInventory)).To(Succeed())
		// 4. Update secret with reset plan
		_, err = r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		// 5. Update MachineInventory plan status
		_, err = r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
	})

	It("should update secret with reset plan when unmanaged annotation is true", func() {
		// Manually enable os.unmanaged annotation
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      planSecret.Name,
			Namespace: planSecret.Namespace,
		}, planSecret)).To(Succeed())
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, mInventory)).To(Succeed())
		mInventory.Annotations[elementalv1.MachineInventoryOSUnmanagedAnnotation] = "true"
		Expect(cl.Update(ctx, mInventory)).To(Succeed())

		wantChecksum, wantPlan, err := r.newUnmanagedResetPlan(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Check Plan status
		Expect(mInventory.Status.Plan.Checksum).To(Equal(wantChecksum))
		Expect(mInventory.Status.Plan.PlanSecretRef.Name).To(Equal(planSecret.Name))
		Expect(mInventory.Status.Plan.PlanSecretRef.Namespace).To(Equal(planSecret.Namespace))
		Expect(mInventory.Status.Plan.State).To(Equal(elementalv1.PlanState("")))

		// Check MachineInventory status
		Expect(mInventory.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(mInventory.Status.Conditions[0].Reason).To(Equal(elementalv1.WaitingForPlanReason))
		Expect(mInventory.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		Expect(mInventory.Status.Conditions[0].Message).To(Equal("waiting for plan to be applied"))

		// Check plan secret was updated
		Expect(planSecret.Annotations[elementalv1.PlanTypeAnnotation]).To(Equal(elementalv1.PlanTypeReset))
		Expect(planSecret.Data["plan"]).To(Equal(wantPlan))
		Expect(planSecret.Data["applied-checksum"]).To(Equal([]byte("")))
		Expect(planSecret.Data["failed-checksum"]).To(Equal([]byte("")))

		// Check we are holding on the MachineInventory (preventing actual deletion)
		_, err = r.Reconcile(ctx, reconcile.Request{
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
	})

	It("should remove finalizer on unmanaged reset plan applied", func() {
		// 6. Mark the reset plan as applied
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, planSecret)).To(Succeed())
		mInventory.Annotations[elementalv1.MachineInventoryOSUnmanagedAnnotation] = "true"
		planSecret.Data["applied-checksum"] = []byte("applied")
		Expect(cl.Update(ctx, planSecret)).To(Succeed())

		// 7. Trigger finalizer removal
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: mInventory.Namespace,
				Name:      mInventory.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		// Check MachineInventory was actually deleted
		err = cl.Get(ctx, client.ObjectKey{
			Name:      mInventory.Name,
			Namespace: mInventory.Namespace,
		}, mInventory)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
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
