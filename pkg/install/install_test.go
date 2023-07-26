/*
Copyright © 2022 - 2023 SUSE LLC

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

package install

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/twpayne/go-vfs"
	"github.com/twpayne/go-vfs/vfst"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestInstall(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Install Suite")
}

var (
	configFixture = elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: elementalv1.Registration{
				URL:             "https://127.0.0.1.sslip.io",
				CACert:          "Just for testing",
				EmulateTPM:      true,
				EmulatedTPMSeed: -1,
				NoSMBIOS:        true,
				Auth:            "test",
			},
			SystemAgent: elementalv1.SystemAgent{
				URL:             "https://127.0.0.1.sslip.io/k8s/cluster/local",
				Token:           "a test token",
				SecretName:      "a test secret",
				SecretNamespace: "a test namespace",
			},
			Install: elementalv1.Install{
				Firmware:         "a test firmware",
				Device:           "a test device",
				NoFormat:         true,
				ConfigURLs:       []string{"foo", "bar"},
				ISO:              "a test iso",
				SystemURI:        "a system uri",
				Debug:            true,
				TTY:              "a test tty",
				PowerOff:         true,
				Reboot:           true,
				EjectCD:          true,
				DisableBootEntry: true,
				ConfigDir:        "a test config dir",
			},
		},
		CloudConfig: map[string]runtime.RawExtension{
			"users": {
				Raw: []byte(`[{"name":"root","passwd":"root"}]`),
			},
		},
	}
)

var _ = Describe("is system installed", Label("installation"), func() {
	var fs vfs.FS
	var err error
	var fsCleanup func()
	var installer Installer
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{})
		Expect(err).ToNot(HaveOccurred())
		installer = NewInstaller(fs)
		DeferCleanup(fsCleanup)
	})
	When("system is already installed", func() {
		BeforeEach(func() {
			Expect(vfs.MkdirAll(fs, filepath.Dir(stateInstallFile), 0700)).ToNot(HaveOccurred())
			Expect(fs.WriteFile(stateInstallFile, []byte("{}\n"), os.ModePerm)).ToNot(HaveOccurred())
		})
		It("should return system is installed", func() {
			Expect(installer.IsSystemInstalled()).To(BeTrue())
		})
	})
	When("system is not installed", func() {
		It("should return system is not installed", func() {
			Expect(installer.IsSystemInstalled()).To(BeFalse())
		})
	})
})

var _ = Describe("update cloud config", Label("installation", "cloud-config"), func() {
	var fs vfs.FS
	var err error
	var fsCleanup func()
	var installer Installer
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{})
		Expect(err).ToNot(HaveOccurred())
		installer = NewInstaller(fs)
		DeferCleanup(fsCleanup)
	})
	When("system is already installed", func() {
		BeforeEach(func() {
			Expect(vfs.MkdirAll(fs, filepath.Dir(stateInstallFile), 0700)).ToNot(HaveOccurred())
			Expect(fs.WriteFile(stateInstallFile, []byte("{}\n"), os.ModePerm)).ToNot(HaveOccurred())
		})
		It("should write config on /oem", func() {
			Expect(installer.UpdateCloudConfig(configFixture)).ToNot(HaveOccurred())
			checkConfigInDir(fs, "/oem")
		})
	})
	When("system is not installed", func() {
		It("should write config on /run/cos/oem", func() {
			Expect(installer.UpdateCloudConfig(configFixture)).ToNot(HaveOccurred())
			checkConfigInDir(fs, "/run/cos/oem")
		})
	})
})

func checkConfigInDir(fs vfs.FS, dir string) {
	config := elementalv1.Config{}
	registrationConfigFile, err := fs.ReadFile(fmt.Sprintf("%s/registration/config.yaml", dir))
	Expect(err).ToNot(HaveOccurred())
	Expect(yaml.Unmarshal(registrationConfigFile, &config)).ToNot(HaveOccurred())
	Expect(config).To(Equal(elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: configFixture.Elemental.Registration,
		},
	}))

	systemAgentPlanRaw, err := fs.ReadFile(fmt.Sprintf("%s/elemental-agent-config-plan.yaml", dir))
	Expect(err).ToNot(HaveOccurred())
	wantSystemAgentPlan, err := ioutil.ReadFile("_testdata/agent-plan.txt")
	Expect(err).ToNot(HaveOccurred())
	Expect(string(systemAgentPlanRaw)).To(Equal(string(wantSystemAgentPlan)))

	cloudConfigRaw, err := fs.ReadFile(fmt.Sprintf("%s/cloud-config.yaml", dir))
	Expect(err).ToNot(HaveOccurred())
	wantCloudConfig, err := ioutil.ReadFile("_testdata/cloud-config.txt")
	Expect(string(cloudConfigRaw)).To(Equal(string(wantCloudConfig)))
}
