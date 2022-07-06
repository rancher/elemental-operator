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
	"github.com/onsi/gomega/types"
	kubectl "github.com/rancher-sandbox/ele-testhelpers/kubectl"

	"github.com/rancher/elemental-operator/tests/catalog"
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
		osImage := "update-osversion"
		osVersion := "osversion"
		AfterEach(func() {
			kube := kubectl.New()
			kube.Delete("managedosimage", "-n", "fleet-default", osImage)
			kube.Delete("managedosversion", "-n", "fleet-default", osVersion)
		})

		createsCorrectPlan := func(meta map[string]interface{}, c *catalog.ContainerSpec, m types.GomegaMatcher) {
			ov := catalog.NewManagedOSVersion(
				osVersion, "v1.0", "0.0.0",
				meta,
				c,
			)

			EventuallyWithOffset(1, func() error {
				return k.ApplyYAML("fleet-default", osVersion, ov)
			}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())

			ui := catalog.NewManagedOSImage(
				osImage,
				[]map[string]interface{}{{"clusterName": "dummycluster"}},
				"",
				osVersion,
			)

			EventuallyWithOffset(1, func() error {
				return k.ApplyYAML("fleet-default", osImage, ui)
			}, 1*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())

			EventuallyWithOffset(1, func() string {
				r, err := kubectl.GetData("fleet-default", "bundle", "mos-update-osversion", `jsonpath={.spec.resources[*].content}`)
				if err != nil {
					fmt.Println(err)
				}
				return string(r)
			}, 1*time.Minute, 2*time.Second).Should(
				m,
			)
		}

		It("creates a new fleet bundle with the upgrade plan image", func() {
			By("creating a new ManagedOSImage referencing a ManagedOSVersion")

			createsCorrectPlan(map[string]interface{}{"upgradeImage": "registry.com/repository/image:v1.0", "robin": "batman"}, nil,
				And(
					ContainSubstring(`"version":"v1.0"`),
					ContainSubstring(`"image":"registry.com/repository/image"`),
					ContainSubstring(`"command":["/usr/sbin/suc-upgrade"]`),
					ContainSubstring(`"name":"METADATA_ROBIN","value":"batman"`),
				),
			)
		})

		It("creates a new fleet bundle with the upgrade plan container", func() {
			By("creating a new ManagedOSImage referencing a ManagedOSVersion")

			createsCorrectPlan(map[string]interface{}{"upgradeImage": "registry.com/repository/image:v1.0", "baz": "batman", "jsondata": struct{ Foo string }{Foo: "foostruct"}},
				&catalog.ContainerSpec{Image: "foo/bar:image"},
				And(
					ContainSubstring(`"version":"v1.0"`),
					ContainSubstring(`"image":"foo/bar:image"`),
					ContainSubstring(`"name":"METADATA_BAZ","value":"batman"`),
					ContainSubstring(`{"name":"METADATA_JSONDATA","value":"{\"foo\":\"foostruct\"}"}`),
				),
			)
		})
	})
})
