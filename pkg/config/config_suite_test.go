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

package config_test

import (
	"fmt"
	eleconfig "github.com/rancher/elemental-operator/pkg/config"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Elemental Config Suite")
}

var _ = Describe("Elemental config unit tests", func() {
	Context("Unmarshall", func() {
		var v *viper.Viper
		var fs afero.Fs
		var config eleconfig.Config
		var createElementalConfig func(config []byte)

		BeforeEach(func() {
			v = viper.New()
			fs = afero.NewMemMapFs()
			v.SetFs(fs)
			config = eleconfig.Config{}

			createElementalConfig = func(config []byte) {
				err := afero.WriteFile(fs, "/test.yaml", config, os.ModePerm)
				Expect(err).ToNot(HaveOccurred())
				v.SetConfigFile("/test.yaml")
				err = v.MergeInConfig()
				if err != nil {
					fmt.Fprintf(GinkgoWriter, err.Error())
				}
			}
		})
		It("Unmarshalls values into the config properly", func() {
			createElementalConfig([]byte(`
elemental:
  registration:
    no-smbios: true
    emulate-tpm: true
    emulated-tpm-seed: 42`))
			Expect(v.Unmarshal(&config)).ToNot(HaveOccurred())
			Expect(config.Elemental.Registration.NoSMBIOS).To(BeTrue())
			Expect(config.Elemental.Registration.EmulateTPM).To(BeTrue())
			Expect(config.Elemental.Registration.EmulatedTPMSeed).To(Equal(int64(42)))
		})
		It("Doesnt unmarshall the old values", func() {
			// Use old values, not valid anymore
			createElementalConfig([]byte(`
elemental:
  registration:
    nosmbios: true
    emulateTPM: true
    emulatedTPMSeed: 42`))
			Expect(v.Unmarshal(&config)).ToNot(HaveOccurred())
			// Check that the default values were not changed
			Expect(config.Elemental.Registration.NoSMBIOS).To(BeFalse())
			Expect(config.Elemental.Registration.EmulateTPM).To(BeFalse())
			Expect(config.Elemental.Registration.EmulatedTPMSeed).To(Equal(int64(0)))
		})
	})
})
