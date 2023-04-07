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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"
)

var _ = Describe("reconcile seed image", func() {
	var r *SeedImageReconciler
	var mRegistration *elementalv1.MachineRegistration
	var seedImg *elementalv1.SeedImage
	var setting *managementv3.Setting
	var pod *corev1.Pod
	var service *corev1.Service

	BeforeEach(func() {
		r = &SeedImageReconciler{
			Client:                   cl,
			SeedImageImage:           "quay.io/costoolkit/seedimage-builder:latest",
			SeedImageImagePullPolicy: corev1.PullIfNotPresent,
		}

		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}

		seedImg = &elementalv1.SeedImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
			Spec: elementalv1.SeedImageSpec{
				BaseImage: "missing-image",
				MachineRegistrationRef: &corev1.ObjectReference{
					Name:      mRegistration.Name,
					Namespace: mRegistration.Namespace,
					Kind:      mRegistration.Kind,
				},
			},
		}

		setting = &managementv3.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: "server-url",
			},
			Value: "https://example.com",
		}

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: seedImg.Namespace,
				Name:      seedImg.Name,
			},
		}

		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: seedImg.Namespace,
				Name:      seedImg.Name,
			},
		}

		Expect(cl.Create(ctx, mRegistration)).To(Succeed())
		Expect(cl.Create(ctx, seedImg)).To(Succeed())
		Expect(cl.Create(ctx, setting)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mRegistration, seedImg, setting, pod, service)).To(Succeed())
	})

	It("should reconcile seed image object", func() {
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: seedImg.Namespace,
				Name:      seedImg.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		}, seedImg)).To(Succeed())

		Expect(seedImg.Status.Conditions).To(HaveLen(2))
		Expect(seedImg.Status.Conditions[0].Type).To(Equal(elementalv1.ReadyCondition))
		Expect(seedImg.Status.Conditions[0].Reason).To(Equal(elementalv1.ResourcesSuccessfullyCreatedReason))
		Expect(seedImg.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(seedImg.Status.Conditions[1].Type).To(Equal(elementalv1.SeedImageConditionReady))
		Expect(seedImg.Status.Conditions[1].Status).To(Equal(metav1.ConditionFalse))

		objKey := types.NamespacedName{Namespace: seedImg.Namespace, Name: seedImg.Name}
		Expect(r.Get(ctx, objKey, &corev1.Pod{})).To(Succeed())
		Expect(r.Get(ctx, objKey, &corev1.Service{})).To(Succeed())

	})

})

var _ = Describe("reconcileBuildImagePod", func() {
	var r *SeedImageReconciler
	var mRegistration *elementalv1.MachineRegistration
	var seedImg *elementalv1.SeedImage
	var pod *corev1.Pod
	var svc *corev1.Service

	BeforeEach(func() {
		r = &SeedImageReconciler{
			Client:                   cl,
			SeedImageImage:           "quay.io/costoolkit/seedimage-builder:latest",
			SeedImageImagePullPolicy: corev1.PullIfNotPresent,
		}

		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}

		seedImg = &elementalv1.SeedImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
			Spec: elementalv1.SeedImageSpec{
				BaseImage: "https://example.com/base.iso",
				MachineRegistrationRef: &corev1.ObjectReference{
					Name:      mRegistration.Name,
					Namespace: mRegistration.Namespace,
				},
			},
		}

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      seedImg.Name,
				Namespace: seedImg.Namespace,
			},
		}

		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: seedImg.Namespace,
				Name:      seedImg.Name,
			},
		}

		Expect(cl.Create(ctx, mRegistration)).To(Succeed())
		Expect(cl.Create(ctx, seedImg)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mRegistration, seedImg, pod, svc)).To(Succeed())
	})

	It("should return error when a pod with the same name but different owner is there", func() {
		pod.Spec.Containers = []corev1.Container{
			{
				Name:  "nginx",
				Image: "nginx:latest",
			},
		}

		Expect(cl.Create(ctx, pod)).To(Succeed())

		err := r.reconcileBuildImagePod(ctx, seedImg)

		// Pod already there and not owned by the SeedImage obj
		Expect(err).To(HaveOccurred())
	})

	It("should recreate the pod if the pod is owned but the base iso is different", func() {

		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: seedImg.Namespace,
				Name:      seedImg.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		}, seedImg)).To(Succeed())

		foundPod := &corev1.Pod{}
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		}, foundPod)).To(Succeed())
		Expect(foundPod.Annotations["elemental.cattle.io/base-image"]).To(Equal(seedImg.Spec.BaseImage))

		seedImg.Spec.BaseImage = "https://example.com/new-base.iso"
		err = r.reconcileBuildImagePod(ctx, seedImg)
		Expect(err).ToNot(HaveOccurred())
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		}, foundPod)).To(Succeed())
		Expect(foundPod.Annotations["elemental.cattle.io/base-image"]).To(Equal(seedImg.Spec.BaseImage))
	})

	It("should recreate the pod if the pod is owned but the cloud-config is different", func() {
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: seedImg.Namespace,
				Name:      seedImg.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		}, seedImg)).To(Succeed())

		foundPod := &corev1.Pod{}
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		}, foundPod)).To(Succeed())
		Expect(foundPod.Annotations["elemental.cattle.io/cloud-config-b64"]).To(Equal(""))

		seedImg.Spec.CloudConfig = map[string]runtime.RawExtension{
			"write_files": {},
		}

		err = r.reconcileBuildImagePod(ctx, seedImg)
		Expect(err).ToNot(HaveOccurred())
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		}, foundPod)).To(Succeed())
		Expect(foundPod.Annotations["elemental.cattle.io/cloud-config-b64"]).To(Equal("I2Nsb3VkLWNvbmZpZwp3cml0ZV9maWxlczoKbnVsbAo="))
	})
})
