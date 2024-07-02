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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/tests/e2e/config"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubectl "github.com/rancher-sandbox/ele-testhelpers/kubectl"
)

const stableCRDSChart = "oci://registry.suse.com/rancher/elemental-operator-crds-chart"
const stableChart = "oci://registry.suse.com/rancher/elemental-operator-chart"

var _ = Describe("Elemental Operator downgrade/upgrade test", func() {
	var k *kubectl.Kubectl
	var downgradeCfg config.E2EConfig
	var channelName string
	var managedByLabel map[string]string

	It("downgrades to latest stable version", func() {
		k = kubectl.New()
		downgradeCfg = *e2eCfg
		downgradeCfg.CRDsChart = stableCRDSChart
		downgradeCfg.Chart = stableChart
		channelName = ""
		managedByLabel = map[string]string{
			"app.kubernetes.io/managed-by": "Helm",
		}

		Expect(isOperatorInstalled(k)).To(BeTrue())

		By("Uninstall Elemental Operator first before downgrading CRDs chart", func() {
			Expect(kubectl.RunHelmBinaryWithCustomErr(
				"uninstall", operatorName,
				"--namespace", operatorNamespace,
				"--wait",
			)).To(Succeed())
		})

		By("Install the new Elemental Operator", func() {
			deployOperator(k, &downgradeCfg)
		})

		By("Check it gets a default channel and syncs ManagedOSVersions", func() {
			Eventually(func() string {
				channels := &elementalv1.ManagedOSVersionChannelList{}
				err := cl.List(ctx, channels, client.InNamespace(fleetDefaultNamespace), client.MatchingLabels(managedByLabel))
				if err == nil {
					// After uninstalling Operator and reinstalling it there should be only a single channel managed by Helm
					Expect(len(channels.Items)).To(Equal(1))
					channelName = channels.Items[0].Name
				}
				return channelName
			}, 10*time.Second, 1*time.Second).ShouldNot(BeEmpty())

			Eventually(func() int {
				mOSes := &elementalv1.ManagedOSVersionList{}
				err := cl.List(ctx, mOSes, client.InNamespace(fleetDefaultNamespace), client.MatchingLabels(map[string]string{
					elementalv1.ElementalManagedOSVersionChannelLabel: channelName,
				}))
				if err == nil {
					return len(mOSes.Items)
				}
				return 0
			}, 2*time.Minute, 2*time.Second).Should(BeNumerically(">", 0))
		})

		By("Upgrading Elemental Operator to testing version", func() {
			deployOperator(k, e2eCfg)
		})

		By("Check we still keep the previous default channel managed by Helm", func() {
			Eventually(func() error {
				ch := &elementalv1.ManagedOSVersionChannel{}
				err := cl.Get(ctx, client.ObjectKey{
					Name:      channelName,
					Namespace: fleetDefaultNamespace,
				}, ch)
				return err
			}, 10*time.Second, 1*time.Second).ShouldNot(HaveOccurred())
		})
	})
})
