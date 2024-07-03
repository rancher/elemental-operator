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

package main

import (
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/twpayne/go-vfs"
	"github.com/twpayne/go-vfs/vfst"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	imocks "github.com/rancher/elemental-operator/pkg/install/mocks"
	"github.com/rancher/elemental-operator/pkg/register"
	rmocks "github.com/rancher/elemental-operator/pkg/register/mocks"
)

func TestRegister(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Register CLI Suite")
}

var (
	baseConfigFixture = elementalv1.Config{
		Network: elementalv1.NetworkTemplate{},
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
	notoolkitConfigFixture = elementalv1.Config{
		Network: elementalv1.NetworkTemplate{},
		Elemental: elementalv1.Elemental{
			Registration: elementalv1.Registration{
				URL:             "https://127.0.0.1.sslip.io",
				CACert:          "Just for testing",
				EmulateTPM:      true,
				EmulatedTPMSeed: -1,
				NoSMBIOS:        true,
				Auth:            "test",
				NoToolkit:       true,
			},
		},
	}
	alternateConfigFixture = elementalv1.Config{
		Network: elementalv1.NetworkTemplate{},
		Elemental: elementalv1.Elemental{
			Registration: elementalv1.Registration{
				URL:             "https://127.0.0.2.sslip.io",
				CACert:          "alternate ca",
				EmulateTPM:      true,
				EmulatedTPMSeed: 9876543210,
				NoSMBIOS:        true,
				Auth:            "alternate auth",
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
			Reset: elementalv1.Reset{
				Enabled:          true,
				ResetPersistent:  false,
				ResetOEM:         false,
				ConfigURLs:       []string{"foo", "bar"},
				SystemURI:        "a system uri",
				PowerOff:         true,
				Reboot:           true,
				DisableBootEntry: true,
			},
		},
	}
	stateFixture = register.State{
		InitialRegistration: time.Now(),
		LastUpdate:          time.Time{},
		EmulatedTPM:         true,
		EmulatedTPMSeed:     987654321,
	}
)

var _ = Describe("elemental-register", Label("registration", "cli"), func() {
	var fs vfs.FS
	var err error
	var fsCleanup func()
	var cmd *cobra.Command
	var mockCtrl *gomock.Controller
	var client *rmocks.MockClient
	var installer *imocks.MockInstaller
	var stateHandler *rmocks.MockStateHandler
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{})
		Expect(err).ToNot(HaveOccurred())
		mockCtrl = gomock.NewController(GinkgoT())
		client = rmocks.NewMockClient(mockCtrl)
		installer = imocks.NewMockInstaller(mockCtrl)
		stateHandler = rmocks.NewMockStateHandler(mockCtrl)
		cmd = newCommand(fs, client, stateHandler, installer)
		DeferCleanup(fsCleanup)
	})
	It("should return no error when printing version", func() {
		cmd.SetArgs([]string{"--version"})
		Expect(cmd.Execute()).ToNot(HaveOccurred())
	})
	When("using existing default config", func() {
		BeforeEach(func() {
			marshalIntoFile(fs, baseConfigFixture, defaultConfigPath)
			stateHandler.EXPECT().Init(defaultStatePath).Return(nil)
			stateHandler.EXPECT().Load().Return(stateFixture, nil)
			stateHandler.EXPECT().Save(stateFixture).Return(nil)
		})
		It("should use the config if no arguments passed", func() {
			cmd.SetArgs([]string{})
			client.EXPECT().
				Register(baseConfigFixture.Elemental.Registration, []byte(baseConfigFixture.Elemental.Registration.CACert), &stateFixture).
				Return(marshalToBytes(baseConfigFixture), nil)
			Expect(cmd.Execute()).ToNot(HaveOccurred())
		})
		It("should overwrite the config values with passed arguments", func() {
			cmd.SetArgs([]string{
				"--registration-url", alternateConfigFixture.Elemental.Registration.URL,
				"--registration-ca-cert", alternateConfigFixture.Elemental.Registration.CACert,
				"--emulate-tpm",
				"--emulated-tpm-seed", fmt.Sprintf("%d", alternateConfigFixture.Elemental.Registration.EmulatedTPMSeed),
				"--no-smbios=false",
				"--auth", alternateConfigFixture.Elemental.Registration.Auth,
			})
			wantConfig := alternateConfigFixture.DeepCopy()
			wantConfig.Elemental.Registration.NoSMBIOS = false
			client.EXPECT().
				Register(wantConfig.Elemental.Registration, []byte(wantConfig.Elemental.Registration.CACert), &stateFixture).
				Return(marshalToBytes(wantConfig), nil)
			Expect(cmd.Execute()).ToNot(HaveOccurred())
		})
		It("should use config path argument", func() {
			newPath := "/a/custom/config/path/custom-config.yaml"
			cmd.SetArgs([]string{"--config-path", newPath})
			marshalIntoFile(fs, alternateConfigFixture, newPath)
			client.EXPECT().
				Register(alternateConfigFixture.Elemental.Registration, []byte(alternateConfigFixture.Elemental.Registration.CACert), &stateFixture).
				Return(marshalToBytes(alternateConfigFixture), nil)
			Expect(cmd.Execute()).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("elemental-register state", Label("registration", "cli", "state"), func() {
	var fs vfs.FS
	var err error
	var fsCleanup func()
	var cmd *cobra.Command
	var mockCtrl *gomock.Controller
	var client *rmocks.MockClient
	var installer *imocks.MockInstaller
	var stateHandler *rmocks.MockStateHandler
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{})
		Expect(err).ToNot(HaveOccurred())
		mockCtrl = gomock.NewController(GinkgoT())
		client = rmocks.NewMockClient(mockCtrl)
		installer = imocks.NewMockInstaller(mockCtrl)
		stateHandler = rmocks.NewMockStateHandler(mockCtrl)
		cmd = newCommand(fs, client, stateHandler, installer)
		DeferCleanup(fsCleanup)
	})
	When("using existing default config", func() {
		BeforeEach(func() {
			marshalIntoFile(fs, baseConfigFixture, defaultConfigPath)
		})
		It("should use state path argument", func() {
			newPath := "/a/custom/state/path/custom-state.yaml"
			cmd.SetArgs([]string{"--state-path", newPath})
			registrationState := register.State{
				InitialRegistration: time.Now(),
				LastUpdate:          time.Now(),
			}
			stateHandler.EXPECT().Init(newPath).Return(nil)
			stateHandler.EXPECT().Load().Return(registrationState, nil)
			client.EXPECT().Register(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			Expect(cmd.Execute()).ToNot(HaveOccurred())
		})
		It("should not skip registration if lastUpdate is stale", func() {
			cmd.SetArgs([]string{})
			registrationState := register.State{
				InitialRegistration: time.Now(),
				LastUpdate:          time.Now().Add(-25 * time.Hour),
			}
			stateHandler.EXPECT().Init(defaultStatePath).Return(nil)
			stateHandler.EXPECT().Load().Return(registrationState, nil)
			stateHandler.EXPECT().Save(registrationState).Return(nil)
			client.EXPECT().
				Register(baseConfigFixture.Elemental.Registration, []byte(baseConfigFixture.Elemental.Registration.CACert), &registrationState).
				Return(marshalToBytes(baseConfigFixture), nil)
			Expect(cmd.Execute()).ToNot(HaveOccurred())
		})
	})
})

// var _ = Describe("elemental-register --install", Label("registration", "cli", "install"), func() {
// 	var fs vfs.FS
// 	var err error
// 	var fsCleanup func()
// 	var cmd *cobra.Command
// 	var mockCtrl *gomock.Controller
// 	var client *rmocks.MockClient
// 	var installer *imocks.MockInstaller
// 	var stateHandler *rmocks.MockStateHandler
// 	BeforeEach(func() {
// 		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{})
// 		Expect(err).ToNot(HaveOccurred())
// 		mockCtrl = gomock.NewController(GinkgoT())
// 		installer = imocks.NewMockInstaller(mockCtrl)
// 		stateHandler = rmocks.NewMockStateHandler(mockCtrl)
// 		client = rmocks.NewMockClient(mockCtrl)
// 		cmd = newCommand(fs, client, stateHandler, installer)
// 		DeferCleanup(fsCleanup)
// 	})
// 	When("using existing live config", func() {
// 		BeforeEach(func() {
// 			marshalIntoFile(fs, baseConfigFixture, defaultLiveConfigPath)
// 			stateHandler.EXPECT().Init(defaultLiveStatePath).Return(nil)
// 			stateHandler.EXPECT().Load().Return(stateFixture, nil)
// 			stateHandler.EXPECT().Save(stateFixture).Return(nil)
// 		})
// 		It("should trigger install when --install argument", func() {
// 			cmd.SetArgs([]string{"--install"})
// 			installer.EXPECT().InstallElemental(alternateConfigFixture, stateFixture).Return(nil)
// 			client.EXPECT().
// 				Register(baseConfigFixture.Elemental.Registration, []byte(baseConfigFixture.Elemental.Registration.CACert), &stateFixture).
// 				Return(marshalToBytes(alternateConfigFixture), nil)
// 			Expect(cmd.Execute()).ToNot(HaveOccurred())
// 		})
// 	})
// })

var _ = Describe("elemental-register --install --no-toolkit", Label("registration", "cli", "install-notoolkit"), func() {
	var fs vfs.FS
	var err error
	var fsCleanup func()
	var cmd *cobra.Command
	var mockCtrl *gomock.Controller
	var client *rmocks.MockClient
	var installer *imocks.MockInstaller
	var stateHandler *rmocks.MockStateHandler
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{})
		Expect(err).ToNot(HaveOccurred())
		mockCtrl = gomock.NewController(GinkgoT())
		installer = imocks.NewMockInstaller(mockCtrl)
		stateHandler = rmocks.NewMockStateHandler(mockCtrl)
		client = rmocks.NewMockClient(mockCtrl)
		cmd = newCommand(fs, client, stateHandler, installer)
		DeferCleanup(fsCleanup)
	})
	When("using existing default config", func() {
		BeforeEach(func() {
			marshalIntoFile(fs, baseConfigFixture, defaultConfigPath)
			stateHandler.EXPECT().Init(defaultStatePath).Return(nil)
			stateHandler.EXPECT().Load().Return(stateFixture, nil)
			stateHandler.EXPECT().Save(stateFixture).Return(nil)
		})
		It("should trigger local system agent config when --install and --no-toolkit arguments", func() {
			cmd.SetArgs([]string{"--install", "--no-toolkit"})
			installer.EXPECT().WriteLocalSystemAgentConfig(notoolkitConfigFixture.Elemental).Return(nil)
			client.EXPECT().
				Register(notoolkitConfigFixture.Elemental.Registration, []byte(notoolkitConfigFixture.Elemental.Registration.CACert), &stateFixture).
				Return(marshalToBytes(notoolkitConfigFixture), nil)
			Expect(cmd.Execute()).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("elemental-register --reset", Label("registration", "cli", "reset"), func() {
	var fs vfs.FS
	var err error
	var fsCleanup func()
	var cmd *cobra.Command
	var mockCtrl *gomock.Controller
	var client *rmocks.MockClient
	var installer *imocks.MockInstaller
	var stateHandler *rmocks.MockStateHandler
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{})
		Expect(err).ToNot(HaveOccurred())
		mockCtrl = gomock.NewController(GinkgoT())
		installer = imocks.NewMockInstaller(mockCtrl)
		stateHandler = rmocks.NewMockStateHandler(mockCtrl)
		client = rmocks.NewMockClient(mockCtrl)
		cmd = newCommand(fs, client, stateHandler, installer)
		DeferCleanup(fsCleanup)
	})
	When("using existing default config", func() {
		BeforeEach(func() {
			marshalIntoFile(fs, baseConfigFixture, defaultConfigPath)
			stateHandler.EXPECT().Init(defaultStatePath).Return(nil)
			stateHandler.EXPECT().Load().Times(0) // When resetting expect new state to be initialized
			stateHandler.EXPECT().Save(register.State{}).Return(nil)
		})
		It("should trigger reset when --reset argument", func() {
			cmd.SetArgs([]string{"--reset"})
			installer.EXPECT().ResetElemental(alternateConfigFixture, register.State{}).Return(nil)
			client.EXPECT().
				Register(baseConfigFixture.Elemental.Registration, []byte(baseConfigFixture.Elemental.Registration.CACert), &register.State{}).
				Return(marshalToBytes(alternateConfigFixture), nil)
			Expect(cmd.Execute()).ToNot(HaveOccurred())
		})
	})
})

func marshalIntoFile(fs vfs.FS, input any, filePath string) {
	bytes := marshalToBytes(input)
	Expect(vfs.MkdirAll(fs, path.Dir(filePath), os.ModePerm)).ToNot(HaveOccurred())
	Expect(fs.WriteFile(filePath, bytes, os.ModePerm)).ToNot(HaveOccurred())
}

func marshalToBytes(input any) []byte {
	bytes, err := yaml.Marshal(input)
	Expect(err).ToNot(HaveOccurred())
	return bytes
}
