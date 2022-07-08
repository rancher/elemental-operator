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
	http "github.com/rancher-sandbox/ele-testhelpers/http"
	kubectl "github.com/rancher-sandbox/ele-testhelpers/kubectl"

	"github.com/rancher/elemental-operator/pkg/installer"
	"github.com/rancher/elemental-operator/tests/catalog"
)

const testRegistrationNamespace = "cattle-elemental-operator-system"

var _ = Describe("MachineRegistration e2e tests", func() {
	k := kubectl.New()
	Context("registration", func() {

		AfterEach(func() {
			kubectl.New().Delete("machineregistration", "-n", testRegistrationNamespace, "machine-registration")
		})

		It("creates a machine registration resource and a URL attaching CA certificate", Focus, func() {

			spec := catalog.MachineRegistrationSpec{Install: &installer.Install{Device: "/dev/vda", ISO: "https://something.example.com"}}
			mr := catalog.NewMachineRegistration("machine-registration", spec)
			Eventually(func() error {
				return k.ApplyYAML(testRegistrationNamespace, "machine-registration", mr)
			}, 2*time.Minute, 2*time.Second).ShouldNot(HaveOccurred())

			var url string
			Eventually(func() string {
				e, err := kubectl.GetData(testRegistrationNamespace, "machineregistration", "machine-registration", `jsonpath={.status.registrationURL}`)
				if err != nil {
					fmt.Println(err)
				}
				url = string(e)
				return string(e)
			}, 1*time.Minute, 2*time.Second).Should(
				And(
					ContainSubstring(fmt.Sprintf("%s.%s/elemental/registration", externalIP, magicDNS)),
				),
			)

			Eventually(func() string {
				out, err := http.GetInsecure(fmt.Sprintf("https://%s", url))
				if err != nil {
					fmt.Println(err)
				}
				return out
			}, 1*time.Minute, 2*time.Second).Should(
				And(
					ContainSubstring(fmt.Sprintf("%s.%s/elemental/registration", externalIP, magicDNS)),
				),
			)
			// TODO: There are no cacerts anymore being generated, do we drop that? Do we follow up recreating the ca in the controller?
			// TODO: We should check that the install values that we passed are indeed returned by the registration?
		})
	})
})
