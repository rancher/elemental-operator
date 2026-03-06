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
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/test"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("reconcile managed os version", func() {
	var r *ManagedOSVersionReconciler

	var managedOSImage *elementalv1.ManagedOSImage
	var referencedManagedOSVersion *elementalv1.ManagedOSVersion
	var standaloneManagedOSVersion *elementalv1.ManagedOSVersion

	BeforeEach(func() {
		r = &ManagedOSVersionReconciler{
			Client: cl,
			Scheme: cl.Scheme(),
		}
		referencedManagedOSVersion = &elementalv1.ManagedOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-referenced",
				Namespace: "default",
			},
			Spec: elementalv1.ManagedOSVersionSpec{},
		}
		managedOSImage = &elementalv1.ManagedOSImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-image",
				Namespace: "default",
				Labels: map[string]string{
					elementalv1.ElementalManagedOSImageVersionNameLabel: referencedManagedOSVersion.Name,
				},
			},
			Spec: elementalv1.ManagedOSImageSpec{
				ManagedOSVersionName: referencedManagedOSVersion.Name,
			},
		}
		standaloneManagedOSVersion = &elementalv1.ManagedOSVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-standalone",
				Namespace: "default",
			},
			Spec: elementalv1.ManagedOSVersionSpec{},
		}

	})

	AfterEach(func() {
		Expect(test.CleanupAndWait(ctx, cl, managedOSImage, referencedManagedOSVersion, standaloneManagedOSVersion)).To(Succeed())
	})

	It("should delete standalone ManagedOSVersion", func() {
		Expect(cl.Create(ctx, standaloneManagedOSVersion)).To(Succeed())

		Eventually(func() error {
			_, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: standaloneManagedOSVersion.Namespace,
					Name:      standaloneManagedOSVersion.Name,
				},
			})
			return err
		}).WithTimeout(time.Minute).ShouldNot(HaveOccurred())

		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      standaloneManagedOSVersion.Name,
			Namespace: standaloneManagedOSVersion.Namespace,
		}, standaloneManagedOSVersion)).To(Succeed())
		Expect(controllerutil.ContainsFinalizer(standaloneManagedOSVersion, elementalv1.ManagedOSVersionFinalizer)).To(BeTrue(), "ManagedOSVersion must contain finalizer after first reconcile")

		// Reconcile after deletion
		Expect(cl.Delete(ctx, standaloneManagedOSVersion))
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: standaloneManagedOSVersion.Namespace,
				Name:      standaloneManagedOSVersion.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		err = cl.Get(ctx, client.ObjectKey{
			Name:      standaloneManagedOSVersion.Name,
			Namespace: standaloneManagedOSVersion.Namespace,
		}, standaloneManagedOSVersion)
		Expect(apierrors.IsNotFound(err)).Should(BeTrue(), "Standalone ManagedOSVersion should have been deleted")
	})
	It("should hold referenced ManagedOSVersion deletion", func() {
		Expect(cl.Create(ctx, referencedManagedOSVersion)).To(Succeed())
		Expect(cl.Create(ctx, managedOSImage)).To(Succeed())

		// First reconcile
		Eventually(func() error {
			_, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: referencedManagedOSVersion.Namespace,
					Name:      referencedManagedOSVersion.Name,
				},
			})
			return err
		}).WithTimeout(time.Minute).ShouldNot(HaveOccurred())

		// Reconcile after deletion
		Expect(cl.Delete(ctx, referencedManagedOSVersion))
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: referencedManagedOSVersion.Namespace,
				Name:      referencedManagedOSVersion.Name,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		// Expect referencedManagedOSVersion to not have been deleted
		Expect(cl.Get(ctx, client.ObjectKey{
			Name:      referencedManagedOSVersion.Name,
			Namespace: referencedManagedOSVersion.Namespace,
		}, referencedManagedOSVersion)).Should(Succeed())

		// Deleted the referencing ManagedOSImage
		Expect(cl.Delete(ctx, managedOSImage))
		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      managedOSImage.Name,
				Namespace: managedOSImage.Namespace,
			}, managedOSImage)
			return apierrors.IsNotFound(err)
		}).WithTimeout(time.Minute).Should(BeTrue(), "ManagedOSImage should have been deleted")

		requestsFromImage := r.ManagedOSImageToManagedOSVersion(ctx, managedOSImage)
		for _, request := range requestsFromImage {
			_, err = r.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
		}

		Eventually(func() bool {
			err := cl.Get(ctx, client.ObjectKey{
				Name:      referencedManagedOSVersion.Name,
				Namespace: referencedManagedOSVersion.Namespace,
			}, referencedManagedOSVersion)
			return apierrors.IsNotFound(err)
		}).WithTimeout(time.Minute).Should(BeTrue(), "Referenced ManagedOSVersion should have been deleted")

	})
})
