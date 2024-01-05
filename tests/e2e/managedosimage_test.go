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
	"github.com/onsi/gomega/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/tests/catalog"
	fleetv1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("ManagedOSImage e2e tests", func() {
	Context("Using OSImage reference", func() {
		var ui *elementalv1.ManagedOSImage

		BeforeEach(func() {
			By("Creating a new ManagedOSImage CRD")
			ui = catalog.NewManagedOSImage(
				fleetDefaultNamespace,
				"update-image",
				[]elementalv1.BundleTarget{{
					ClusterName: "dummycluster",
				}},
				"my.registry.com/image/repository:v1.0",
				"",
			)

			Expect(cl.Create(ctx, ui)).To(Succeed())
		})

		AfterEach(func() {
			ui = catalog.NewManagedOSImage(
				fleetDefaultNamespace,
				"update-image",
				[]elementalv1.BundleTarget{{
					ClusterName: "dummycluster",
				}},
				"my.registry.com/image/repository:v1.0",
				"",
			)

			Expect(cl.Delete(ctx, ui)).To(Succeed())
		})

		It("creates a new fleet bundle with the upgrade plan", func() {

			Eventually(func() string {
				bundle := &fleetv1.Bundle{}

				if err := cl.Get(ctx, client.ObjectKey{
					Name:      "mos-update-image",
					Namespace: fleetDefaultNamespace,
				}, bundle); err != nil {
					return err.Error()
				}

				return fmt.Sprint(bundle.Spec.Resources)
			}, 1*time.Minute, 2*time.Second).Should(
				And(
					ContainSubstring(`"version":"v1.0"`),
					ContainSubstring(`"image":"my.registry.com/image/repository"`),
				),
			)
		})
	})
	Context("Using ManagedOSVersion reference", func() {
		withUpgradePlan := "with-upgrade-plan"
		withUpgradeImage := "with-upgrade-image"

		AfterEach(func() {
			Expect(cl.DeleteAllOf(ctx, &elementalv1.ManagedOSImage{}, client.InNamespace(fleetDefaultNamespace)))
			Expect(cl.DeleteAllOf(ctx, &elementalv1.ManagedOSVersion{}, client.InNamespace(fleetDefaultNamespace)))
		})

		createsCorrectPlan := func(name string, meta map[string]runtime.RawExtension, c *upgradev1.ContainerSpec, m types.GomegaMatcher) {
			ov := catalog.NewManagedOSVersion(
				fleetDefaultNamespace,
				name, "v1.0", "0.0.0",
				meta,
				c,
			)

			Expect(cl.Create(ctx, ov)).To(Succeed())

			ui := catalog.NewManagedOSImage(
				fleetDefaultNamespace,
				name,
				[]elementalv1.BundleTarget{{
					ClusterName: "dummycluster",
				}},
				"",
				name,
			)

			Expect(cl.Create(ctx, ui)).To(Succeed())

			Eventually(func() string {
				bundle := &fleetv1.Bundle{}

				if err := cl.Get(ctx, client.ObjectKey{
					Name:      "mos-" + name,
					Namespace: fleetDefaultNamespace,
				}, bundle); err != nil {
					return err.Error()
				}

				return fmt.Sprint(bundle.Spec.Resources)
			}, 1*time.Minute, 2*time.Second).Should(
				m,
			)
		}

		It("creates a new fleet bundle with the upgrade plan image", func() {
			By("creating a new ManagedOSImage referencing a ManagedOSVersion")

			metaConfig := map[string]runtime.RawExtension{}
			registry, _ := json.Marshal("registry.com/repository/image:v1.0")
			metaConfig["upgradeImage"] = runtime.RawExtension{
				Raw: registry,
			}
			batman, _ := json.Marshal("batman")
			metaConfig["robin"] = runtime.RawExtension{
				Raw: batman,
			}

			createsCorrectPlan(withUpgradeImage, metaConfig, nil,
				And(
					ContainSubstring(`"version":"v1.0"`),
					ContainSubstring(`"image":"registry.com/repository/image"`),
					ContainSubstring(`"command":["/usr/sbin/suc-upgrade"]`),
					ContainSubstring(`"name":"METADATA_ROBIN","value":"batman"`),
					ContainSubstring(`"name":"METADATA_UPGRADEIMAGE","value":"registry.com/repository/image:v1.0"`),
				),
			)
		})

		It("creates a new fleet bundle with the upgrade plan container", func() {
			By("creating a new ManagedOSImage referencing a ManagedOSVersion")

			metaConfig := map[string]runtime.RawExtension{}
			registry, _ := json.Marshal("registry.com/repository/image:v1.0")
			metaConfig["upgradeImage"] = runtime.RawExtension{
				Raw: registry,
			}
			batman, _ := json.Marshal("batman")
			metaConfig["baz"] = runtime.RawExtension{
				Raw: batman,
			}
			nested := struct{ Foo string }{Foo: "foostruct"}
			nestedJSON, _ := json.Marshal(nested)
			metaConfig["jsondata"] = runtime.RawExtension{
				Raw: nestedJSON,
			}

			createsCorrectPlan(withUpgradePlan, metaConfig,
				&upgradev1.ContainerSpec{Image: "foo/bar:image"},
				And(
					ContainSubstring(`"version":"v1.0"`),
					ContainSubstring(`"image":"foo/bar:image"`),
					ContainSubstring(`"name":"METADATA_BAZ","value":"batman"`),
					ContainSubstring(`{"name":"METADATA_JSONDATA","value":"{\"Foo\":\"foostruct\"}"}`),
					ContainSubstring(`"name":"METADATA_UPGRADEIMAGE","value":"registry.com/repository/image:v1.0"`),
				),
			)
		})
	})
})
