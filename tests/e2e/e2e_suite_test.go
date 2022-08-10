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
	"bytes"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubectl "github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher/elemental-operator/tests/catalog"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var (
	chart      string
	externalIP string
	magicDNS   string
	bridgeIP   string
)

const operatorNamespace = "cattle-elemental-system"

var testResources = []string{"machineregistration", "managedosversionchannel"}

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "elemental-operator e2e test Suite")
}

func isOperatorInstalled(k *kubectl.Kubectl) bool {
	pods, err := k.GetPodNames(operatorNamespace, "")
	Expect(err).ToNot(HaveOccurred())
	return len(pods) > 0
}

func deployOperator(k *kubectl.Kubectl) {
	By("Deploying elemental-operator chart", func() {
		err := kubectl.RunHelmBinaryWithCustomErr(
			"-n",
			operatorNamespace,
			"install",
			"--create-namespace",
			"--set", "sync_interval=30s",
			"--set", "debug=true",
			"elemental-operator",
			chart)
		Expect(err).ToNot(HaveOccurred())

		err = k.WaitForPod(operatorNamespace, "app=elemental-operator", "elemental-operator")
		Expect(err).ToNot(HaveOccurred())

		pods, err := k.GetPodNames(operatorNamespace, "app=elemental-operator")
		Expect(err).ToNot(HaveOccurred())
		Expect(len(pods)).To(Equal(1))

		err = k.WaitForNamespaceWithPod(operatorNamespace, "app=elemental-operator")
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() string {
			str, _ := kubectl.Run("logs", "-n", operatorNamespace, pods[0])
			return str
		}, 5*time.Minute, 2*time.Second).Should(
			And(
				ContainSubstring("Starting management.cattle.io/v3, Kind=Setting controller"),
			))

		// As we are not bootstrapping rancher in the tests (going to the first login page, setting new password and rancher-url)
		// We need to manually set this value, which is the same value you would get from doing the bootstrap
		err = k.ApplyYAML("", "server-url", catalog.NewSetting("server-url", "env", fmt.Sprintf("https://%s.%s", externalIP, magicDNS)))
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
		rancherVersion = "2.6.6"
	}

	externalIP = os.Getenv("EXTERNAL_IP")
	if externalIP == "" {
		Fail("No EXTERNAL_IP provided, a known (reachable) node external ip it is required to run e2e tests")
	}

	bridgeIP = os.Getenv("BRIDGE_IP")
	if bridgeIP == "" {
		bridgeIP = "172.17.0.1"
	}

	if os.Getenv("NO_SETUP") != "" {
		By("No setup")
		return
	}

	chart = os.Getenv("CHART")
	if chart == "" && !isOperatorInstalled(k) {
		Fail("No CHART provided, a elemental operator helm chart is required to run e2e tests")
	} else if isOperatorInstalled(k) {
		//
		// Upgrade/delete of operator only goes here
		// (no further bootstrap is required)
		By("Upgrading the operator only", func() {
			err := kubectl.DeleteNamespace(operatorNamespace)
			Expect(err).ToNot(HaveOccurred())

			err = k.WaitForNamespaceDelete(operatorNamespace)
			Expect(err).ToNot(HaveOccurred())

			deployOperator(k)

			// Somehow rancher needs to be restarted after a elemental-operator upgrade
			// to get machineregistration working
			pods, err := k.GetPodNames("cattle-system", "")
			Expect(err).ToNot(HaveOccurred())
			for _, p := range pods {
				err = k.Delete("pod", "-n", "cattle-system", p)
				Expect(err).ToNot(HaveOccurred())
			}

			err = k.WaitForNamespaceWithPod("cattle-system", "app=rancher")
			Expect(err).ToNot(HaveOccurred())
		})
		return
	}

	if isOperatorInstalled(k) {
		By("rancher-os already deployed, skipping setup")
		return
	}

	installed := func(n string) bool {
		pods, err := k.GetPodNames(n, "")
		if err == nil && len(pods) > 0 {
			return true
		}
		return false
	}
	By("Deploying elemental-operator chart dependencies", func() {
		By("installing nginx", func() {
			if installed("ingress-nginx") {
				By("already installed")
				return
			}
			kubectl.CreateNamespace("ingress-nginx")
			err := kubectl.Apply("ingress-nginx", "https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/kind/deploy.yaml")
			Expect(err).ToNot(HaveOccurred())

			err = k.WaitForNamespaceWithPod("ingress-nginx", "app.kubernetes.io/component=controller")
			Expect(err).ToNot(HaveOccurred())
		})

		By("installing cert-manager", func() {
			if installed("cert-manager") {
				By("already installed")
				return
			}

			err := kubectl.RunHelmBinaryWithCustomErr("-n", "cert-manager", "install", "--set", "installCRDs=true", "--create-namespace", "cert-manager", "https://charts.jetstack.io/charts/cert-manager-v1.5.3.tgz")
			Expect(err).ToNot(HaveOccurred())

			err = k.WaitForPod("cert-manager", "app.kubernetes.io/instance=cert-manager", "cert-manager-cainjector")
			Expect(err).ToNot(HaveOccurred())

			err = k.WaitForNamespaceWithPod("cert-manager", "app.kubernetes.io/instance=cert-manager")
			Expect(err).ToNot(HaveOccurred())
		})

		By("installing rancher", func() {
			if installed("cattle-system") {
				By("already installed")
				return
			}

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

			err = k.WaitForNamespaceWithPod("cattle-system", "app=rancher")
			Expect(err).ToNot(HaveOccurred())

			err = k.WaitForNamespaceWithPod("cattle-fleet-local-system", "app=fleet-agent")
			Expect(err).ToNot(HaveOccurred())
		})

		By("installing system-upgrade-controller", func() {

			resp, err := http.Get("https://github.com/rancher/system-upgrade-controller/releases/download/v0.9.1/system-upgrade-controller.yaml")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			data := bytes.NewBuffer([]byte{})

			_, err = io.Copy(data, resp.Body)
			Expect(err).ToNot(HaveOccurred())

			// It needs to look over cattle-system ns to be functional
			toApply := strings.ReplaceAll(data.String(), "namespace: system-upgrade", "namespace: cattle-system")

			temp, err := ioutil.TempFile("", "temp")
			Expect(err).ToNot(HaveOccurred())

			defer os.RemoveAll(temp.Name())
			err = ioutil.WriteFile(temp.Name(), []byte(toApply), os.ModePerm)
			Expect(err).ToNot(HaveOccurred())

			err = kubectl.Apply("cattle-system", temp.Name())
			Expect(err).ToNot(HaveOccurred())
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
