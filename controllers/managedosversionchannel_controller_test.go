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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"
	ctrlHelpers "github.com/rancher/elemental-operator/tests/controllerHelpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const syncJSON = `[
  {
    "metadata": {
      "name": "v0.1.0"
    },
    "spec": {
      "version": "v0.1.0",
      "type": "container",
      "metadata": {
        "upgradeImage": "foo/bar:v0.1.0"
      }
    }
  },
  {
    "metadata": {
      "name": "v0.2.0"
    },
    "spec": {
      "version": "v0.2.0",
      "type": "container",
      "metadata": {
        "upgradeImage": "foo/bar:v0.2.0"
      }
    }
  }
]`

const invalidJSON = `[
  {
    "metadata": {
      "name": "v0.1.0"
    },
    "spec": {
      "version": "v0.1.0",
      "type": "container",
      "metadata": {
        "upgradeImage": "foo/bar:v0.1.0"
    }
  }
]`

var _ = Describe("reconcile managed os version channel", func() {
	var r *ManagedOSVersionChannelReconciler
	var managedOSVersionChannel *elementalv1.ManagedOSVersionChannel
	var syncerProvider *ctrlHelpers.FakeSyncerProvider

	BeforeEach(func() {
		syncerProvider = &ctrlHelpers.FakeSyncerProvider{JSON: syncJSON}
		r = &ManagedOSVersionChannelReconciler{
			Client:         cl,
			syncerProvider: syncerProvider,
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

	It("should reconcile and sync managed os version channel object", func() {
		managedOSVersion := &elementalv1.ManagedOSVersion{}
		managedOSVersionChannel.Spec.Type = "custom"
		managedOSVersionChannel.Spec.SyncInterval = "1m"
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		res1, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: managedOSVersionChannel.Namespace,
				Name:      managedOSVersionChannel.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res1.RequeueAfter).Should(BeNumerically(">", 50*time.Second))

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions).To(HaveLen(1))
		Expect(managedOSVersionChannel.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.SyncedReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(managedOSVersionChannel.Status.Failures).To(Equal(uint32(0)))

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      "v0.1.0",
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersion)).To(Succeed())

		res2, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: managedOSVersionChannel.Namespace,
				Name:      managedOSVersionChannel.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res2.RequeueAfter).Should(BeNumerically("<", res1.RequeueAfter))
		Expect(res2.RequeueAfter).Should(BeNumerically(">", 50*time.Second))
	})

	It("should reconcile managed os version channel object without a type", func() {
		managedOSVersionChannel.Spec.SyncInterval = "1m"
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		res, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: managedOSVersionChannel.Namespace,
				Name:      managedOSVersionChannel.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(0 * time.Second))
		Expect(res.Requeue).To(BeFalse())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions).To(HaveLen(1))
		Expect(managedOSVersionChannel.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.InvalidConfigurationReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(managedOSVersionChannel.Status.Conditions[0].Message).To(ContainSubstring("spec.Type can't be empty"))
		// Only synchronization failures are counted, not configuration issues
		Expect(managedOSVersionChannel.Status.Failures).To(Equal(uint32(0)))
	})

	It("should reconcile managed os version channel object without a sync interval", func() {
		managedOSVersionChannel.Spec.Type = "custom"
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		res, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: managedOSVersionChannel.Namespace,
				Name:      managedOSVersionChannel.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(0 * time.Second))
		Expect(res.Requeue).To(BeFalse())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions).To(HaveLen(1))
		Expect(managedOSVersionChannel.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.InvalidConfigurationReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(managedOSVersionChannel.Status.Conditions[0].Message).To(ContainSubstring("spec.SyncInterval is not parseable"))
		// Only synchronization failures are counted, not configuration issues
		Expect(managedOSVersionChannel.Status.Failures).To(Equal(uint32(0)))
	})

	It("should reconcile managed os version channel object with a not valid sync interval", func() {
		managedOSVersionChannel.Spec.Type = "custom"
		managedOSVersionChannel.Spec.SyncInterval = "notATimeDuration"
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		res, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: managedOSVersionChannel.Namespace,
				Name:      managedOSVersionChannel.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(0 * time.Second))
		Expect(res.Requeue).To(BeFalse())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions).To(HaveLen(1))
		Expect(managedOSVersionChannel.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.InvalidConfigurationReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(managedOSVersionChannel.Status.Conditions[0].Message).To(ContainSubstring("spec.SyncInterval is not parseable"))
		// Only synchronization failures are counted, not configuration issues
		Expect(managedOSVersionChannel.Status.Failures).To(Equal(uint32(0)))
	})

	It("should reconcile managed os version channel object with a not valid type", func() {
		syncerProvider.UnknownType = "unknown"
		managedOSVersionChannel.Spec.Type = "unknown"
		managedOSVersionChannel.Spec.SyncInterval = "1m"
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		res, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: managedOSVersionChannel.Namespace,
				Name:      managedOSVersionChannel.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(0 * time.Second))
		Expect(res.Requeue).To(BeFalse())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions).To(HaveLen(1))
		Expect(managedOSVersionChannel.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.InvalidConfigurationReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(managedOSVersionChannel.Status.Conditions[0].Message).To(ContainSubstring("failed to create a syncer"))
		// Only synchronization failures are counted, not configuration issues
		Expect(managedOSVersionChannel.Status.Failures).To(Equal(uint32(0)))
	})

	It("it fails to reconcile a managed os version channel when channel provides invalid JSON", func() {
		syncerProvider.JSON = invalidJSON
		managedOSVersionChannel.Spec.Type = "json"
		managedOSVersionChannel.Spec.SyncInterval = "1m"
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		res, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: managedOSVersionChannel.Namespace,
				Name:      managedOSVersionChannel.Name,
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(0 * time.Second))
		Expect(res.Requeue).To(BeTrue())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions).To(HaveLen(1))
		Expect(managedOSVersionChannel.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.FailedToSyncReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		Expect(managedOSVersionChannel.Status.Conditions[0].Message).To(ContainSubstring("Failed syncing channel"))
		Expect(managedOSVersionChannel.Status.Failures).To(Equal(uint32(1)))
	})

	It("it counts failures until retries are over", func() {
		syncerProvider.JSON = invalidJSON
		managedOSVersionChannel.Spec.Type = "json"
		managedOSVersionChannel.Spec.SyncInterval = "1m"
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: managedOSVersionChannel.Namespace,
				Name:      managedOSVersionChannel.Name,
			},
		}
		objKey := client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}

		for i := 1; i < maxRetries; i++ {
			res, err := r.Reconcile(ctx, req)
			Expect(err).To(HaveOccurred())
			Expect(res.RequeueAfter).To(Equal(0 * time.Second))
			Expect(res.Requeue).To(BeTrue())

			Expect(cl.Get(ctx, objKey, managedOSVersionChannel)).To(Succeed())

			Expect(managedOSVersionChannel.Status.Conditions).To(HaveLen(1))
			Expect(managedOSVersionChannel.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
			Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.FailedToSyncReason))
			Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(managedOSVersionChannel.Status.Conditions[0].Message).To(ContainSubstring("Failed syncing channel"))
			Expect(managedOSVersionChannel.Status.Failures).To(Equal(uint32(i)))
			Expect(managedOSVersionChannel.Status.LastSyncedTime).To(BeNil())
		}

		res, err := r.Reconcile(ctx, req)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(1 * time.Minute))
		Expect(res.Requeue).To(BeFalse())

		Expect(cl.Get(ctx, objKey, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions).To(HaveLen(1))
		Expect(managedOSVersionChannel.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.FailedToSyncReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(managedOSVersionChannel.Status.Conditions[0].Message).To(ContainSubstring("Failed syncing channel"))
		Expect(managedOSVersionChannel.Status.Failures).To(Equal(uint32(0)))
		Expect(managedOSVersionChannel.Status.LastSyncedTime).NotTo(BeNil())
	})
})
