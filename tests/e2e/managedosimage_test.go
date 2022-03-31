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

package e2e_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubectl "github.com/rancher-sandbox/ele-testhelpers/kubectl"

	"github.com/rancher-sandbox/rancheros-operator/tests/catalog"
)

var _ = Describe("ManagedOSImage e2e tests", func() {
	k := kubectl.New()
	Context("Using OSImage reference", func() {

		BeforeEach(func() {
			By("Creating a new ManagedOSImage CRD")
			ui := catalog.NewManagedOSImage(
				"update-image",
				[]map[string]interface{}{{"clusterName": "dummycluster"}},
				"my.registry.com/image/repository:v1.0",
				"",
			)

			Eventually(func() error {
				return k.ApplyYAML("fleet-default", "update-image", ui)
			}, 2*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			kubectl.New().Delete("managedosimage", "-n", "fleet-default", "update-image")
		})

		It("creates a new fleet bundle with the upgrade plan", func() {
			Eventually(func() string {
				r, err := kubectl.GetData("fleet-default", "bundle", "mos-update-image", `jsonpath={.spec.resources[*].content}`)
				if err != nil {
					fmt.Println(err)
				}
				return string(r)
			}, 1*time.Minute, 2*time.Second).Should(
				And(
					ContainSubstring(`"version":"v1.0"`),
					ContainSubstring(`"image":"my.registry.com/image/repository"`),
				),
			)
		})
	})
	Context("Using ManagedOSVersion reference", func() {

		BeforeEach(func() {
			By("creating a new ManagedOSImage referencing a ManagedOSVersion")
			ov := catalog.NewManagedOSVersion(
				"osversion", "v1.0", "0.0.0",
				map[string]interface{}{"upgrade_image": "registry.com/repository/image:v1.0"},
				catalog.ContainerSpec{},
			)

			Eventually(func() error {
				return k.ApplyYAML("fleet-default", "osversion", ov)
			}, 2*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())

			ui := catalog.NewManagedOSImage(
				"update-osversion",
				[]map[string]interface{}{{"clusterName": "dummycluster"}},
				"",
				"osversion",
			)

			Eventually(func() error {
				return k.ApplyYAML("fleet-default", "update-osversion", ui)
			}, 2*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			kube := kubectl.New()
			kube.Delete("managedosimage", "-n", "fleet-default", "update-osversion")
			kube.Delete("managedosversion", "-n", "fleet-default", "osversion")
		})

		It("creates a new fleet bundle with the upgrade plan", func() {
			Eventually(func() string {
				r, err := kubectl.GetData("fleet-default", "bundle", "mos-update-osversion", `jsonpath={.spec.resources[*].content}`)
				if err != nil {
					fmt.Println(err)
				}
				return string(r)
			}, 1*time.Minute, 2*time.Second).Should(
				And(
					ContainSubstring(`"version":"v1.0"`),
					ContainSubstring(`"image":"registry.com/repository/image"`),
				),
			)
		})
	})
})
