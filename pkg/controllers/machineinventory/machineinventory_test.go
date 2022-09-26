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

package machineinventory

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	helpers "github.com/rancher/elemental-operator/tests/controllerHelpers"
	"github.com/rancher/wrangler/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "elemental-operator unit tests")
}

var _ = Describe("Machine Inventory", func() {
	var mInventoryCache helpers.FakeMachineInventoryIndexer
	var inventory *v1beta1.MachineInventory
	var mInventory *helpers.FakeMachineInventory
	var hl handler
	var status v1beta1.MachineInventoryStatus

	BeforeEach(func() {
		status = v1beta1.MachineInventoryStatus{Plan: &v1beta1.PlanStatus{}}
		inventory = &v1beta1.MachineInventory{
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{Name: "The World", Namespace: "jojo"},
			Spec:       v1beta1.MachineInventorySpec{},
			Status:     status,
		}
		mInventoryCache = helpers.FakeMachineInventoryIndexer{}
		mInventoryCache.AddToIndex(inventory)
		mInventory = helpers.NewFakeInventory(inventory)
		hl = handler{
			machineInventories:    mInventory,
			MachineInventoryCache: &mInventoryCache,
		}
		// Sanity checks before starting, checksums should be empty
		Expect(inventory.Status.Plan.FailedChecksum).To(Equal(""))
		Expect(inventory.Status.Plan.AppliedChecksum).To(Equal(""))
		Expect(inventory.Status.Plan.Checksum).To(Equal(""))
	})
	Describe("onInventoryPlanChange", func() {
		It("Fills the checksums", func() {
			sec := &corev1.Secret{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Type:       "elemental.cattle.io/plan",
				Data: map[string][]byte{
					"plan":             []byte("Star Platinum"),
					"applied-checksum": []byte("Magician's Red"),
					"failed-checksum":  []byte("Hermit Purple"),
				},
			}

			_, err := hl.onInventoryPlanChange("", sec)
			Expect(err).ToNot(HaveOccurred())
			updatedInventory := mInventory.GetInventory()
			Expect(updatedInventory.Status.Plan.FailedChecksum).To(Equal("Hermit Purple"))
			Expect(updatedInventory.Status.Plan.AppliedChecksum).To(Equal("Magician's Red"))
			Expect(updatedInventory.Status.Plan.Checksum).To(Equal(PlanChecksum([]byte("Star Platinum"))))
			// Check that we are dealing with the same inventory we started with
			Expect(updatedInventory.Name).To(Equal("The World"))
			// Expect that the update was called on the inventory
			Expect(mInventory.UpdateStatusCalled).To(BeTrue())
		})
		It("Does nothing if Secret.type does not match", func() {
			sec := &corev1.Secret{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Type:       "something.or.other",
				Data: map[string][]byte{
					"plan":             []byte("Hierophant Green"),
					"applied-checksum": []byte("Silver Chariot"),
					"failed-checksum":  []byte("The Fool"),
				},
			}

			_, err := hl.onInventoryPlanChange("", sec)
			Expect(err).ToNot(HaveOccurred())
			updatedInventory := mInventory.GetInventory()
			// Checksums should be empty as no secret with proper type was found
			Expect(updatedInventory.Status.Plan.FailedChecksum).To(Equal(""))
			Expect(updatedInventory.Status.Plan.AppliedChecksum).To(Equal(""))
			Expect(updatedInventory.Status.Plan.Checksum).To(Equal(""))
			Expect(updatedInventory.Name).To(Equal("The World"))
			Expect(mInventory.UpdateStatusCalled).To(BeFalse())
		})
		It("Does nothing if plan is empty", func() {
			_, err := hl.onInventoryPlanChange("", nil)
			Expect(err).ToNot(HaveOccurred())
			updatedInventory := mInventory.GetInventory()
			// Checksums should be empty as no secret with proper type was found
			Expect(updatedInventory.Status.Plan.FailedChecksum).To(Equal(""))
			Expect(updatedInventory.Status.Plan.AppliedChecksum).To(Equal(""))
			Expect(updatedInventory.Status.Plan.Checksum).To(Equal(""))
			Expect(updatedInventory.Name).To(Equal("The World"))
			Expect(mInventory.UpdateStatusCalled).To(BeFalse())
		})
		It("Errors out if updateStatus fails", func() {
			mInventory.FailOnUpdateStatus = true

			sec := &corev1.Secret{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Type:       "elemental.cattle.io/plan",
				Data: map[string][]byte{
					"plan":             []byte("Star Platinum"),
					"applied-checksum": []byte("Magician's Red"),
					"failed-checksum":  []byte("Hermit Purple"),
				},
			}
			_, err := hl.onInventoryPlanChange("", sec)
			Expect(err).To(HaveOccurred())

		})
		It("Fails if cache returns less than one inventory", func() {
			mInventoryCache.CleanIndex()

			sec := &corev1.Secret{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Type:       "elemental.cattle.io/plan",
				Data: map[string][]byte{
					"plan":             []byte("Star Platinum"),
					"applied-checksum": []byte("Magician's Red"),
					"failed-checksum":  []byte("Hermit Purple"),
				},
			}
			_, err := hl.onInventoryPlanChange("", sec)
			Expect(err).To(HaveOccurred())
			// TODO: error message should be in a language file or something so we can use it here even if it changes.
			Expect(err.Error()).To(ContainSubstring("failed to find machine inventory for plan"))

		})
		It("Fails if cache returns more than one inventory", func() {
			mInventoryCache.AddToIndex(inventory)

			sec := &corev1.Secret{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Type:       "elemental.cattle.io/plan",
				Data: map[string][]byte{
					"plan":             []byte("Star Platinum"),
					"applied-checksum": []byte("Magician's Red"),
					"failed-checksum":  []byte("Hermit Purple"),
				},
			}
			_, err := hl.onInventoryPlanChange("", sec)
			Expect(err).To(HaveOccurred())
			// TODO: error message should be in a language file or something so we can use it here even if it changes.
			Expect(err.Error()).To(ContainSubstring("failed to find machine inventory for plan"))

		})
	})
	Describe("planReadyHandler", func() {
		It("skips if inventory is nil", func() {
			st, err := hl.planReadyHandler(nil, status)
			Expect(st).To(Equal(status))
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(generic.ErrSkip))
		})
		It("skips if status is nil", func() {
			status.Plan = nil
			st, err := hl.planReadyHandler(inventory, status)
			Expect(st).To(Equal(status))
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(generic.ErrSkip))
		})
		It("Sets plan ready if checksums match with AppliedChecksum", func() {
			status.Plan.Checksum = "check"
			status.Plan.AppliedChecksum = "check"
			status.Plan.FailedChecksum = "brrrrr"
			st, err := hl.planReadyHandler(inventory, status)
			Expect(err).ToNot(HaveOccurred())
			// as the applied checksum coincides with the checksum, it should set the plan ready to true
			Expect(v1beta1.PlanReadyCondition.GetStatus(st)).To(Equal("True"))
			Expect(v1beta1.PlanReadyCondition.GetReason(st)).To(Equal(v1beta1.PlanSuccefullyAppliedReason))
			// If plan is ready and matches the applied the controller cleans up any failed checksum
			Expect(st.Plan.FailedChecksum).To(Equal(""))
			Expect(st.Plan.Checksum).To(Equal("check"))
			Expect(st.Plan.AppliedChecksum).To(Equal("check"))
		})
		It("Sets plan ready if checksum match with FailedChecksum", func() {
			status.Plan.Checksum = "check"
			status.Plan.FailedChecksum = "check"
			st, err := hl.planReadyHandler(inventory, status)
			Expect(err).ToNot(HaveOccurred())
			// it sets the PlanReady to True...I guess this is ok now, but we should really set it to failed or something?
			// According to the controller: "a plan is ready if it fails or is applied"
			Expect(v1beta1.PlanReadyCondition.GetStatus(st)).To(Equal("True"))
			Expect(v1beta1.PlanReadyCondition.GetReason(st)).To(Equal(v1beta1.PlanFailedToBeAppliedReason))
			Expect(st.Plan.FailedChecksum).ToNot(Equal(""))
			Expect(st.Plan.Checksum).To(Equal("check"))
			Expect(st.Plan.FailedChecksum).To(Equal("check"))
			Expect(st.Plan.AppliedChecksum).To(Equal(""))
		})
		It("Sets plan to not ready if checksums dont match anything", func() {
			status.Plan.Checksum = "check"
			st, err := hl.planReadyHandler(inventory, status)
			Expect(err).ToNot(HaveOccurred())
			Expect(v1beta1.PlanReadyCondition.GetStatus(st)).To(Equal("False"))
			Expect(v1beta1.PlanReadyCondition.GetReason(st)).To(Equal(v1beta1.WaitingForPlanToBeAppliedReason))
			Expect(st.Plan.Checksum).To(Equal("check"))
			Expect(st.Plan.FailedChecksum).To(Equal(""))
			Expect(st.Plan.AppliedChecksum).To(Equal(""))
		})

	})
	Describe("readyHandler", func() {
		It("inventory is not initialized, sets Ready to false", func() {
			st, err := hl.readyHandler(inventory, status)
			Expect(err).ToNot(HaveOccurred())

			Expect(v1beta1.ReadyCondition.GetStatus(st)).To(Equal("False"))
			Expect(v1beta1.ReadyCondition.GetReason(st)).To(Equal(v1beta1.WaitingForInitializationReason))
			Expect(v1beta1.ReadyCondition.GetMessage(st)).To(Equal("waiting for initialization"))

		})
		It("plan is not ready, sets Ready to false", func() {
			v1beta1.InitializedCondition.True(inventory)

			st, err := hl.readyHandler(inventory, status)
			Expect(err).ToNot(HaveOccurred())
			Expect(v1beta1.ReadyCondition.GetStatus(st)).To(Equal("False"))
			Expect(v1beta1.ReadyCondition.GetReason(st)).To(Equal(v1beta1.WaitingForPlanToBeAppliedReason))
			Expect(v1beta1.ReadyCondition.GetMessage(st)).To(Equal("waiting for plan to be applied"))
		})
		It("Sets Ready to true when plan and inventory are ready", func() {
			v1beta1.InitializedCondition.True(inventory)
			v1beta1.PlanReadyCondition.True(inventory)

			st, err := hl.readyHandler(inventory, status)
			Expect(err).ToNot(HaveOccurred())
			Expect(v1beta1.ReadyCondition.GetStatus(st)).To(Equal("True"))
			Expect(v1beta1.ReadyCondition.GetReason(st)).To(Equal(v1beta1.PlanSuccefullyAppliedReason))
		})
	})
	Describe("initializeHandler", func() {
		It("does not initializes the inventory if its already initialized", func() {
			v1beta1.InitializedCondition.True(inventory)
			_, _, err := hl.initializeHandler(inventory, status)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(generic.ErrSkip))
		})
		It("does nothing if inventory is nil", func() {
			_, _, err := hl.initializeHandler(nil, status)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(generic.ErrSkip))
		})
		It("Creates plan on initialize", func() {
			Expect(status.Plan.SecretRef).To(BeNil())
			_, s, err := hl.initializeHandler(inventory, status)
			Expect(err).ToNot(HaveOccurred())
			// Should be initialized
			Expect(v1beta1.InitializedCondition.GetStatus(s)).To(Equal("True"))
			Expect(v1beta1.InitializedCondition.GetReason(s)).To(Equal(v1beta1.InitializedPlanReason))
			// Should have created a plan in a secret
			Expect(s.Plan.SecretRef).ToNot(BeNil())
			// should inherit the name/namespace of the inventory
			Expect(s.Plan.SecretRef.Namespace).To(Equal("jojo"))
			Expect(s.Plan.SecretRef.Name).To(Equal("The World"))
		})
	})
})
