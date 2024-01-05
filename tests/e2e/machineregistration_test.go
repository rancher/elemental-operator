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

package e2e_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	http "github.com/rancher-sandbox/ele-testhelpers/http"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/tests/catalog"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("MachineRegistration e2e tests", func() {
	Context("registration", func() {
		var mRegistration *elementalv1.MachineRegistration

		BeforeEach(func() {
			mRegistration = catalog.NewMachineRegistration(
				fleetNamespace,
				"machine-registration",
				&elementalv1.Config{})
		})

		AfterEach(func() {
			Expect(cl.Delete(ctx, mRegistration)).To(Succeed())

			Eventually(func() bool {
				if err := cl.Get(ctx, client.ObjectKeyFromObject(mRegistration), &rbacv1.Role{}); !apierrors.IsNotFound(err) {
					return false
				}
				if err := cl.Get(ctx, client.ObjectKeyFromObject(mRegistration), &corev1.ServiceAccount{}); !apierrors.IsNotFound(err) {
					return false
				}
				if err := cl.Get(ctx, client.ObjectKeyFromObject(mRegistration), &rbacv1.RoleBinding{}); !apierrors.IsNotFound(err) {
					return false
				}
				if err := cl.Get(ctx, client.ObjectKey{
					Namespace: mRegistration.Namespace,
					Name:      mRegistration.Name + "token",
				}, &corev1.Secret{}); !apierrors.IsNotFound(err) {
					return false
				}
				return true
			}, 2*time.Minute, 2*time.Second).Should(BeTrue())
		})

		It("creates a machine registration resource and a URL attaching CA certificate", func() {
			config := map[string]string{
				"content":  "V2h5IGFyZSB5b3UgY2hlY2tpbmcgdGhpcz8K",
				"encoding": "b64",
			}
			cconfig, _ := json.Marshal(config)
			mRegistration = catalog.NewMachineRegistration(
				fleetNamespace,
				"machine-registration",
				&elementalv1.Config{
					Elemental: elementalv1.Elemental{
						Install: elementalv1.Install{
							Device: "/dev/vda",
							ISO:    "https://something.example.com",
						},
					},
					CloudConfig: map[string]runtime.RawExtension{
						"write_files": {
							Raw: cconfig,
						},
					},
				},
			)

			Expect(cl.Create(ctx, mRegistration)).To(Succeed())

			var url string
			Eventually(func() string {
				mReg := &elementalv1.MachineRegistration{}
				if err := cl.Get(ctx, client.ObjectKeyFromObject(mRegistration), mReg); err != nil {
					return err.Error()
				}
				url = mReg.Status.RegistrationURL
				return mReg.Status.RegistrationURL
			}, 1*time.Minute, 2*time.Second).Should(
				ContainSubstring(fmt.Sprintf("%s.%s/elemental/registration", e2eCfg.ExternalIP, e2eCfg.MagicDNS)))

			Eventually(func() string {
				out, err := http.GetInsecure(url)
				if err != nil {
					return err.Error()
				}
				return out
			}, 1*time.Minute, 2*time.Second).Should(
				And(
					ContainSubstring(fmt.Sprintf("%s.%s/elemental/registration", e2eCfg.ExternalIP, e2eCfg.MagicDNS)),
					ContainSubstring("BEGIN CERTIFICATE"),
					ContainSubstring("END CERTIFICATE"),
				),
			)

			Eventually(func() bool {
				if err := cl.Get(ctx, client.ObjectKeyFromObject(mRegistration), &rbacv1.Role{}); err != nil {
					return false
				}
				if err := cl.Get(ctx, client.ObjectKeyFromObject(mRegistration), &corev1.ServiceAccount{}); err != nil {
					return false
				}
				if err := cl.Get(ctx, client.ObjectKeyFromObject(mRegistration), &rbacv1.RoleBinding{}); err != nil {
					return false
				}
				if err := cl.Get(ctx, client.ObjectKey{
					Namespace: mRegistration.Namespace,
					Name:      mRegistration.Name + "-token",
				}, &corev1.Secret{}); err != nil {
					return false
				}
				return true
			}, 2*time.Minute, 2*time.Second).Should(BeTrue())
		})
	})
})
