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

var _ = Describe("ManagedOSVersionChannel e2e tests", func() {
	k := kubectl.New()
	Context("Create ManagedOSVersions from JSON", func() {

		BeforeEach(func() {
			By("Create a ManagedOSVersionChannel")
			ui := catalog.NewManagedOSVersionChannel(
				"testchannel",
				"json",
				map[string]interface{}{},
			)

			Eventually(func() error {
				return k.ApplyYAML("fleet-default", "testchannel", ui)
			}, 2*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			kubectl.New().Delete("managedosversionchannel", "-n", "fleet-default", "testchannel")
		})

		It("Creates a list of ManagedOSVersion", func() {
			Eventually(func() string {
				r, err := kubectl.GetData("fleet-default", "ManagedOSVersion", "--all", `jsonpath={.item[*]}`)
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
})
