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

package install

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jaypipes/ghw"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/twpayne/go-vfs"
	"github.com/twpayne/go-vfs/vfst"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	climocks "github.com/rancher/elemental-operator/pkg/elementalcli/mocks"
	"github.com/rancher/elemental-operator/pkg/register"
)

var (
	configFixture = elementalv1.Config{
		Elemental: elementalv1.Elemental{
			Registration: elementalv1.Registration{
				URL:             "https://127.0.0.1.sslip.io/test/registration/endpoint",
				CACert:          "a test ca",
				EmulateTPM:      true,
				EmulatedTPMSeed: 9876543210,
				NoSMBIOS:        true,
				Auth:            "a test auth",
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
				Enabled:         true,
				ResetPersistent: false,
				ResetOEM:        false,
				ConfigURLs:      []string{"foo", "bar"},
				SystemURI:       "a system uri",
				PowerOff:        true,
				Reboot:          true,
			},
			SystemAgent: elementalv1.SystemAgent{
				URL:             "https://127.0.0.1.sslip.io/test/control/plane/endpoint",
				Token:           "a test token",
				SecretName:      "a test secret name",
				SecretNamespace: "a test namespace",
			},
		},
		CloudConfig: map[string]runtime.RawExtension{
			"users": {
				Raw: []byte(`[{"name":"root","passwd":"root"}]`),
			},
		},
	}
	stateFixture = register.State{
		InitialRegistration: time.Date(2023, time.August, 2, 12, 35, 10, 3, time.UTC),
		LastUpdate:          time.Time{},
		EmulatedTPM:         true,
		EmulatedTPMSeed:     987654321,
	}
)

func TestInstall(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Install Suite")
}

// var _ = Describe("installer install elemental", Label("installer", "install"), func() {
// 	var fs *vfst.TestFS
// 	var err error
// 	var fsCleanup func()
// 	var cliRunner *climocks.MockRunner
// 	var install Installer
// 	BeforeEach(func() {
// 		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{"/tmp/init": ""})
// 		Expect(err).ToNot(HaveOccurred())
// 		mockCtrl := gomock.NewController(GinkgoT())
// 		cliRunner = climocks.NewMockRunner(mockCtrl)
// 		install = &installer{
// 			fs:     fs,
// 			runner: cliRunner,
// 		}
// 		DeferCleanup(fsCleanup)
// 	})
// 	It("should call elemental install", func() {
// 		wantConfig := configFixture.DeepCopy()
// 		wantConfig.Elemental.Install.ConfigURLs = append(wantConfig.Elemental.Install.ConfigURLs, additionalConfigs(fs)...)
// 		cliRunner.EXPECT().Install(wantConfig.Elemental.Install).Return(nil)
// 		Expect(install.InstallElemental(configFixture, stateFixture)).ToNot(HaveOccurred())
// 		checkConfigs(fs)
// 	})
// })

var _ = Describe("installer pick device", Label("installer", "install", "device", "disk"), func() {
	var fs *vfst.TestFS
	var err error
	var fsCleanup func()
	var cliRunner *climocks.MockRunner
	var install *installer
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{"/tmp/init": ""})
		Expect(err).ToNot(HaveOccurred())
		mockCtrl := gomock.NewController(GinkgoT())
		cliRunner = climocks.NewMockRunner(mockCtrl)
		DeferCleanup(fsCleanup)
	})
	It("should pick single device no selectors", func() {
		install = &installer{
			fs:     fs,
			runner: cliRunner,
			disks:  []*ghw.Disk{{Name: "pickme"}},
		}
		actualDevice, err := install.findInstallationDevice(elementalv1.DeviceSelector{})
		Expect(err).ToNot(HaveOccurred())
		Expect(actualDevice).To(Equal("/dev/pickme"))
	})
	It("should pick device based on selector name", func() {
		install = &installer{
			fs:     fs,
			runner: cliRunner,
			disks: []*ghw.Disk{
				{Name: "sda"},
				{Name: "sdb"},
				{Name: "sdc"},
				{Name: "sdd"},
				{Name: "sde"},
				{Name: "sdf"},
				{Name: "sdg"},
			},
		}
		selector := elementalv1.DeviceSelector{
			{
				Key:      elementalv1.DeviceSelectorKeyName,
				Operator: elementalv1.DeviceSelectorOpIn,
				Values:   []string{"/dev/sdd"},
			},
		}

		actualDevice, err := install.findInstallationDevice(selector)
		Expect(err).ToNot(HaveOccurred())
		Expect(actualDevice).To(Equal("/dev/sdd"))
	})
	It("should pick device less than 100Gi", func() {
		install = &installer{
			fs:     fs,
			runner: cliRunner,
			disks: []*ghw.Disk{
				{Name: "sda", SizeBytes: 85899345920},
				{Name: "sdb", SizeBytes: 214748364800},
			},
		}
		selector := elementalv1.DeviceSelector{
			{
				Key:      elementalv1.DeviceSelectorKeySize,
				Operator: elementalv1.DeviceSelectorOpLt,
				Values:   []string{"100Gi"},
			},
		}

		actualDevice, err := install.findInstallationDevice(selector)
		Expect(err).ToNot(HaveOccurred())
		Expect(actualDevice).To(Equal("/dev/sda"))
	})
	It("should pick device greater than 100Gi", func() {
		install = &installer{
			fs:     fs,
			runner: cliRunner,
			disks: []*ghw.Disk{
				{Name: "sda", SizeBytes: 85899345920},
				{Name: "sdb", SizeBytes: 214748364800},
			},
		}
		selector := elementalv1.DeviceSelector{
			{
				Key:      elementalv1.DeviceSelectorKeySize,
				Operator: elementalv1.DeviceSelectorOpGt,
				Values:   []string{"100Gi"},
			},
		}

		actualDevice, err := install.findInstallationDevice(selector)
		Expect(err).ToNot(HaveOccurred())
		Expect(actualDevice).To(Equal("/dev/sdb"))
	})
	It("should not error out for 2 matching devices", func() {
		install = &installer{
			fs:     fs,
			runner: cliRunner,
			disks: []*ghw.Disk{
				{Name: "sda"},
				{Name: "sdb"},
			},
		}
		selector := elementalv1.DeviceSelector{
			{
				Key:      elementalv1.DeviceSelectorKeyName,
				Operator: elementalv1.DeviceSelectorOpIn,
				Values:   []string{"/dev/sda", "/dev/sdb"},
			},
		}
		actualDevice, err := install.findInstallationDevice(selector)
		Expect(err).ToNot(HaveOccurred())
		Expect(actualDevice).ToNot(BeEmpty())
	})
	It("should error out for no devices", func() {
		install = &installer{
			fs:     fs,
			runner: cliRunner,
			disks:  []*ghw.Disk{},
		}
		actualDevice, err := install.findInstallationDevice(elementalv1.DeviceSelector{})
		Expect(err).To(HaveOccurred())
		Expect(actualDevice).To(BeEmpty())
	})
})

// var _ = Describe("installer reset elemental", Label("installer", "reset"), func() {
// 	var fs *vfst.TestFS
// 	var err error
// 	var fsCleanup func()
// 	var cliRunner *climocks.MockRunner
// 	var install Installer
// 	BeforeEach(func() {
// 		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{"/tmp/init": "", "/oem/init": ""})
// 		Expect(err).ToNot(HaveOccurred())
// 		mockCtrl := gomock.NewController(GinkgoT())
// 		cliRunner = climocks.NewMockRunner(mockCtrl)
// 		install = &installer{
// 			fs:     fs,
// 			runner: cliRunner,
// 		}
// 		DeferCleanup(fsCleanup)
// 	})
// 	It("should call elemental reset", func() {
// 		wantConfig := configFixture.DeepCopy()
// 		wantConfig.Elemental.Reset.ConfigURLs = append(wantConfig.Elemental.Reset.ConfigURLs, additionalConfigs(fs)...)
// 		cliRunner.EXPECT().Reset(wantConfig.Elemental.Reset).Return(nil)
// 		Expect(install.ResetElemental(configFixture, stateFixture)).ToNot(HaveOccurred())
// 		checkConfigs(fs)
// 	})
// 	It("should remove reset plan", func() {
// 		Expect(fs.WriteFile(controllers.LocalResetPlanPath, []byte("{}\n"), os.FileMode(0600))).ToNot(HaveOccurred())
// 		cliRunner.EXPECT().Reset(gomock.Any()).Return(nil)
// 		Expect(install.ResetElemental(configFixture, stateFixture)).ToNot(HaveOccurred())
// 		_, err := fs.Stat(controllers.LocalResetPlanPath)
// 		Expect(err).To(MatchError(os.ErrNotExist))
// 	})
// })

func additionalConfigs(fs *vfst.TestFS) []string {
	// Prefix the go-vfs temp dir because that's what file.Name() returns
	return []string{
		fmt.Sprintf("%s%s", fs.TempDir(), tempSystemAgent),
		fmt.Sprintf("%s%s", fs.TempDir(), tempCloudInit),
		fmt.Sprintf("%s%s", fs.TempDir(), tempRegistrationConf),
		fmt.Sprintf("%s%s", fs.TempDir(), tempRegistrationState),
	}
}

func checkConfigs(fs vfs.FS) {
	compareFiles(fs, tempRegistrationConf, "_testdata/registration-config-config.yaml")
	compareFiles(fs, tempRegistrationState, "_testdata/registration-state-config.yaml")
	compareFiles(fs, tempSystemAgent, "_testdata/system-agent-config.yaml")
	compareFiles(fs, tempCloudInit, "_testdata/cloud-init-config.yaml")
}

func compareFiles(fs vfs.FS, got string, want string) {
	gotFile, err := fs.ReadFile(got)
	Expect(err).ToNot(HaveOccurred())
	wantFile, err := os.ReadFile(want)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(gotFile)).To(Equal(string(wantFile)))
}
