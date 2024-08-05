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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"
	ctrlHelpers "github.com/rancher/elemental-operator/tests/controllerHelpers"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
  }
]`

const updatedJSON = `[
  {
    "metadata": {
      "name": "v0.1.0"
    },
    "spec": {
      "version": "v0.1.0-patched",
      "type": "container",
      "metadata": {
        "upgradeImage": "foo/bar:v0.1.0-patched"
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

// v0.1.0 removed
const deprecatingJSON = `[
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
	var managedOSVersion *elementalv1.ManagedOSVersion
	var syncerProvider *ctrlHelpers.FakeSyncerProvider
	var pod *corev1.Pod
	var setPodPhase func(pod *corev1.Pod, phase corev1.PodPhase)

	BeforeEach(func() {
		managedOSVersion = &elementalv1.ManagedOSVersion{}
		syncerProvider = &ctrlHelpers.FakeSyncerProvider{}
		syncerProvider.SetJSON(syncJSON)
		r = &ManagedOSVersionChannelReconciler{
			Client:         cl,
			syncerProvider: syncerProvider,
			OperatorImage:  "test/image:latest",
		}

		managedOSVersionChannel = &elementalv1.ManagedOSVersionChannel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}

		setPodPhase = func(pod *corev1.Pod, phase corev1.PodPhase) {
			patchBase := client.MergeFrom(pod.DeepCopy())
			pod.Status.Phase = phase
			Expect(cl.Status().Patch(ctx, pod, patchBase)).To(Succeed())
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, managedOSVersionChannel, pod, managedOSVersion)).To(Succeed())
	})

	It("should reconcile and sync managed os version channel object", func() {
		managedOSVersionChannel.Spec.Type = "json"
		managedOSVersionChannel.Spec.SyncInterval = "1m"
		name := types.NamespacedName{
			Namespace: managedOSVersionChannel.Namespace,
			Name:      managedOSVersionChannel.Name,
		}
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		// No error and status updated (no requeue)
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.SyncingReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}, pod)).To(Succeed())

		Expect(pod.Status.Phase).To(Equal(corev1.PodPending))
		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).ToNot(HaveOccurred())

		setPodPhase(pod, corev1.PodRunning)
		res, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).ToNot(HaveOccurred())

		setPodPhase(pod, corev1.PodSucceeded)
		res, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(60 * time.Second))

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.SyncedReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))

		// Now the managed os vesion is already created
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      "v0.1.0",
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersion)).To(Succeed())

		// Synchronization done, pod deleted
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}, pod)).NotTo(Succeed())

		// Re-sync is triggered to interval
		res, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.RequeueAfter).To(BeNumerically("~", 59*time.Second, 1*time.Minute))
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

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.InvalidConfigurationReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	})

	It("should reconcile managed os version channel object with a bad a sync interval", func() {
		managedOSVersionChannel.Spec.Type = "custom"
		managedOSVersionChannel.Spec.SyncInterval = "badtime"
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

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.InvalidConfigurationReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
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

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.InvalidConfigurationReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	})

	It("it fails to reconcile a managed os version channel when channel provides invalid JSON", func() {
		syncerProvider.SetJSON(invalidJSON)
		managedOSVersionChannel.Spec.Type = "json"
		name := types.NamespacedName{
			Namespace: managedOSVersionChannel.Namespace,
			Name:      managedOSVersionChannel.Name,
		}
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		// No error and status updated (no requeue)
		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(0 * time.Second))

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.SyncingReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}, pod)).To(Succeed())

		setPodPhase(pod, corev1.PodSucceeded)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).To(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.FailedToSyncReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

		// Synchronization failed, pod deleted
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}, pod)).NotTo(Succeed())
	})

	It("it fails to reconcile when logs can't be read", func() {
		syncerProvider.LogsError = true
		managedOSVersionChannel.Spec.Type = "json"
		name := types.NamespacedName{
			Namespace: managedOSVersionChannel.Namespace,
			Name:      managedOSVersionChannel.Name,
		}
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		// No error and status updated (no requeue)
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.SyncingReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}, pod)).To(Succeed())

		Expect(pod.Status.Phase).To(Equal(corev1.PodPending))

		setPodPhase(pod, corev1.PodSucceeded)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).To(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.FailedToSyncReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

		// Synchronization failed, pod deleted
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}, pod)).NotTo(Succeed())
	})

	It("it fails to reconcile if syncer pod process fails", func() {
		managedOSVersionChannel.Spec.Type = "json"
		name := types.NamespacedName{
			Namespace: managedOSVersionChannel.Namespace,
			Name:      managedOSVersionChannel.Name,
		}
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		// No error and status updated (no requeue)
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.SyncingReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}, pod)).To(Succeed())

		Expect(pod.Status.Phase).To(Equal(corev1.PodPending))

		setPodPhase(pod, corev1.PodFailed)
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).To(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.FailedToSyncReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

		// Synchronization failed, pod deleted
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}, pod)).NotTo(Succeed())
	})

	It("it fails to reconcile if operator image is undefined", func() {
		r.OperatorImage = ""
		managedOSVersionChannel.Spec.Type = "json"
		name := types.NamespacedName{
			Namespace: managedOSVersionChannel.Namespace,
			Name:      managedOSVersionChannel.Name,
		}
		Expect(cl.Create(ctx, managedOSVersionChannel)).To(Succeed())

		// No error and status updated (no requeue)
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: name})
		Expect(err).To(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSVersionChannel.Name,
			Namespace: managedOSVersionChannel.Namespace,
		}, managedOSVersionChannel)).To(Succeed())

		Expect(managedOSVersionChannel.Status.Conditions[0].Reason).To(Equal(elementalv1.FailedToCreatePodReason))
		Expect(managedOSVersionChannel.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	})
})

var _ = Describe("managed os version channel controller integration tests", func() {
	var r *ManagedOSVersionChannelReconciler
	var ch *elementalv1.ManagedOSVersionChannel
	var managedOSVersion *elementalv1.ManagedOSVersion
	var syncerProvider *ctrlHelpers.FakeSyncerProvider
	var mgr manager.Manager
	var mgrCtx context.Context
	var mgrCancel context.CancelFunc
	var pod *corev1.Pod
	var setPodPhase func(pod *corev1.Pod, phase corev1.PodPhase)

	BeforeEach(func() {
		var err error
		managedOSVersion = &elementalv1.ManagedOSVersion{}

		mgr, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme: cl.Scheme(),
		})
		Expect(err).ToNot(HaveOccurred())

		syncerProvider = &ctrlHelpers.FakeSyncerProvider{}
		syncerProvider.SetJSON(syncJSON)
		r = &ManagedOSVersionChannelReconciler{
			Client:         cl,
			syncerProvider: syncerProvider,
			OperatorImage:  "test/image:latest",
		}
		Expect(r.SetupWithManager(mgr)).To(Succeed())

		ch = &elementalv1.ManagedOSVersionChannel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}

		setPodPhase = func(pod *corev1.Pod, phase corev1.PodPhase) {
			patchBase := client.MergeFrom(pod.DeepCopy())
			pod.Status.Phase = phase
			Expect(cl.Status().Patch(ctx, pod, patchBase)).To(Succeed())
		}

		mgrCtx, mgrCancel = context.WithCancel(ctx)
		go func() {
			defer GinkgoRecover()
			err = mgr.Start(mgrCtx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()

	})

	AfterEach(func() {
		if mgrCancel != nil {
			mgrCancel()
		}
		Expect(test.CleanupAndWait(ctx, cl, ch, pod)).To(Succeed())

		list := &elementalv1.ManagedOSVersionList{}
		Expect(cl.List(ctx, list)).To(Succeed())
		for _, version := range list.Items {
			Expect(test.CleanupAndWait(ctx, cl, &version)).To(Succeed())
		}
	})

	It("should reconcile and sync managed os version channel object and apply channel updates", func() {
		ch.Spec.Type = "json"

		Expect(cl.Create(ctx, ch)).To(Succeed())

		// Pod is created
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err == nil
		}, 12*time.Second, 2*time.Second).Should(BeTrue())
		setPodPhase(pod, corev1.PodSucceeded)

		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, ch)
			return err == nil && ch.Status.Conditions[0].Status == metav1.ConditionTrue
		}, 12*time.Second, 2*time.Second).Should(BeTrue())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      "v0.1.0",
			Namespace: ch.Namespace,
		}, managedOSVersion)).To(Succeed())
		Expect(managedOSVersion.Spec.Version).To(Equal("v0.1.0"))

		// Pod is deleted
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err != nil && apierrors.IsNotFound(err)
		}, 6*time.Second, 1*time.Second).Should(BeTrue())

		// Simulate a channel content change
		syncerProvider.SetJSON(updatedJSON)

		// Updating the channel causes an automatic update
		patchBase := client.MergeFrom(ch.DeepCopy())
		ch.Spec.SyncInterval = "10s"
		Expect(cl.Patch(ctx, ch, patchBase)).To(Succeed())

		// Pod is created
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err == nil
		}, 6*time.Second, 1*time.Second).Should(BeTrue())
		setPodPhase(pod, corev1.PodSucceeded)

		// New added versions are synced
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      "v0.2.0",
				Namespace: ch.Namespace,
			}, managedOSVersion)
			return err == nil
		}, 6*time.Second, 1*time.Second).Should(BeTrue())

		// After channel update already existing versions were patched
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      "v0.1.0",
			Namespace: ch.Namespace,
		}, managedOSVersion)).To(Succeed())
		Expect(managedOSVersion.Spec.Version).To(Equal("v0.1.0-patched"))

		// Pod is deleted
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err != nil && apierrors.IsNotFound(err)
		}, 2*time.Second, 1*time.Second).Should(BeTrue())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      ch.Name,
			Namespace: ch.Namespace,
		}, ch)).To(Succeed())

		// Simulate another channel content change
		syncerProvider.SetJSON(deprecatingJSON)

		timeout := time.Until(ch.Status.LastSyncedTime.Add(10*time.Second)) - 1*time.Second

		// No pod is created during the interval
		Consistently(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return apierrors.IsNotFound(err)
		}, timeout, 1*time.Second).Should(BeTrue())

		// Pod is created once the resync is triggered automatically
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err == nil
		}, 4*time.Second, 1*time.Second).Should(BeTrue())
		setPodPhase(pod, corev1.PodSucceeded)

		// v0.1.0 becomes a managedOSVersion out of sync
		Eventually(func() bool {
			Expect(cl.Get(ctx, client.ObjectKey{
				Name:      "v0.1.0",
				Namespace: ch.Namespace,
			}, managedOSVersion)).To(Succeed())

			return managedOSVersion.Annotations[elementalv1.ElementalManagedOSVersionNoLongerSyncedAnnotation] == elementalv1.ElementalManagedOSVersionNoLongerSyncedValue
		}, 6*time.Second, 1*time.Second).Should(BeTrue())
	})

	It("should deprecate a version after it's removed from channel", func() {
		ch.Spec.Type = "json"

		Expect(cl.Create(ctx, ch)).To(Succeed())

		// Pod is created
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err == nil
		}, 12*time.Second, 2*time.Second).Should(BeTrue())
		setPodPhase(pod, corev1.PodSucceeded)

		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, ch)
			return err == nil && ch.Status.Conditions[0].Status == metav1.ConditionTrue
		}, 12*time.Second, 2*time.Second).Should(BeTrue())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      "v0.1.0",
			Namespace: ch.Namespace,
		}, managedOSVersion)).To(Succeed())
		Expect(managedOSVersion.Spec.Version).To(Equal("v0.1.0"))

		// Pod is deleted
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err != nil && apierrors.IsNotFound(err)
		}, 12*time.Second, 2*time.Second).Should(BeTrue())

		// Simulate a channel content change
		syncerProvider.SetJSON(deprecatingJSON)

		// Updating the channel after the minimum time between syncs causes an automatic update
		patchBase := client.MergeFrom(ch.DeepCopy())
		ch.Spec.SyncInterval = "10m"
		Expect(cl.Patch(ctx, ch, patchBase)).To(Succeed())

		// Pod is created
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err == nil
		}, 12*time.Second, 2*time.Second).Should(BeTrue())
		setPodPhase(pod, corev1.PodSucceeded)

		// New added versions are synced
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      "v0.2.0",
				Namespace: ch.Namespace,
			}, managedOSVersion)
			return err == nil
		}, 12*time.Second, 2*time.Second).Should(BeTrue())
		_, found := managedOSVersion.Annotations[elementalv1.ElementalManagedOSVersionNoLongerSyncedAnnotation]
		Expect(found).To(BeFalse(), "no-longer-synced annotation must not be present when versions are actually synced")
		Expect(managedOSVersion.Annotations[elementalv1.ElementalManagedOSVersionChannelLastSyncAnnotation]).ToNot(BeEmpty(), "Last sync annotation should contain the UTC timestamp")

		// After channel update already existing versions were patched
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      "v0.1.0",
			Namespace: ch.Namespace,
		}, managedOSVersion)).To(Succeed())
		Expect(managedOSVersion.Annotations[elementalv1.ElementalManagedOSVersionNoLongerSyncedAnnotation]).To(Equal(elementalv1.ElementalManagedOSVersionNoLongerSyncedValue))
	})

	It("should auto-delete a version after it's removed from channel", func() {
		ch.Spec.Type = "json"
		ch.Spec.DeleteNoLongerInSyncVersions = true

		Expect(cl.Create(ctx, ch)).To(Succeed())

		// Pod is created
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err == nil
		}, 12*time.Second, 2*time.Second).Should(BeTrue())
		setPodPhase(pod, corev1.PodSucceeded)

		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, ch)
			return err == nil && ch.Status.Conditions[0].Status == metav1.ConditionTrue
		}, 12*time.Second, 2*time.Second).Should(BeTrue())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      "v0.1.0",
			Namespace: ch.Namespace,
		}, managedOSVersion)).To(Succeed())
		Expect(managedOSVersion.Spec.Version).To(Equal("v0.1.0"))

		// Pod is deleted
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err != nil && apierrors.IsNotFound(err)
		}, 12*time.Second, 2*time.Second).Should(BeTrue())

		// Simulate a channel content change
		syncerProvider.SetJSON(deprecatingJSON)

		// Updating the channel after the minimum time between syncs causes an automatic update
		patchBase := client.MergeFrom(ch.DeepCopy())
		ch.Spec.SyncInterval = "10m"
		Expect(cl.Patch(ctx, ch, patchBase)).To(Succeed())

		// Pod is created
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err == nil
		}, 12*time.Second, 2*time.Second).Should(BeTrue())
		setPodPhase(pod, corev1.PodSucceeded)

		// Check deprecated version was deleted
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      "v0.1.0",
				Namespace: ch.Namespace,
			}, managedOSVersion)
			return apierrors.IsNotFound(err)
		}, 12*time.Second, 2*time.Second).Should(BeTrue(), "No longer in sync version should have been deleted")
	})

	It("should not reconcile again if it errors during pod lifecycle", func() {
		ch.Spec.Type = "json"

		Expect(cl.Create(ctx, ch)).To(Succeed())

		// Pod is created
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err == nil
		}, 4*time.Second, 1*time.Second).Should(BeTrue())
		setPodPhase(pod, corev1.PodFailed)

		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, ch)
			return err == nil && ch.Status.Conditions[0].Reason == elementalv1.FailedToSyncReason
		}, 12*time.Second, 2*time.Second).Should(BeTrue())

		// Pod is deleted
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, pod)
			return err != nil && apierrors.IsNotFound(err)
		}, 12*time.Second, 2*time.Second).Should(BeTrue())
	})
})
