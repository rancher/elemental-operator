package e2e_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/rancheros-operator/tests/catalog"
	"github.com/rancher-sandbox/rancheros-operator/tests/sut"
)

var (
	chart      string
	externalIP string
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "os2 e2e test Suite")
}

var _ = BeforeSuite(func() {

	kubectl := sut.NewKubectl()

	chart = os.Getenv("ROS_CHART")
	if chart == "" {
		Fail("No chart provided, skipping tests")
	}
	externalIP = os.Getenv("EXTERNAL_IP")
	if externalIP == "" {
		Fail("No EXTERNAL_IP provided, skipping tests")
	}

	By("Deploying ros-operator chart dependencies", func() {
		err := sut.RunHelmBinaryWithCustomErr("-n", "cert-manager", "install", "--set", "installCRDs=true", "--create-namespace", "cert-manager", "https://charts.jetstack.io/charts/cert-manager-v1.5.3.tgz")
		Expect(err).ToNot(HaveOccurred())

		err = kubectl.WaitForPod("cert-manager", "app.kubernetes.io/instance=cert-manager", "cert-manager-cainjector")
		Expect(err).ToNot(HaveOccurred())

		err = sut.RunHelmBinaryWithCustomErr("-n", "cattle-management", "install", "--set", "bootstrapPassword=admin", "--set", "replicas=1", "--set", fmt.Sprintf("hostname=%s.xip.io", externalIP), "--create-namespace", "rancher", "https://releases.rancher.com/server-charts/stable/rancher-2.6.3.tgz")
		Expect(err).ToNot(HaveOccurred())

		err = kubectl.WaitForPod("cattle-management", "app=rancher", "rancher")
		Expect(err).ToNot(HaveOccurred())

		pods, err := kubectl.GetPodNames("cattle-management", "app=rancher")
		Expect(err).ToNot(HaveOccurred())
		Expect(len(pods)).To(Equal(1))

		Eventually(func() bool {
			e, _ := kubectl.PodStatus("cattle-management", pods[0])
			return e.ContainerStatuses[0].Ready
		}, 15*time.Minute, 2*time.Second).Should(BeTrue())

	})

	By("Deploying ros-operator chart", func() {
		err := sut.RunHelmBinaryWithCustomErr(
			"-n", "cattle-rancheros-operator-system", "install", "--create-namespace", "rancheros-operator", chart)
		Expect(err).ToNot(HaveOccurred())

		err = kubectl.WaitForPod("cattle-rancheros-operator-system", "app=rancheros-operator", "rancheros-operator")
		Expect(err).ToNot(HaveOccurred())

		pods, err := kubectl.GetPodNames("cattle-rancheros-operator-system", "app=rancheros-operator")
		Expect(err).ToNot(HaveOccurred())
		Expect(len(pods)).To(Equal(1))

		Eventually(func() bool {
			e, _ := kubectl.PodStatus("cattle-rancheros-operator-system", pods[0])
			if len(e.ContainerStatuses) == 0 {
				return false
			}
			return e.ContainerStatuses[0].Ready
		}, 15*time.Minute, 2*time.Second).Should(BeTrue())

		err = kubectl.ApplyYAML("", "server-url", catalog.NewSetting("server-url", "env", fmt.Sprintf("%s.nip.io", externalIP)))
		Expect(err).ToNot(HaveOccurred())
	})
})
