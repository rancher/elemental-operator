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

package plainauth

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
)

func TestRegisterPlainAuth(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Register PlainAuth")
}

var _ = Describe("is not filled", Label("plainauth"), func() {
	It("returns true when empty", func() {
		value := ""
		res := isNotFilled(value)
		Expect(res).To(BeTrue())
	})
	It("returns true when 'Unspecified'", func() {
		value := "Unspecified"
		res := isNotFilled(value)
		Expect(res).To(BeTrue())
	})
	It("returns true when 'None'", func() {
		value := "None"
		res := isNotFilled(value)
		Expect(res).To(BeTrue())
	})
	It("returns true when 'Not ...'", func() {
		value := "Not Present"
		res := isNotFilled(value)
		Expect(res).To(BeTrue())
	})
	It("returns false when has a regular value", func() {
		value := "Data"
		res := isNotFilled(value)
		Expect(res).To(BeFalse())
	})
})

var _ = Describe("authentication client init", Label("plainauth"), func() {
	It("returns error on unknown registration authentication", func() {
		reg := elementalv1.Registration{
			Auth: "unknown",
		}
		authCli := AuthClient{}
		err := authCli.Init(reg)
		Expect(err).To(HaveOccurred())
	})
})
