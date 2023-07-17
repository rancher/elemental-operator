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
	"fmt"
	"os"
	"path"
	"testing"
	"time"

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
				EmulateTPM:      false,
				EmulatedTPMSeed: -1,
				NoSMBIOS:        false,
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
				cmd.SetArgs([]string{})
				client.EXPECT().Register(configFixture.Elemental.Registration, []byte(configFixture.Elemental.Registration.CACert)).Return([]byte("{}\n"), nil)
				Expect(cmd.Execute()).ToNot(HaveOccurred())
			})
			It("should overwrite the config values with passed arguments", func() {
				wantConfig := elementalv1.Config{
					Elemental: elementalv1.Elemental{
						Registration: elementalv1.Registration{
							URL:             "https://127.0.0.2.sslip.io",
							CACert:          "ca updated by argument",
							EmulateTPM:      true,
							EmulatedTPMSeed: 9876543210,
							NoSMBIOS:        true,
							Auth:            "auth updated by argument",
						},
					},
				}
				cmd.SetArgs([]string{
					"--registration-url", wantConfig.Elemental.Registration.URL,
					"--registration-ca-cert", wantConfig.Elemental.Registration.CACert,
					"--emulate-tpm",
					"--emulated-tpm-seed", fmt.Sprintf("%d", wantConfig.Elemental.Registration.EmulatedTPMSeed),
					"--no-smbios",
					"--auth", wantConfig.Elemental.Registration.Auth,
				})
				client.EXPECT().Register(wantConfig.Elemental.Registration, []byte(wantConfig.Elemental.Registration.CACert)).Return([]byte("{}\n"), nil)
				Expect(cmd.Execute()).ToNot(HaveOccurred())
			})
			It("should use config path argument", func() {
				newPath := fmt.Sprintf("%s/%s", path.Dir(defaultStatePath), "custom-config.yaml")
				cmd.SetArgs([]string{"--config-path", newPath})
				wantConfig := elementalv1.Config{
					Elemental: elementalv1.Elemental{
						Registration: elementalv1.Registration{
							URL:             "https://127.0.0.2.sslip.io",
							CACert:          "ca updated by argument",
							EmulateTPM:      false,
							EmulatedTPMSeed: 9876543210,
							NoSMBIOS:        false,
							Auth:            "auth updated by argument",
						},
					},
				}
				bytes, err := yaml.Marshal(wantConfig)
				Expect(err).ToNot(HaveOccurred())
				Expect(fs.WriteFile(newPath, bytes, os.ModePerm)).ToNot(HaveOccurred())
				client.EXPECT().Register(wantConfig.Elemental.Registration, []byte(wantConfig.Elemental.Registration.CACert)).Return([]byte("{}\n"), nil)
				Expect(cmd.Execute()).ToNot(HaveOccurred())
			})
			It("should skip registration if lastUpdate is recent", func() {
				cmd.SetArgs([]string{})
				registrationState := register.State{
					InitialRegistration: time.Now(),
					LastUpdate:          time.Now(),
				}
				bytes, err := yaml.Marshal(registrationState)
				Expect(err).ToNot(HaveOccurred())
				Expect(fs.WriteFile(defaultStatePath, bytes, os.ModePerm)).ToNot(HaveOccurred())
				client.EXPECT().Register(gomock.Any(), gomock.Any()).Times(0)
				Expect(cmd.Execute()).ToNot(HaveOccurred())
			})
			It("should use state path argument", func() {
				newPath := fmt.Sprintf("%s/%s", path.Dir(defaultStatePath), "custom-state.yaml")
				cmd.SetArgs([]string{"--state-path", newPath})
				registrationState := register.State{
					InitialRegistration: time.Now(),
					LastUpdate:          time.Now(),
				}
				bytes, err := yaml.Marshal(registrationState)
				Expect(err).ToNot(HaveOccurred())
				Expect(fs.WriteFile(newPath, bytes, os.ModePerm)).ToNot(HaveOccurred())
				client.EXPECT().Register(gomock.Any(), gomock.Any()).Times(0)
				Expect(cmd.Execute()).ToNot(HaveOccurred())
			})
		})
	})
})
