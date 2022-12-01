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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	kubectl "github.com/rancher-sandbox/ele-testhelpers/kubectl"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"

	"github.com/rancher/elemental-operator/tests/catalog"
)

const discoveryPluginImage = "quay.io/costoolkit/upgradechannel-discovery:v0.4.0"

func getPlan(s string) (up *upgradev1.Plan, err error) {
	up = &upgradev1.Plan{}
	err = kubectl.GetObject(s, cattleSystemNamespace, "plan", up)
	return
}

func waitTestChannelPopulate(k *kubectl.Kubectl, mr *catalog.ManagedOSVersionChannel, name string, image, version string) {
	EventuallyWithOffset(1, func() error {
		return k.ApplyYAML(fleetNamespace, name, mr)
	}, 2*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())

	EventuallyWithOffset(1, func() string {
		r, err := kubectl.GetData(fleetNamespace, "ManagedOSVersion", version, `jsonpath={.spec.metadata.upgradeImage}`)
		if err != nil {
			fmt.Println(err)
		}
		return string(r)
	}, 6*time.Minute, 2*time.Second).Should(
		Equal(image),
	)
}

func upgradePod(k *kubectl.Kubectl) string {
	pods, err := k.GetPodNames(cattleSystemNamespace, "upgrade.cattle.io/controller=system-upgrade-controller")
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	podName := ""
	for _, p := range pods {
		if strings.Contains(p, "apply-os-upgrader-update-osversion") {
			podName = p
		}
	}
	return podName
}

func checkUpgradePod(k *kubectl.Kubectl, env, image, command, args, mm types.GomegaMatcher) {
	// Wait for the upgrade pod to appear
	k.EventuallyPodMatch(
		cattleSystemNamespace,
		"upgrade.cattle.io/controller=system-upgrade-controller",
		3*time.Minute, 2*time.Second,
		ContainElement(ContainSubstring("apply-os-upgrader-update-osversion-on-operator-e2e")),
	)

	podName := upgradePod(k)

	pod := &corev1.Pod{}
	err := kubectl.GetObject(podName, cattleSystemNamespace, "pods", pod)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	container := pod.Spec.Containers[0]

	envs := []string{}
	for _, e := range container.Env {
		envs = append(envs, fmt.Sprintf("%s=%s", e.Name, e.Value))
	}
	mounts := []string{}
	for _, e := range container.VolumeMounts {
		mounts = append(mounts, fmt.Sprintf("%s=%s", e.Name, e.MountPath))
	}
	ExpectWithOffset(1, container.Name).To(Equal("upgrade"))
	ExpectWithOffset(1, envs).To(env)
	ExpectWithOffset(1, container.Image).To(image)
	ExpectWithOffset(1, container.Command).To(command)
	ExpectWithOffset(1, container.Args).To(args)
	ExpectWithOffset(1, mounts).To(mm)
}

var _ = Describe("ManagedOSImage Upgrade e2e tests", func() {
	k := kubectl.New()

	Context("Using ManagedOSVersion reference", func() {
		osImage := "update-osversion"
		osVersion := "osversion"
		AfterEach(func() {
			k.Delete("managedosimage", "-n", fleetNamespace, osImage)
			k.Delete("managedosversion", "-n", fleetNamespace, osVersion)

			// Plans do not successfully apply, so nodes stays in cordoned state.
			// uncordon them so tests will keep running
			nodeName, err := kubectl.Run("get", "nodes", "-o", "jsonpath='{.items[0].metadata.name}'")
			Expect(err).ToNot(HaveOccurred())
			kubectl.Run("uncordon", nodeName)

			k.Delete("jobs", "--all", "--wait", "-n", fleetNamespace)
			k.Delete("plans", "--all", "--wait", "-n", fleetNamespace)
			k.Delete("bundles", "--all", "--wait", "-n", fleetNamespace)
			k.Delete("managedosversionchannel", "--all", "--wait", "-n", fleetNamespace)

			// delete dangling upgrade pods
			EventuallyWithOffset(1, func() []string {
				pods, err := k.GetPodNames(cattleSystemNamespace, "upgrade.cattle.io/controller=system-upgrade-controller")
				if err != nil {
					fmt.Println(err)
				}
				fmt.Println(pods)

				applyPods := []string{}
				for _, p := range pods {
					if !strings.HasPrefix(p, "system-upgrade-controller") {
						applyPods = append(applyPods, p)
					}

					if strings.Contains(p, "apply-os-upgrader") {
						By("deleting " + p)
						k.Delete("pod", "-n", cattleSystemNamespace, "--wait", "--force", p)
						err = k.WaitForPodDelete(cattleSystemNamespace, p)
						Expect(err).ToNot(HaveOccurred())
					}
				}

				return applyPods
			}, 3*time.Minute, 2*time.Second).Should(Equal([]string{}))
		})

		createsCorrectPlan := func(meta map[string]interface{}, c *catalog.ContainerSpec, m types.GomegaMatcher) {
			ov := catalog.LegacyNewManagedOSVersion(
				osVersion, "v1.0", "0.0.0",
				meta,
				c,
			)

			EventuallyWithOffset(1, func() error {
				return k.ApplyYAML(fleetNamespace, osVersion, ov)
			}, 2*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())

			ui := catalog.LegacyNewManagedOSImage(
				osImage,
				[]map[string]interface{}{},
				"",
				osVersion,
			)

			EventuallyWithOffset(1, func() error {
				return k.ApplyYAML(fleetNamespace, osImage, ui)
			}, 2*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())

			EventuallyWithOffset(1, func() string {
				r, err := kubectl.GetData(fleetNamespace, "bundle", "mos-update-osversion", `jsonpath={.spec.resources[*].content}`)
				if err != nil {
					fmt.Println(err)
				}
				return string(r)
			}, 1*time.Minute, 2*time.Second).Should(
				m,
			)
		}

		It("tries to apply a plan", func() {
			By("creating a new ManagedOSImage referencing a ManagedOSVersion")

			createsCorrectPlan(map[string]interface{}{"upgradeImage": "registry.com/repository/image:v1.0", "robin": "batman"}, nil,
				And(
					ContainSubstring(`"version":"v1.0"`),
					ContainSubstring(`"image":"registry.com/repository/image"`),
					ContainSubstring(`"command":["/usr/sbin/suc-upgrade"]`),
					ContainSubstring(`"name":"METADATA_ROBIN","value":"batman"`),
				),
			)

			Eventually(func() string {
				up, err := getPlan("os-upgrader-update-osversion")
				if err == nil {
					return up.Spec.Version
				}
				return ""
			}, 1*time.Minute, 2*time.Second).Should(Equal("v1.0"))

			plan, err := getPlan("os-upgrader-update-osversion")
			Expect(err).ToNot(HaveOccurred())
			Expect(plan.Spec.Upgrade.Image).To(Equal("registry.com/repository/image"))

			checkUpgradePod(k,
				And(
					ContainElement("METADATA_ROBIN=batman"),
					ContainElement("METADATA_UPGRADEIMAGE=registry.com/repository/image:v1.0"),
				),
				Equal("registry.com/repository/image:v1.0"),
				Equal([]string{"/usr/sbin/suc-upgrade"}),
				BeNil(),
				And(
					ContainElement("host-root=/host"),
				),
			)
		})

		It("applies an os-upgrade from a channel", func() {
			mr := catalog.LegacyNewManagedOSVersionChannel(
				"testchannel3",
				"custom",
				"1m",
				map[string]interface{}{
					"image": discoveryPluginImage,
					"envs": []map[string]string{
						{
							"name":  "REPOSITORY",
							"value": "https://github.com/rancher-sandbox/upgradechannel-discovery-test-repo",
						},
					},
					"args": []string{"git"},
				},
				nil,
			)
			defer k.Delete("managedosversionchannel", "-n", fleetNamespace, "testchannel3")

			waitTestChannelPopulate(k, mr, "testchannel3", "foo/bar:v0.1.0-beta1", "v0.1.0-beta1")

			err := k.ApplyYAML(fleetNamespace, osImage, catalog.LegacyNewManagedOSImage(
				osImage,
				[]map[string]interface{}{},
				"",
				"v0.1.0-beta1",
			))
			Expect(err).ToNot(HaveOccurred())

			checkUpgradePod(k,
				And(
					ContainElement("METADATA_UPGRADEIMAGE=foo/bar:v0.1.0-beta1"),
				),
				Equal("foo/bar:v0.1.0-beta1"),
				Equal([]string{"/usr/sbin/suc-upgrade"}),
				BeNil(),
				And(
					ContainElement("host-root=/host"),
				),
			)
		})

		It("overwrites default container spec from a channel", func() {
			mr := catalog.LegacyNewManagedOSVersionChannel(
				"testchannel4",
				"custom",
				"1m",
				map[string]interface{}{
					"image": discoveryPluginImage,
					"envs": []map[string]string{
						{
							"name":  "REPOSITORY",
							"value": "https://github.com/rancher-sandbox/upgradechannel-discovery-test-repo",
						},
					},
					"args": []string{"git"},
				},
				&catalog.ContainerSpec{
					Image:   "Foobarz",
					Command: []string{"foo"},
					Args:    []string{"baz"},
				},
			)
			defer k.Delete("managedosversionchannel", "-n", fleetNamespace, "testchannel4")

			waitTestChannelPopulate(k, mr, "testchannel4", "foo/bar:v0.1.0-beta1", "v0.1.0-beta1")

			err := k.ApplyYAML(fleetNamespace, osImage, catalog.LegacyNewManagedOSImage(
				osImage,
				[]map[string]interface{}{},
				"",
				"v0.1.0-beta1",
			))
			Expect(err).ToNot(HaveOccurred())

			checkUpgradePod(k,
				And(
					ContainElement("METADATA_UPGRADEIMAGE=foo/bar:v0.1.0-beta1"),
				),
				Equal("Foobarz"),
				Equal([]string{"foo"}),
				Equal([]string{"baz"}),
				And(
					ContainElement("host-root=/host"),
				),
			)
		})

		It("tries to apply an upgrade", func() {
			mr := catalog.LegacyNewManagedOSVersionChannel(
				"testchannel5",
				"custom",
				"1m",
				map[string]interface{}{
					"image": discoveryPluginImage,
					"envs": []map[string]string{
						{
							"name":  "REPOSITORY",
							"value": "rancher/elemental",
						},
						{
							"name":  "IMAGE_PREFIX",
							"value": "quay.io/costoolkit/elemental",
						},
						{
							"name":  "VERSION_SUFFIX",
							"value": "-amd64",
						},
						{
							"name":  "PRE_RELEASES",
							"value": "true",
						},
					},
					"args": []string{"github"},
				},
				nil,
			)
			defer k.Delete("managedosversionchannel", "-n", fleetNamespace, "testchannel5")

			waitTestChannelPopulate(k, mr, "testchannel5", "quay.io/costoolkit/elemental:v0.1.0-amd64", "v0.1.0-amd64")

			err := k.ApplyYAML(fleetNamespace, osImage,
				catalog.LegacyNewManagedOSImage(osImage, []map[string]interface{}{}, "", "v0.1.0-amd64"))
			Expect(err).ToNot(HaveOccurred())

			checkUpgradePod(k,
				And(
					ContainElement("METADATA_UPGRADEIMAGE=quay.io/costoolkit/elemental:v0.1.0-amd64"),
				),
				Equal("quay.io/costoolkit/elemental:v0.1.0-amd64"),
				Equal([]string{"/usr/sbin/suc-upgrade"}),
				BeNil(),
				And(
					ContainElement("host-root=/host"),
				),
			)

			Eventually(func() string {
				podName := upgradePod(k)
				str, _ := kubectl.Run("logs", "-n", cattleSystemNamespace, podName)
				fmt.Println(str)
				return str
			}, 5*time.Minute, 2*time.Second).Should(
				And(
					ContainSubstring("Starting elemental version"),
					ContainSubstring("Could not find device for COS_RECOVERY label: no device found"),
				))
		})
	})
})
