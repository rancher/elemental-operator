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

package main

import (
	"os"
	"path"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/install"
	"github.com/rancher/elemental-operator/pkg/register"
	"github.com/rancher/elemental-operator/pkg/register/mocks"
	"github.com/spf13/cobra"
	"github.com/twpayne/go-vfs"
	"github.com/twpayne/go-vfs/vfst"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"
)

func TestRegister(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Register CLI Suite")
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
		},
	}
)

var _ = Describe("elemental-register arguments", Label("registration", "cli"), func() {
	var fs vfs.FS
	var err error
	var fsCleanup func()
	var cmd *cobra.Command
	var mockCtrl *gomock.Controller
	var client *mocks.MockClient
	When("system is already installed", func() {
		BeforeEach(func() {
			fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{
				"/run/initramfs/cos-state/state.yaml": "{}/n",
			})
			Expect(err).ToNot(HaveOccurred())
			mockCtrl = gomock.NewController(GinkgoT())
			client = mocks.NewMockClient(mockCtrl)
			cmd = newCommand(fs, client, register.NewFileStateHandler(fs), install.NewInstaller(fs))
			DeferCleanup(fsCleanup)
		})
		It("should return no error when printing version", func() {
			cmd.SetArgs([]string{"--version"})
			Expect(cmd.Execute()).ToNot(HaveOccurred())
		})
		When("using existing default config", func() {
			BeforeEach(func() {
				bytes, err := yaml.Marshal(configFixture)
				Expect(err).ToNot(HaveOccurred())
				Expect(vfs.MkdirAll(fs, path.Dir(defaultConfigPath), os.ModePerm)).ToNot(HaveOccurred())
				Expect(fs.WriteFile(defaultConfigPath, bytes, os.ModePerm)).ToNot(HaveOccurred())
			})
			It("should use the config if no arguments passed", func() {
				client.EXPECT().Register(configFixture.Elemental.Registration, []byte(configFixture.Elemental.Registration.CACert)).Return([]byte("{}\n"), nil)
				Expect(cmd.Execute()).ToNot(HaveOccurred())
			})
		})
	})
})
