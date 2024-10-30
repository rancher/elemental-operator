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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"
	fleetv1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/rancher/wrangler/v2/pkg/genericcondition"
	"github.com/rancher/wrangler/v2/pkg/name"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("reconcile managed os image", func() {
	var r *ManagedOSImageReconciler
	var managedOSImage *elementalv1.ManagedOSImage
	var bundle *fleetv1.Bundle

	BeforeEach(func() {
		r = &ManagedOSImageReconciler{
			Client: cl,
			Scheme: cl.Scheme(),
		}

		managedOSImage = &elementalv1.ManagedOSImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}

		bundle = &fleetv1.Bundle{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeConcatName("mos", managedOSImage.Name),
				Namespace: managedOSImage.Namespace,
			},
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, managedOSImage, bundle)).To(Succeed())
	})

	It("should reconcile managed os image object", func() {
		Expect(cl.Create(ctx, managedOSImage)).To(Succeed())

		Eventually(func() error {
			_, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: managedOSImage.Namespace,
					Name:      managedOSImage.Name,
				},
			})
			return err
		}).WithTimeout(time.Minute).ShouldNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      managedOSImage.Name,
			Namespace: managedOSImage.Namespace,
		}, managedOSImage)).To(Succeed())

		bundle := &fleetv1.Bundle{}
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      name.SafeConcatName("mos", managedOSImage.Name),
			Namespace: managedOSImage.Namespace,
		}, bundle)).To(Succeed())
	})
})

var _ = Describe("newFleetBundleResources", func() {
	var r *ManagedOSImageReconciler
	var managedOSImage *elementalv1.ManagedOSImage
	var managedOSVersion *elementalv1.ManagedOSVersion

	BeforeEach(func() {
		r = &ManagedOSImageReconciler{
			Client: cl,
			Scheme: cl.Scheme(),
		}

		managedOSImage = &elementalv1.ManagedOSImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
		}

		managedOSVersion = &elementalv1.ManagedOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: elementalv1.ManagedOSVersionSpec{
				Type: "container",
				Metadata: map[string]runtime.RawExtension{
					"upgradeImage": {Raw: []byte(`"foo/bar:image"`)},
					"displayName":  {Raw: []byte(`"Unit Test Image"`)},
				},
			},
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, managedOSImage, managedOSVersion)).To(Succeed())
	})

	It("should create fleet bundle resources", func() {
		bundleResources, err := r.newFleetBundleResources(ctx, managedOSImage)
		Expect(err).ToNot(HaveOccurred())

		Expect(bundleResources).To(HaveLen(5))

		Expect(bundleResources[0].Name).To(Equal("ClusterRole--os-upgrader-test-name-28ceb391618a.yaml"))
		Expect(bundleResources[1].Name).To(Equal("ClusterRoleBinding--os-upgrader-test-name-cc7ce4275b54.yaml"))
		Expect(bundleResources[2].Name).To(Equal("ServiceAccount-cattle-system-os-upgrader-test-name-08929531f5c0.yaml"))
		Expect(bundleResources[3].Name).To(Equal("Secret-cattle-system-os-upgrader-test-name-52e9d8e041f4.yaml"))
		Expect(bundleResources[4].Name).To(Equal("Plan-cattle-system-os-upgrader-test-name-b3e84b45c9e0.yaml"))
	})

	It("should create fleet bundle when managedOSVersion exists", func() {
		Expect(r.Create(ctx, managedOSVersion)).To(Succeed())

		managedOSImage.Spec.ManagedOSVersionName = "test"
		bundleResources, err := r.newFleetBundleResources(ctx, managedOSImage)
		Expect(err).ToNot(HaveOccurred())

		Expect(bundleResources).To(HaveLen(5))
		plan := &upgradev1.Plan{}
		Expect(json.Unmarshal([]byte(bundleResources[4].Content), plan)).To(Succeed())
		Expect(plan.Spec.Upgrade.Image).To(Equal("foo/bar"))
		Expect(plan.Spec.Version).To(Equal("image"))
	})

	It("return error of managedOSVersion doesn't exist", func() {
		managedOSImage.Spec.ManagedOSVersionName = "test"
		bundleResources, err := r.newFleetBundleResources(ctx, managedOSImage)
		Expect(err).To(HaveOccurred())
		Expect(bundleResources).To(BeNil())
	})

	It("should convert ManagedOSImage name to DNS Label standard", func() {
		managedOSImage.Name = "test.name"
		bundleResources, err := r.newFleetBundleResources(ctx, managedOSImage)
		Expect(err).ToNot(HaveOccurred())

		Expect(bundleResources[0].Name).To(Equal("ClusterRole--os-upgrader-test-name-28ceb391618a.yaml"))
		Expect(bundleResources[1].Name).To(Equal("ClusterRoleBinding--os-upgrader-test-name-cc7ce4275b54.yaml"))
		Expect(bundleResources[2].Name).To(Equal("ServiceAccount-cattle-system-os-upgrader-test-name-08929531f5c0.yaml"))
		Expect(bundleResources[3].Name).To(Equal("Secret-cattle-system-os-upgrader-test-name-52e9d8e041f4.yaml"))
		Expect(bundleResources[4].Name).To(Equal("Plan-cattle-system-os-upgrader-test-name-8130169de1eb.yaml"))
	})
	It("should add correlation id and snapshot labels", func() {
		wantLabelsEnv := corev1.EnvVar{
			Name:  "ELEMENTAL_REGISTER_UPGRADE_SNAPSHOT_LABELS",
			Value: "managedOSImage=test-name,image=foo/bar:image,managedOSVersion=test",
		}
		wantCorrelationIDEnv := corev1.EnvVar{
			Name:  "ELEMENTAL_REGISTER_UPGRADE_CORRELATION_ID",
			Value: "6c18cee8fa6945d5661a39aee878c8035780b5d2e62e1509d154a2bd",
		}

		Expect(r.Create(ctx, managedOSVersion)).To(Succeed())
		managedOSImage.Spec.ManagedOSVersionName = "test"
		bundleResources, err := r.newFleetBundleResources(ctx, managedOSImage)
		Expect(err).ToNot(HaveOccurred())

		Expect(bundleResources).To(HaveLen(5))
		plan := &upgradev1.Plan{}
		Expect(json.Unmarshal([]byte(bundleResources[4].Content), plan)).To(Succeed())
		Expect(plan.Spec.Upgrade.Env).Should(ContainElement(wantLabelsEnv))
		Expect(plan.Spec.Upgrade.Env).Should(ContainElement(wantCorrelationIDEnv))

		value, found := plan.Labels[elementalv1.ElementalUpgradeCorrelationIDLabel]
		Expect(found).Should(BeTrue(), "Plan should have correlation ID as label")
		Expect(value).Should(Equal("6c18cee8fa6945d5661a39aee878c8035780b5d2e62e1509d154a2bd"))

		value, found = managedOSImage.Labels[elementalv1.ElementalUpgradeCorrelationIDLabel]
		Expect(found).Should(BeTrue(), "ManagedOSImage should have correlation ID as label")
		Expect(value).Should(Equal("6c18cee8fa6945d5661a39aee878c8035780b5d2e62e1509d154a2bd"))
	})
})

var _ = Describe("createFleetBundle", func() {
	var r *ManagedOSImageReconciler
	var managedOSImage *elementalv1.ManagedOSImage
	var bundle *fleetv1.Bundle

	BeforeEach(func() {
		r = &ManagedOSImageReconciler{
			Client: cl,
			Scheme: cl.Scheme(),
		}

		managedOSImage = &elementalv1.ManagedOSImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
				UID:       "test",
			},
			Spec: elementalv1.ManagedOSImageSpec{
				ClusterRolloutStrategy: &fleetv1.RolloutStrategy{
					MaxUnavailable: &intstr.IntOrString{
						IntVal: 3,
					},
				},
				Targets: []fleetv1.BundleTarget{
					{
						Name: "test",
					},
				},
			},
		}

		bundle = &fleetv1.Bundle{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeConcatName("mos", managedOSImage.Name),
				Namespace: managedOSImage.Namespace,
			},
		}
	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, managedOSImage, bundle)).To(Succeed())
	})

	It("should create fleet bundle resources", func() {
		err := r.createFleetBundle(ctx, managedOSImage, []fleetv1.BundleResource{
			{
				Name: "test",
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      name.SafeConcatName("mos", managedOSImage.Name),
			Namespace: managedOSImage.Namespace,
		}, bundle)).To(Succeed())

		Expect(bundle.Spec.Resources).To(HaveLen(1))
		Expect(bundle.Spec.Resources[0].Name).To(Equal("test"))
		Expect(bundle.Spec.RolloutStrategy).ToNot(BeNil())
		Expect(bundle.Spec.RolloutStrategy.MaxUnavailable.IntVal).To(Equal(int32(3)))
		Expect(bundle.Spec.Targets).To(HaveLen(1))
		Expect(bundle.Spec.Targets[0].Name).To(Equal("test"))
	})

	It("should change target when namespace is fleet-local", func() {
		managedOSImage.Namespace = fleetLocalNamespace
		err := r.createFleetBundle(ctx, managedOSImage, []fleetv1.BundleResource{
			{
				Name: "test",
			},
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      r.formatBundleName(*managedOSImage),
			Namespace: managedOSImage.Namespace,
		}, bundle)).To(Succeed())

		Expect(bundle.Spec.Resources).To(HaveLen(1))
		Expect(bundle.Spec.Resources[0].Name).To(Equal("test"))
		Expect(bundle.Spec.RolloutStrategy).ToNot(BeNil())
		Expect(bundle.Spec.RolloutStrategy.MaxUnavailable.IntVal).To(Equal(int32(3)))
		Expect(bundle.Spec.Targets).To(HaveLen(1))
		Expect(bundle.Spec.Targets[0].ClusterName).To(Equal("local"))
	})

	It("update fleet bundle if already exists", func() {
		Expect(r.createFleetBundle(ctx, managedOSImage, []fleetv1.BundleResource{})).Should(Succeed())
		meta.SetStatusCondition(&managedOSImage.Status.Conditions, metav1.Condition{
			Type:   elementalv1.FleetBundleCreation,
			Reason: elementalv1.FleetBundleCreateSuccessReason,
			Status: metav1.ConditionTrue,
		})

		wantResource := []fleetv1.BundleResource{{Name: "test", Content: "test-content"}}
		Expect(r.updateFleetBundle(ctx, managedOSImage, wantResource)).Should(Succeed())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      r.formatBundleName(*managedOSImage),
			Namespace: managedOSImage.Namespace,
		}, bundle)).To(Succeed())
		Expect(bundle.Spec.Resources).To(Equal(wantResource))
	})
})

var _ = Describe("updateManagedOSImageStatus", func() {
	var r *ManagedOSImageReconciler
	var managedOSImage *elementalv1.ManagedOSImage
	var bundle *fleetv1.Bundle

	BeforeEach(func() {
		r = &ManagedOSImageReconciler{
			Client: cl,
			Scheme: cl.Scheme(),
		}

		managedOSImage = &elementalv1.ManagedOSImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "default",
			},
			Spec: elementalv1.ManagedOSImageSpec{
				ClusterRolloutStrategy: &fleetv1.RolloutStrategy{
					MaxUnavailable: &intstr.IntOrString{
						IntVal: 3,
					},
				},
				Targets: []fleetv1.BundleTarget{
					{
						Name: "test",
					},
				},
			},
		}

		bundle = &fleetv1.Bundle{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeConcatName("mos", managedOSImage.Name),
				Namespace: managedOSImage.Namespace,
			},
		}

	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, managedOSImage, bundle)).To(Succeed())
	})

	It("should update manage os image status", func() {
		Expect(r.Create(ctx, bundle)).To(Succeed())

		patchBase := client.MergeFrom(bundle.DeepCopy())

		bundle.Status = fleetv1.BundleStatus{
			Conditions: []genericcondition.GenericCondition{
				{
					Type:    "test",
					Status:  corev1.ConditionTrue,
					Reason:  "test",
					Message: "test",
				},
			},
		}
		Expect(r.Status().Patch(ctx, bundle, patchBase)).To(Succeed())

		err := r.updateManagedOSImageStatus(ctx, managedOSImage)
		Expect(err).ToNot(HaveOccurred())

		Expect(managedOSImage.Status.Conditions).To(HaveLen(1))
		Expect(managedOSImage.Status.Conditions[0].Type).To(Equal("test"))
		Expect(managedOSImage.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		Expect(managedOSImage.Status.Conditions[0].Reason).To(Equal("test"))
	})

	It("should update manage os image status when condition is missing fields", func() {
		Expect(r.Create(ctx, bundle)).To(Succeed())

		patchBase := client.MergeFrom(bundle.DeepCopy())

		bundle.Status = fleetv1.BundleStatus{
			Conditions: []genericcondition.GenericCondition{
				{
					Message: "test",
				},
			},
		}
		Expect(r.Status().Patch(ctx, bundle, patchBase)).To(Succeed())

		err := r.updateManagedOSImageStatus(ctx, managedOSImage)
		Expect(err).ToNot(HaveOccurred())

		Expect(managedOSImage.Status.Conditions).To(HaveLen(1))
		Expect(managedOSImage.Status.Conditions[0].Type).To(Equal("UnknownType"))
		Expect(managedOSImage.Status.Conditions[0].Status).To(Equal(metav1.ConditionUnknown))
		Expect(managedOSImage.Status.Conditions[0].Reason).To(Equal("UnknownReason"))
	})
})

var _ = Describe("metadataEnv", func() {
	It("should container env variables when passed as strings", func() {
		envMap := map[string]runtime.RawExtension{
			"upgradeImage": {Raw: []byte(`"registry.com/repository/image:v1.0"`)},
			"robin":        {Raw: []byte(`"batman"`)},
		}
		env := metadataEnv(envMap)
		Expect(env).To(HaveLen(2))
		for _, e := range env {
			if e.Name == "METADATA_UPGRADEIMAGE" {
				Expect(e.Value).To(Equal("registry.com/repository/image:v1.0"))
			} else {
				Expect(e.Name).To(Equal("METADATA_ROBIN"))
				Expect(e.Value).To(Equal("batman"))
			}
		}

	})

	It("should container env variables when passed as jsondata", func() {
		jsonStruct := struct{ Foo string }{Foo: "foostruct"}
		jsonData, err := json.Marshal(jsonStruct)
		Expect(err).ToNot(HaveOccurred())

		envMap := map[string]runtime.RawExtension{
			"jsondata": {Raw: jsonData},
		}
		env := metadataEnv(envMap)
		Expect(env).To(HaveLen(1))
		Expect(env[0].Name).To(Equal("METADATA_JSONDATA"))
		Expect(env[0].Value).To(Equal("{\"Foo\":\"foostruct\"}"))
	})
})

var _ = Describe("getImageVersion", func() {
	It("should format images with no registry and no version", func() {
		osImage := &elementalv1.ManagedOSImage{
			Spec: elementalv1.ManagedOSImageSpec{
				OSImage: "a-test-image",
			},
		}
		image, version, err := getImageVersion(osImage, nil)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(version).Should(Equal("latest"))
		Expect(image).Should(Equal(osImage.Spec.OSImage))
	})
	It("should format images with no version", func() {
		osImage := &elementalv1.ManagedOSImage{
			Spec: elementalv1.ManagedOSImageSpec{
				OSImage: "a-test-registry/a-test-image",
			},
		}
		image, version, err := getImageVersion(osImage, nil)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(version).Should(Equal("latest"))
		Expect(image).Should(Equal(osImage.Spec.OSImage))
	})
	It("should format images with fully qualified name", func() {
		osImage := &elementalv1.ManagedOSImage{
			Spec: elementalv1.ManagedOSImageSpec{
				OSImage: "a-test-registry/a-test-image:my-version",
			},
		}
		image, version, err := getImageVersion(osImage, nil)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(version).Should(Equal("my-version"))
		Expect(image).Should(Equal("a-test-registry/a-test-image"))
	})
	It("should format images with non standard registry ports", func() {
		osImage := &elementalv1.ManagedOSImage{
			Spec: elementalv1.ManagedOSImageSpec{
				OSImage: "a-test-registry:30000/a-test-image:my-version",
			},
		}
		image, version, err := getImageVersion(osImage, nil)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(version).Should(Equal("my-version"))
		Expect(image).Should(Equal("a-test-registry:30000/a-test-image"))
	})
	It("should format images with non standard registry ports and no version", func() {
		osImage := &elementalv1.ManagedOSImage{
			Spec: elementalv1.ManagedOSImageSpec{
				OSImage: "a-test-registry:30000/a-test-image",
			},
		}
		image, version, err := getImageVersion(osImage, nil)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(version).Should(Equal("latest"))
		Expect(image).Should(Equal(osImage.Spec.OSImage))
	})
})

var _ = Describe("Plan naming", func() {
	It("should replace invalid characters with -", func() {
		input := "-my.invalid!name@"
		wanted := "my-invalid-name"
		Expect(toDNSLabel(input)).To(Equal(wanted))
	})
	It("should not replace valid strings", func() {
		wanted := "my-valid-name"
		Expect(toDNSLabel(wanted)).To(Equal(wanted))
	})
})
