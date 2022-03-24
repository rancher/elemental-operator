package smoke_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/rancheros-operator/tests/sut"
)

var _ = Describe("os2 Smoke tests", func() {
	var s *sut.SUT
	BeforeEach(func() {
		s = sut.NewSUT()
		s.EventuallyConnects()
		s.HasRunningPodByAppLabel("kube-system", "cainjector")
		s.HasRunningPodByAppLabel("cattle-system", "rancher")
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			s.Command("k3s kubectl get pods -A -o json > /tmp/pods.json")
			s.Command("k3s kubectl get events -A -o json > /tmp/events.json")
			s.Command("k3s kubectl get helmcharts -A -o json > /tmp/helm.json")
			s.Command("k3s kubectl get ingress -A -o json > /tmp/ingress.json")
			s.Command("df -h > /tmp/disk")
			s.Command("mount > /tmp/mounts")
			s.Command("blkid > /tmp/blkid")

			s.GatherAllLogs(
				[]string{
					"cos-setup-boot",
					"cos-setup-network",
					"rancherd",
					"k3s",
				},
				[]string{
					"/tmp/pods.json",
					"/tmp/disk",
					"/tmp/mounts",
					"/tmp/blkid",
					"/tmp/events.json",
					"/tmp/helm.json",
					"/tmp/ingress.json",
				})
		}
	})

	Context("ros-operator", func() {
		It("installs and create a machine registration resource", func() {
			chart := os.Getenv("ROS_CHART")
			if chart == "" {
				Skip("No chart provided, skipping tests")
			}
			err := s.SendFile(chart, "/tmp/ros.tgz", "0770")
			Expect(err).ToNot(HaveOccurred())

			err = s.SendFile("../assets/machineregistration.yaml", "/tmp/machine.yaml", "0770")
			Expect(err).ToNot(HaveOccurred())

			By("installing the ros-operator chart", func() {

				Eventually(func() string {
					out, _ := s.Command("sudo KUBECONFIG=/etc/rancher/k3s/k3s.yaml helm -n cattle-rancheros-operator-system install --create-namespace rancheros-operator /tmp/ros.tgz")
					return out
				}, 15*time.Minute, 2*time.Second).Should(ContainSubstring("STATUS: deployed"))

				Eventually(func() bool {
					return s.HasRunningPodByAppLabel("cattle-rancheros-operator-system", "rancheros-operator")
				}).Should(BeTrue())
			})

			By("adding a machine registration", func() {
				Eventually(func() string {
					out, _ := s.Command("sudo k3s kubectl apply -f /tmp/machine.yaml")
					return out
				}, 30*time.Minute, 1*time.Second).Should(
					Or(
						ContainSubstring("unchanged"),
						ContainSubstring("configured"),
					),
				)

				var url string
				Eventually(func() string {
					out, _ := s.Command("sudo k3s kubectl get machineregistration -n fleet-default machine-registration -o jsonpath='{.status.registrationURL}'")
					url = out
					return out
				}, 15*time.Minute, 1*time.Second).Should(ContainSubstring("127.0.0.1.nip.io/v1-rancheros/registration"))

				Eventually(func() string {
					out, _ := s.Command("curl -k -L " + url)
					return out
				}, 5*time.Minute, 1*time.Second).Should(And(ContainSubstring("BEGIN CERTIFICATE"), ContainSubstring("127.0.0.1.nip.io/v1-rancheros/registration")))
			})
		})
	})
})
