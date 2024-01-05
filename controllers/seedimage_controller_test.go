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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
			SeedImageImage:           "registry.suse.com/rancher/seedimage-builder:latest",
			SeedImageImagePullPolicy: corev1.PullIfNotPresent,
		}

		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
			Status: elementalv1.MachineRegistrationStatus{
				RegistrationURL:   "https://example.com/token",
				RegistrationToken: "token",
				Conditions: []metav1.Condition{
					{
						Type:               elementalv1.ReadyCondition,
						Reason:             elementalv1.SuccessfullyCreatedReason,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				},
			},
			Spec: elementalv1.MachineRegistrationSpec{
				Config: &elementalv1.Config{
					Elemental: elementalv1.Elemental{
						Install: elementalv1.Install{},
					},
				},
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
				},
				Type: elementalv1.TypeIso,
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

		statusCopy := mRegistration.Status.DeepCopy()

		Expect(cl.Create(ctx, mRegistration)).To(Succeed())
		Expect(cl.Create(ctx, seedImg)).To(Succeed())
		Expect(cl.Create(ctx, setting)).To(Succeed())

		patchBase := client.MergeFrom(mRegistration.DeepCopy())
		mRegistration.Status = *statusCopy
		Expect(cl.Status().Patch(ctx, mRegistration, patchBase)).To(Succeed())
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
		Expect(seedImg.OwnerReferences[0].UID).To(Equal(mRegistration.UID))

		objKey := types.NamespacedName{Namespace: seedImg.Namespace, Name: seedImg.Name}
		Expect(r.Get(ctx, objKey, &corev1.Pod{})).To(Succeed())
		Expect(r.Get(ctx, objKey, &corev1.Service{})).To(Succeed())

	})

	It("should create the pod pulling the ISO from a container", func() {
		seedImg.Spec.BaseImage = "domain.org/some/repo:and-a-tag"

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
		Expect(len(foundPod.Spec.InitContainers)).To(Equal(2))
		Expect(foundPod.Spec.InitContainers[0].Image).To(Equal(seedImg.Spec.BaseImage))
	})

})

var _ = Describe("validate seed image", func() {
	var mRegistration *elementalv1.MachineRegistration
	var seedImg *elementalv1.SeedImage

	BeforeEach(func() {
		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name2",
				Namespace: "default",
			},
			Status: elementalv1.MachineRegistrationStatus{
				RegistrationURL:   "https://example.com/token",
				RegistrationToken: "token",
				Conditions: []metav1.Condition{
					{
						Type:               elementalv1.ReadyCondition,
						Reason:             elementalv1.SuccessfullyCreatedReason,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				},
			},
			Spec: elementalv1.MachineRegistrationSpec{
				Config: &elementalv1.Config{
					Elemental: elementalv1.Elemental{
						Install: elementalv1.Install{},
					},
				},
			},
		}

	})

	It("should fail validation on unknown platform format", func() {
		seedImg = &elementalv1.SeedImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
			Spec: elementalv1.SeedImageSpec{
				TargetPlatform: "test-platform",
				BaseImage:      "missing-image",
				MachineRegistrationRef: &corev1.ObjectReference{
					Name:      mRegistration.Name,
					Namespace: mRegistration.Namespace,
				},
				Type: elementalv1.TypeIso,
			},
		}

		Expect(cl.Create(ctx, mRegistration)).To(Succeed())
		err := cl.Create(ctx, seedImg)
		Expect(err).ToNot(Succeed())
		Expect(err.Error()).To(ContainSubstring("Invalid value: \"test-platform\": spec.targetPlatform in body should match '^$|^\\S+\\/\\S+$'"))

	})

	It("should pass validation on linux/amd64", func() {
		seedImg = &elementalv1.SeedImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
			Spec: elementalv1.SeedImageSpec{
				TargetPlatform: "linux/amd64",
				BaseImage:      "missing-image",
				MachineRegistrationRef: &corev1.ObjectReference{
					Name:      mRegistration.Name,
					Namespace: mRegistration.Namespace,
				},
				Type: elementalv1.TypeIso,
			},
		}

		Expect(cl.Create(ctx, mRegistration)).To(Succeed())
		Expect(cl.Create(ctx, seedImg)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mRegistration, seedImg)).To(Succeed())
	})

})

var _ = Describe("reconcile seed image build container", func() {
	var r *SeedImageReconciler
	var mRegistration *elementalv1.MachineRegistration
	var seedImg *elementalv1.SeedImage
	var setting *managementv3.Setting
	var pod *corev1.Pod
	var service *corev1.Service

	BeforeEach(func() {
		r = &SeedImageReconciler{
			Client:                   cl,
			SeedImageImage:           "registry.suse.com/rancher/seedimage-builder:latest",
			SeedImageImagePullPolicy: corev1.PullIfNotPresent,
		}

		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
			Status: elementalv1.MachineRegistrationStatus{
				RegistrationURL:   "https://example.com/token",
				RegistrationToken: "token",
				Conditions: []metav1.Condition{
					{
						Type:               elementalv1.ReadyCondition,
						Reason:             elementalv1.SuccessfullyCreatedReason,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				},
			},
			Spec: elementalv1.MachineRegistrationSpec{
				Config: &elementalv1.Config{
					Elemental: elementalv1.Elemental{
						Install: elementalv1.Install{},
					},
				},
			},
		}

		seedImg = &elementalv1.SeedImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
			Spec: elementalv1.SeedImageSpec{
				TargetPlatform: "linux/amd64",
				BaseImage:      "seed-image:latest",
				MachineRegistrationRef: &corev1.ObjectReference{
					Name:      mRegistration.Name,
					Namespace: mRegistration.Namespace,
				},
				BuildContainer: &elementalv1.BuildContainer{
					Image:           "test-seedimage-builder:latest",
					ImagePullPolicy: corev1.PullNever,
				},
				Type: elementalv1.TypeIso,
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

		statusCopy := mRegistration.Status.DeepCopy()

		Expect(cl.Create(ctx, mRegistration)).To(Succeed())
		Expect(cl.Create(ctx, seedImg)).To(Succeed())
		Expect(cl.Create(ctx, setting)).To(Succeed())

		patchBase := client.MergeFrom(mRegistration.DeepCopy())
		mRegistration.Status = *statusCopy
		Expect(cl.Status().Patch(ctx, mRegistration, patchBase)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mRegistration, seedImg, setting, pod, service)).To(Succeed())
	})

	It("should create the initContainer from a user-specified BuildContainer", func() {
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
		Expect(len(foundPod.Spec.InitContainers)).To(Equal(1))
		Expect(foundPod.Spec.InitContainers[0].Image).To(Equal(seedImg.Spec.BuildContainer.Image))
		Expect(foundPod.Spec.InitContainers[0].ImagePullPolicy).To(Equal(seedImg.Spec.BuildContainer.ImagePullPolicy))
		Expect(len(foundPod.Spec.InitContainers[0].Env)).To(Equal(4))
	})
})

var _ = Describe("reconcileBuildImagePod", func() {
	var r *SeedImageReconciler
	var setting *managementv3.Setting
	var mRegistration *elementalv1.MachineRegistration
	var seedImg *elementalv1.SeedImage
	var pod *corev1.Pod
	var svc *corev1.Service

	BeforeEach(func() {
		r = &SeedImageReconciler{
			Client:                   cl,
			SeedImageImage:           "registry.suse.com/rancher/seedimage-builder:latest",
			SeedImageImagePullPolicy: corev1.PullIfNotPresent,
		}

		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
			Status: elementalv1.MachineRegistrationStatus{
				RegistrationURL:   "https://example.com/token",
				RegistrationToken: "token",
				Conditions: []metav1.Condition{
					{
						Type:               elementalv1.ReadyCondition,
						Reason:             elementalv1.SuccessfullyCreatedReason,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				},
			},
			Spec: elementalv1.MachineRegistrationSpec{
				Config: &elementalv1.Config{
					Elemental: elementalv1.Elemental{
						Install: elementalv1.Install{},
					},
				},
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
				Type: elementalv1.TypeIso,
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

		setting = &managementv3.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: "server-url",
			},
			Value: "https://example.com",
		}

		statusCopy := mRegistration.Status.DeepCopy()

		Expect(cl.Create(ctx, mRegistration)).To(Succeed())
		Expect(cl.Create(ctx, seedImg)).To(Succeed())
		Expect(cl.Create(ctx, setting)).To(Succeed())

		patchBase := client.MergeFrom(mRegistration.DeepCopy())
		mRegistration.Status = *statusCopy
		Expect(cl.Status().Patch(ctx, mRegistration, patchBase)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mRegistration, setting, seedImg, pod, svc)).To(Succeed())
	})

	It("should return error when a pod with the same name but different owner is there", func() {
		pod.Spec.Containers = []corev1.Container{
			{
				Name:  "nginx",
				Image: "nginx:latest",
			},
		}

		Expect(cl.Create(ctx, pod)).To(Succeed())

		err := r.reconcileBuildImagePod(ctx, seedImg, mRegistration)

		// Pod already there and not owned by the SeedImage obj
		Expect(err).To(HaveOccurred())
	})

	It("should skip the reconcile loop when the SeedImageReady condition is met and the Image lifetime has expired", func() {
		seedImg.Status.Conditions = []metav1.Condition{
			{
				Type:   elementalv1.SeedImageConditionReady,
				Status: metav1.ConditionTrue,
				Reason: elementalv1.SeedImageBuildDeadline,
			},
		}

		err := r.reconcileBuildImagePod(ctx, seedImg, mRegistration)

		Expect(err).ToNot(HaveOccurred())
		foundPod := &corev1.Pod{}
		err = cl.Get(ctx, types.NamespacedName{
			Namespace: seedImg.Namespace,
			Name:      seedImg.Name,
		}, foundPod)

		Expect(apierrors.IsNotFound(err)).To(BeTrue())

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

		err = r.reconcileBuildImagePod(ctx, seedImg, mRegistration)
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

		seedImg.Spec.CloudConfig = map[string]runtime.RawExtension{
			"write_files": {},
		}

		err = r.reconcileBuildImagePod(ctx, seedImg, mRegistration)
		Expect(err).ToNot(HaveOccurred())
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		}, foundPod)).To(Succeed())
	})
})

var _ = Describe("createConfigMapObject", func() {
	var r *SeedImageReconciler
	var setting *managementv3.Setting
	var mRegistration *elementalv1.MachineRegistration
	var seedImg *elementalv1.SeedImage
	var configMap *corev1.ConfigMap
	var pod *corev1.Pod
	var svc *corev1.Service

	BeforeEach(func() {
		r = &SeedImageReconciler{
			Client:                   cl,
			SeedImageImage:           "registry.suse.com/rancher/seedimage-builder:latest",
			SeedImageImagePullPolicy: corev1.PullIfNotPresent,
		}

		mRegistration = &elementalv1.MachineRegistration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
			Status: elementalv1.MachineRegistrationStatus{
				RegistrationURL:   "https://example.com/token",
				RegistrationToken: "token",
				Conditions: []metav1.Condition{
					{
						Type:               elementalv1.ReadyCondition,
						Reason:             elementalv1.SuccessfullyCreatedReason,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				},
			},
			Spec: elementalv1.MachineRegistrationSpec{
				Config: &elementalv1.Config{
					Elemental: elementalv1.Elemental{
						Install: elementalv1.Install{},
					},
				},
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
				Type: elementalv1.TypeIso,
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
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: seedImg.Namespace,
				Name:      seedImg.Name,
			},
		}

		setting = &managementv3.Setting{
			ObjectMeta: metav1.ObjectMeta{
				Name: "server-url",
			},
			Value: "https://example.com",
		}

		statusCopy := mRegistration.Status.DeepCopy()

		Expect(cl.Create(ctx, mRegistration)).To(Succeed())
		Expect(cl.Create(ctx, seedImg)).To(Succeed())
		Expect(cl.Create(ctx, setting)).To(Succeed())

		patchBase := client.MergeFrom(mRegistration.DeepCopy())
		mRegistration.Status = *statusCopy
		Expect(cl.Status().Patch(ctx, mRegistration, patchBase)).To(Succeed())
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, mRegistration, seedImg, setting, configMap, pod, svc)).To(Succeed())
	})

	It("should create a configmap with empty cloud-config data", func() {
		Expect(r.reconcileConfigMapObject(ctx, seedImg, mRegistration)).To(Succeed())
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		}, configMap)).To(Succeed())
		Expect(len(configMap.BinaryData["cloud-config"])).To(Equal(0))
	})

	It("should create a configmap with non-empty cloud-config data", func() {
		config := []map[string]string{{
			"path":    "/some/path",
			"content": "file contents",
		}}
		rawConf, err := json.Marshal(config)
		Expect(err).ToNot(HaveOccurred())
		seedImg.Spec.CloudConfig = map[string]runtime.RawExtension{}
		seedImg.Spec.CloudConfig["write_files"] = runtime.RawExtension{Raw: rawConf}
		Expect(r.reconcileConfigMapObject(ctx, seedImg, mRegistration)).To(Succeed())
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      seedImg.Name,
			Namespace: seedImg.Namespace,
		}, configMap)).To(Succeed())
		Expect(string(configMap.BinaryData["cloud-config"])).
			To(ContainSubstring("#cloud-config\nwrite_files:"))
	})

	It("should fail if cloud-config can't be marshalled", func() {
		seedImg.Spec.CloudConfig = map[string]runtime.RawExtension{}
		seedImg.Spec.CloudConfig["write_files"] = runtime.RawExtension{Raw: []byte("invalid data")}
		Expect(r.reconcileConfigMapObject(ctx, seedImg, mRegistration)).ToNot(Succeed())
	})
})

var _ = Describe("updateStatusFromPod", func() {
	var r *SeedImageReconciler
	var seedImg *elementalv1.SeedImage
	var pod *corev1.Pod

	BeforeEach(func() {
		r = &SeedImageReconciler{
			Client:         cl,
			SeedImageImage: "registry.suse.com/rancher/seedimage-builder:latest",
		}
		seedImg = &elementalv1.SeedImage{
			Status: elementalv1.SeedImageStatus{
				Conditions: []metav1.Condition{
					{
						Type:   elementalv1.SeedImageConditionReady,
						Status: metav1.ConditionUnknown,
					},
				},
			},
		}

		pod = &corev1.Pod{}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, seedImg, pod)).To(Succeed())
	})

	It("should sync SeedImageReady condition to Pod phase", func() {
		expectedStates := []struct {
			podPhase           corev1.PodPhase
			errorExpected      bool
			imgConditionState  metav1.ConditionStatus
			imgConditionReason string
		}{
			{corev1.PodPending, false, metav1.ConditionFalse, elementalv1.SeedImageBuildOngoingReason},
			{corev1.PodRunning, true, metav1.ConditionFalse, elementalv1.SeedImageExposeFailureReason},
			{corev1.PodFailed, false, metav1.ConditionFalse, elementalv1.SeedImageBuildFailureReason},
			{corev1.PodSucceeded, true, metav1.ConditionFalse, elementalv1.SeedImageBuildDeadline},
			{corev1.PodUnknown, false, metav1.ConditionUnknown, elementalv1.SeedImageBuildUnknown},
		}

		for _, expectedState := range expectedStates {
			pod.Status.Phase = expectedState.podPhase
			err := r.updateStatusFromPod(ctx, seedImg, pod)
			if expectedState.errorExpected {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			annotation := fmt.Sprintf("pod phase: %s", expectedState.podPhase)
			Expect(seedImg.Status.Conditions[0].Status).To(Equal(expectedState.imgConditionState), annotation)
			Expect(seedImg.Status.Conditions[0].Reason).To(Equal(expectedState.imgConditionReason), annotation)
		}
	})

	It("should skip syncing when SeedImageReady condition is True", func() {
		seedImg.Status.Conditions = []metav1.Condition{
			{
				Type:   elementalv1.SeedImageConditionReady,
				Status: metav1.ConditionTrue,
				Reason: "should stay untouched",
			},
		}

		err := r.updateStatusFromPod(ctx, seedImg, pod)
		Expect(err).ToNot(HaveOccurred())
		Expect(seedImg.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(seedImg.Status.Conditions[0].Reason).To(Equal("should stay untouched"))
	})
})

var _ = Describe("fillBuildImagePod", func() {
	It("should provide default build container", func() {
		defaultBuildImg := "default-builder:latest"
		seedImg := &elementalv1.SeedImage{
			Spec: elementalv1.SeedImageSpec{
				BaseImage: "http://localhost/default-image.iso",
			},
		}

		pod := fillBuildImagePod(seedImg, defaultBuildImg, corev1.PullNever)

		Expect(len(pod.Spec.InitContainers)).To(Equal(1))
		Expect(pod.Spec.InitContainers[0].Image).To(Equal(defaultBuildImg))
		Expect(pod.Spec.InitContainers[0].ImagePullPolicy).To(Equal(corev1.PullNever))
	})

	It("should use user-provided build container", func() {
		buildImg := "custom-image:latest"

		quantity10M := *resource.NewQuantity(10*1024*1024, resource.BinarySI)

		seedImg := &elementalv1.SeedImage{
			Spec: elementalv1.SeedImageSpec{
				BuildContainer: &elementalv1.BuildContainer{
					Image:           buildImg,
					ImagePullPolicy: corev1.PullNever,
				},
				Size: quantity10M,
			},
		}

		pod := fillBuildImagePod(seedImg, "", corev1.PullNever)

		Expect(len(pod.Spec.InitContainers)).To(Equal(1))
		Expect(pod.Spec.InitContainers[0].Image).To(Equal(buildImg))
		Expect(pod.Spec.InitContainers[0].ImagePullPolicy).To(Equal(corev1.PullNever))
	})
})
