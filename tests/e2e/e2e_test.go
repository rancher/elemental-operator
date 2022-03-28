package e2e_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/rancheros-operator/tests/catalog"
	"github.com/rancher-sandbox/rancheros-operator/tests/sut"
)

var _ = Describe("os2 e2e tests", func() {
	kubectl := sut.NewKubectl()
	Context("ros-operator", func() {
		It("create a machine registration resource", func() {
			mr := catalog.NewMachineRegistration("machine-registration", map[string]interface{}{
				"install":   map[string]string{"device": "/dev/vda"},
				"rancheros": map[string]interface{}{"install": map[string]string{"isoUrl": "https://something.example.com"}},
				"users": []map[string]string{
					{
						"name":   "root",
						"passwd": "root",
					},
				},
			})
			err := kubectl.ApplyYAML("fleet-default", "machine-registration", mr)
			Expect(err).ToNot(HaveOccurred())

			var url string

			Eventually(func() string {
				e, err := sut.GetData("fleet-default", "machineregistration", "machine-registration", `jsonpath="{.status.registrationURL}"`)
				if err != nil {
					fmt.Println(err)
				}
				url = string(e)
				return string(e)
			}, 1*time.Minute, 2*time.Second).Should(
				And(
					ContainSubstring(fmt.Sprintf("%s.xip.io/v1-rancheros/registration", externalIP)),
				),
			)

			out, err := sut.Get(url)
			Expect(err).ToNot(HaveOccurred())

			Expect(out).Should(
				And(
					ContainSubstring("BEGIN CERTIFICATE"),
					ContainSubstring(fmt.Sprintf("%s.xip.io/v1-rancheros/registration", externalIP)),
				),
			)
		})
	})
})
