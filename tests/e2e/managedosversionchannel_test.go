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

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/tests/catalog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	http "github.com/rancher-sandbox/ele-testhelpers/http"
)

const channelName = "testchannel"

var _ = Describe("ManagedOSVersionChannel e2e tests", Ordered, func() {
	var versions []elementalv1.ManagedOSVersion

	AfterEach(func() {
		Expect(cl.DeleteAllOf(
			ctx, &elementalv1.ManagedOSVersionChannel{}, client.InNamespace(fleetNamespace)),
		).To(Succeed())
		Eventually(func() int {
			lst := &elementalv1.ManagedOSVersionChannelList{}
			cl.List(ctx, lst, client.InNamespace(fleetNamespace))
			return len(lst.Items)
		}, 1*time.Minute, 2*time.Second).Should(
			Equal(0),
		)
	})

	Context("Create ManagedOSVersions", func() {
		BeforeEach(func() {
			versions = []elementalv1.ManagedOSVersion{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "v1"},
					Spec: elementalv1.ManagedOSVersionSpec{
						Version:    "v1",
						Type:       "container",
						MinVersion: "0.0.0",
						Metadata: map[string]runtime.RawExtension{
							"upgradeImage": {Raw: []byte(`"registry.com/repository/image:v1"`)},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "v2"},
					Spec: elementalv1.ManagedOSVersionSpec{
						Version:    "v2",
						Type:       "container",
						MinVersion: "0.0.0",
						Metadata: map[string]runtime.RawExtension{
							"upgradeImage": {Raw: []byte(`"registry.com/repository/image:v2"`)},
						},
					},
				},
			}
		})

		It("Reports failure events", func() {
			By("Create an invalid ManagedOSVersionChannel")
			ch := catalog.NewManagedOSVersionChannel(
				fleetNamespace, channelName, "", "10m",
				map[string]runtime.RawExtension{
					"uri": {Raw: []byte(`"http://` + e2eCfg.BridgeIP + `:9999"`)},
				}, nil,
			)

			// Not type is set
			ch.Spec.Options = map[string]runtime.RawExtension{
				"uri": {Raw: []byte(`"http://` + e2eCfg.BridgeIP + `:9999"`)},
			}
			ch.Spec.SyncInterval = "10m"

			Expect(cl.Create(ctx, ch)).To(Succeed())

			By("Check that reports event failure")
			Eventually(func() string {
				gCh := &elementalv1.ManagedOSVersionChannel{}
				_ = cl.Get(ctx, client.ObjectKey{
					Name:      channelName,
					Namespace: fleetNamespace,
				}, gCh)

				if len(gCh.Status.Conditions) > 0 {
					return gCh.Status.Conditions[0].Message
				}
				return ""
			}, 1*time.Minute, 2*time.Second).Should(
				ContainSubstring("spec.Type can't be empty"),
			)
		})

		It("creates a list of ManagedOSVersion from a JSON server", func() {
			newCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			b, err := json.Marshal(versions)
			Expect(err).ShouldNot(HaveOccurred())

			http.Server(newCtx, e2eCfg.BridgeIP+":9999", string(b))

			By("Create a ManagedOSVersionChannel")
			ch := catalog.NewManagedOSVersionChannel(
				fleetNamespace, channelName, "json", "10m",
				map[string]runtime.RawExtension{
					"uri": {Raw: []byte(`"http://` + e2eCfg.BridgeIP + `:9999"`)},
				}, nil,
			)

			Expect(cl.Create(ctx, ch)).To(Succeed())

			gCh := &elementalv1.ManagedOSVersionChannel{}
			Expect(cl.Get(ctx, client.ObjectKey{
				Name:      channelName,
				Namespace: fleetNamespace,
			}, gCh)).To(Succeed())
			Expect(gCh.Spec.Type).To(Equal("json"))

			By("Check new ManagedOSVersions are created")
			Eventually(func() string {
				v := &elementalv1.ManagedOSVersion{}
				cl.Get(ctx, client.ObjectKey{
					Name:      "v1",
					Namespace: fleetNamespace,
				}, v)
				if v.Spec.Metadata["upgradeImage"].Raw != nil {
					return string(v.Spec.Metadata["upgradeImage"].Raw)
				}
				return ""
			}, 5*time.Minute, 2*time.Second).Should(
				Equal(`"registry.com/repository/image:v1"`),
			)

			Eventually(func() string {
				v := &elementalv1.ManagedOSVersion{}
				cl.Get(ctx, client.ObjectKey{
					Name:      "v2",
					Namespace: fleetNamespace,
				}, v)
				if v.Spec.Metadata["upgradeImage"].Raw != nil {
					return string(v.Spec.Metadata["upgradeImage"].Raw)
				}
				return ""
			}, 1*time.Minute, 2*time.Second).Should(
				Equal(`"registry.com/repository/image:v2"`),
			)

			Expect(cl.Delete(ctx, gCh)).To(Succeed())

			By("Check ManagedOSVersions are deleted on channel clean up")

			Eventually(func() int {
				lst := &elementalv1.ManagedOSVersionList{}
				cl.List(ctx, lst, client.InNamespace(fleetNamespace))
				return len(lst.Items)
			}, 1*time.Minute, 2*time.Second).Should(
				Equal(0),
			)
		})

		It("creates a list of ManagedOSVersion from a custom hook", func() {

			jVersions, _ := json.Marshal(versions)
			command, _ := json.Marshal([]string{"/bin/bash", "-c", "--"})
			args, _ := json.Marshal([]string{fmt.Sprintf("echo '%s' > /output/data", string(jVersions))})

			By("Create a ManagedOSVersionChannel")
			ch := catalog.NewManagedOSVersionChannel(
				fleetNamespace, channelName, "custom", "10m",
				map[string]runtime.RawExtension{
					"image":      {Raw: []byte(`"opensuse/tumbleweed"`)},
					"command":    {Raw: command},
					"mountPath":  {Raw: []byte(`"/output"`)},
					"outputFile": {Raw: []byte(`"/output/data"`)},
					"args":       {Raw: args},
				}, nil,
			)

			Expect(cl.Create(ctx, ch)).To(Succeed())

			gCh := &elementalv1.ManagedOSVersionChannel{}
			Expect(cl.Get(ctx, client.ObjectKey{
				Name:      channelName,
				Namespace: fleetNamespace,
			}, gCh)).To(Succeed())
			Expect(gCh.Spec.Type).To(Equal("custom"))

			By("Check new ManagedOSVersions are created")
			Eventually(func() string {
				v := &elementalv1.ManagedOSVersion{}
				cl.Get(ctx, client.ObjectKey{
					Name:      "v1",
					Namespace: fleetNamespace,
				}, v)
				if v.Spec.Metadata["upgradeImage"].Raw != nil {
					return string(v.Spec.Metadata["upgradeImage"].Raw)
				}
				return ""
			}, 4*time.Minute, 2*time.Second).Should(
				Equal(`"registry.com/repository/image:v1"`),
			)

			Eventually(func() string {
				v := &elementalv1.ManagedOSVersion{}
				cl.Get(ctx, client.ObjectKey{
					Name:      "v2",
					Namespace: fleetNamespace,
				}, v)
				if v.Spec.Metadata["upgradeImage"].Raw != nil {
					return string(v.Spec.Metadata["upgradeImage"].Raw)
				}
				return ""
			}, 10*time.Second, 2*time.Second).Should(
				Equal(`"registry.com/repository/image:v2"`),
			)
		})
	})
})
