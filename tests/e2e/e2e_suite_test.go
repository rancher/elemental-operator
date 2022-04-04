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
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubectl "github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/rancheros-operator/tests/catalog"
)

var (
	chart      string
	externalIP string
	magicDNS   string
)

var testResources = []string{"machineregistration"}

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ros-operator e2e test Suite")
}

func waitNamespacePodsDelete(namespace string) {
	k := kubectl.New()

	Eventually(func() bool {
		pods, err := k.GetPodNames(namespace, "")
		Expect(err).ToNot(HaveOccurred())

		if len(pods) > 0 {
			return false
		}
		return true
	}, 15*time.Minute, 2*time.Second).Should(BeTrue())
}

func waitNamespaceDeletion(namespace string) {
	Eventually(func() bool {
		phase, _ := kubectl.GetData(namespace, "namespace", namespace, `jsonpath={.status.phase}`)
		if string(phase) == "" {
			return true
		}

		return false
	}, 15*time.Minute, 2*time.Second).Should(BeTrue())
}
func waitNamespace(namespace, label string) {
	k := kubectl.New()

	Eventually(func() bool {
		pods, err := k.GetPodNames(namespace, label)
		Expect(err).ToNot(HaveOccurred())

		for _, p := range pods {
			e, _ := k.PodStatus(namespace, p)
			if e == nil || e.ContainerStatuses == nil || len(e.ContainerStatuses) == 0 {
				return false
			}
			if !e.ContainerStatuses[0].Ready {
				return false
			}
		}
		return true
	}, 15*time.Minute, 2*time.Second).Should(BeTrue())
}

func isOperatorInstalled(k *kubectl.Kubectl) bool {
	pods, err := k.GetPodNames("cattle-rancheros-operator-system", "")
	Expect(err).ToNot(HaveOccurred())
	if len(pods) > 0 {
		return true
	}
	return false
}

func deployOperator(k *kubectl.Kubectl) {
	By("Deploying ros-operator chart", func() {
		err := kubectl.RunHelmBinaryWithCustomErr(
			"-n", "cattle-rancheros-operator-system", "install", "--create-namespace", "rancheros-operator", chart)
		Expect(err).ToNot(HaveOccurred())

		err = k.WaitForPod("cattle-rancheros-operator-system", "app=rancheros-operator", "rancheros-operator")
		Expect(err).ToNot(HaveOccurred())

		waitNamespace("cattle-rancheros-operator-system", "app=rancheros-operator")

		err = k.ApplyYAML("", "server-url", catalog.NewSetting("server-url", "env", fmt.Sprintf("%s.%s", externalIP, magicDNS)))
		Expect(err).ToNot(HaveOccurred())
	})
}

var _ = BeforeSuite(func() {

	k := kubectl.New()

	magicDNS = os.Getenv("MAGIC_DNS")
	if magicDNS == "" {
		magicDNS = "sslip.io"
	}

	rancherVersion := os.Getenv("RANCHER_VERSION")
	if rancherVersion == "" {
		rancherVersion = "2.6.4"
	}

	externalIP = os.Getenv("EXTERNAL_IP")
	if externalIP == "" {
		Fail("No EXTERNAL_IP provided, a known (reachable) node external ip it is required to run e2e tests")
	}

	chart = os.Getenv("ROS_CHART")
	if chart == "" && !isOperatorInstalled(k) {
		Fail("No ROS_CHART provided, a ros operator helm chart is required to run e2e tests")
	} else if isOperatorInstalled(k) {
		//
		// Upgrade/delete of operator only goes here
		// (no further bootstrap is required)
		By("Upgrading the operator only", func() {
			kubectl.DeleteNamespace("cattle-rancheros-operator-system")
			waitNamespaceDeletion("cattle-rancheros-operator-system")
			deployOperator(k)
			// Somehow rancher needs to be restarted after a ros-operator upgrade
			// to get machineregistration working
			pods, _ := k.GetPodNames("cattle-system", "")
			for _, p := range pods {
				k.Delete("pod", "-n", "cattle-system", p)
			}

			waitNamespace("cattle-system", "app=rancher")
		})
		return
	}

	if os.Getenv("NO_SETUP") != "" {
		By("No setup")
		return
	}

	if isOperatorInstalled(k) {
		By("rancher-os already deployed, skipping setup")
		return
	}

	By("Deploying ros-operator chart dependencies", func() {
		By("installing nginx", func() {
			kubectl.CreateNamespace("ingress-nginx")
			err := kubectl.Apply("ingress-nginx", "https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/kind/deploy.yaml")
			Expect(err).ToNot(HaveOccurred())

			waitNamespace("ingress-nginx", "app.kubernetes.io/component=controller")
		})

		By("installing cert-manager", func() {
			err := kubectl.RunHelmBinaryWithCustomErr("-n", "cert-manager", "install", "--set", "installCRDs=true", "--create-namespace", "cert-manager", "https://charts.jetstack.io/charts/cert-manager-v1.5.3.tgz")
			Expect(err).ToNot(HaveOccurred())
			err = k.WaitForPod("cert-manager", "app.kubernetes.io/instance=cert-manager", "cert-manager-cainjector")
			Expect(err).ToNot(HaveOccurred())

			waitNamespace("cert-manager", "app.kubernetes.io/instance=cert-manager")
		})

		By("installing rancher", func() {
			err := kubectl.RunHelmBinaryWithCustomErr(
				"-n",
				"cattle-system",
				"install",
				"--set",
				"bootstrapPassword=admin",
				"--set",
				"replicas=1",
				"--set", fmt.Sprintf("hostname=%s.%s", externalIP, magicDNS),
				"--create-namespace",
				"rancher",
				fmt.Sprintf("https://releases.rancher.com/server-charts/stable/rancher-%s.tgz", rancherVersion),
			)
			Expect(err).ToNot(HaveOccurred())

			err = k.WaitForPod("cattle-system", "app=rancher", "rancher")
			Expect(err).ToNot(HaveOccurred())

			waitNamespace("cattle-system", "app=rancher")
			waitNamespace("cattle-fleet-local-system", "app=fleet-agent")
		})
	})

	deployOperator(k)
})

var _ = AfterSuite(func() {
	// Note, this prevents concurrent tests on same cluster, but makes sure we don't leave any dangling resources from the e2e tests
	for _, r := range testResources {
		kubectl.New().Delete(r, "--all", "--all-namespaces")
	}
})
