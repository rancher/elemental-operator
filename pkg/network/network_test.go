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

package network

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/elemental-operator/api/v1beta1"
	utilmocks "github.com/rancher/elemental-operator/pkg/util/mocks"
	"github.com/rancher/yip/pkg/schema"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/twpayne/go-vfs"
	"github.com/twpayne/go-vfs/vfst"
)

func TestNetwork(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Network Suite")
}

var _ = Describe("configurator", Label("network", "configurator"), func() {
	var fs *vfst.TestFS
	var err error
	var fsCleanup func()
	var runner *utilmocks.MockCommandRunner
	var networkConfigurator Configurator
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{"/tmp/init": ""})
		Expect(err).ToNot(HaveOccurred())
		mockCtrl := gomock.NewController(GinkgoT())
		runner = utilmocks.NewMockCommandRunner(mockCtrl)
		networkConfigurator = &configurator{
			fs:     fs,
			runner: runner,
		}
		DeferCleanup(fsCleanup)
	})
	It("should return error on uknown configurator", func() {
		_, err := networkConfigurator.GetNetworkConfigApplicator(v1beta1.NetworkConfig{Configurator: "does not exist"})
		Expect(errors.Is(err, ErrUnknownConfigurator)).Should(BeTrue(), "Uknown configurator must return ErrUnknownConfigurator")
	})
	It("should return nmconnections yip config applicator", func() {
		wantNetworkConfig := v1beta1.NetworkConfig{
			Configurator: "nmconnections",
			IPAddresses: map[string]string{
				"my-ip-1": "192.168.122.10",
				"my-ip-2": "192.168.122.11",
			},
			Config: map[string]runtime.RawExtension{
				"eth0": {
					Raw: []byte(`"[connection]\nid=Wired connection 1\n[ipv4]\naddress1={my-ip-1}"`),
				},
				"eth1": {
					Raw: []byte(`"[connection]\nid=Wired connection 2\n[ipv4]\naddress1={my-ip-2}"`),
				},
			},
		}
		wantFiles := []schema.File{
			{
				Content:     "[connection]\nid=Wired connection 1\n[ipv4]\naddress1=192.168.122.10",
				Path:        filepath.Join(systemConnectionsDir, "eth0.nmconnection"),
				Permissions: 0600,
			},
			{
				Content:     "[connection]\nid=Wired connection 2\n[ipv4]\naddress1=192.168.122.11",
				Path:        filepath.Join(systemConnectionsDir, "eth1.nmconnection"),
				Permissions: 0600,
			},
		}

		applicator, err := networkConfigurator.GetNetworkConfigApplicator(wantNetworkConfig)
		Expect(err).ShouldNot(HaveOccurred())

		stage, found := applicator.Stages["initramfs"]
		Expect(found).To(BeTrue(), "Config should be applied at initramfs stage")

		Expect(len(stage[0].Files)).To(Equal(2), "Two nmconnection files must have been copied")
		Expect(stage[0].Files).To(ContainElements(wantFiles))
	})
	It("should return nmstate yip config applicator", func() {
		wantNetworkConfig := v1beta1.NetworkConfig{
			Configurator: "nmstate",
			IPAddresses: map[string]string{
				"foo": "192.168.122.10",
				"bar": "192.168.122.11",
			},
			Config: map[string]runtime.RawExtension{
				"foo": {
					Raw: []byte(`"{foo}"`),
				},
				"bar": {
					Raw: []byte(`"{bar}"`),
				},
			},
		}

		wantFiles := []schema.File{
			{
				Content:     "[connection]\nid=Wired connection 1\n",
				Path:        filepath.Join(systemConnectionsDir, "wired1.nmconnection"),
				Permissions: 0600,
			},
			{
				Content:     "[connection]\nid=Wired connection 2\n",
				Path:        filepath.Join(systemConnectionsDir, "wired2.nmconnection"),
				Permissions: 0600,
			},
		}

		runner.EXPECT().Run("nmstatectl", "apply", nmstateTempPath).Return(nil)
		//prepare some dummy nmconnection files to simulate `nmstatectl apply` result
		Expect(vfs.MkdirAll(fs, systemConnectionsDir, 0700)).Should(Succeed())
		Expect(fs.WriteFile(filepath.Join(systemConnectionsDir, "wired1.nmconnection"), []byte("[connection]\nid=Wired connection 1\n"), 0600)).Should(Succeed())
		Expect(fs.WriteFile(filepath.Join(systemConnectionsDir, "wired2.nmconnection"), []byte("[connection]\nid=Wired connection 2\n"), 0600)).Should(Succeed())

		applicator, err := networkConfigurator.GetNetworkConfigApplicator(wantNetworkConfig)
		Expect(err).ShouldNot(HaveOccurred())

		// Test the variable substitution took place when generating the nmstate config from the template
		compareFiles(fs, nmstateTempPath, "_testdata/digested-intermediate.yaml")

		stage, found := applicator.Stages["initramfs"]
		Expect(found).To(BeTrue(), "Config should be applied at initramfs stage")

		Expect(len(stage[0].Files)).To(Equal(2), "Two nmconnection files must have been copied")
		Expect(stage[0].Files).To(ContainElements(wantFiles))
	})
	It("should return nmc yip config applicator", func() {
		wantNetworkConfig := v1beta1.NetworkConfig{
			Configurator: "nmc",
			IPAddresses: map[string]string{
				"foo": "192.168.122.10",
				"bar": "192.168.122.11",
			},
			Config: map[string]runtime.RawExtension{
				"foo": {
					Raw: []byte(`"{foo}"`),
				},
				"bar": {
					Raw: []byte(`"{bar}"`),
				},
			},
		}
		wantFiles := []schema.File{
			{
				Content:     "[connection]\nid=Wired connection 1\n",
				Path:        filepath.Join(systemConnectionsDir, "wired1.nmconnection"),
				Permissions: 0600,
			},
			{
				Content:     "[connection]\nid=Wired connection 2\n",
				Path:        filepath.Join(systemConnectionsDir, "wired2.nmconnection"),
				Permissions: 0600,
			},
		}

		runner.EXPECT().Run("nmc", "generate", "--config-dir", nmcDesiredStatesDir, "--output-dir", nmcNewtorkConfigDir).Return(nil)
		runner.EXPECT().Run("nmc", "apply", "--config-dir", nmcNewtorkConfigDir).Return(nil)
		//prepare some dummy nmconnection files to simulate `nmc apply` result
		Expect(vfs.MkdirAll(fs, systemConnectionsDir, 0700)).Should(Succeed())
		Expect(fs.WriteFile(filepath.Join(systemConnectionsDir, "wired1.nmconnection"), []byte("[connection]\nid=Wired connection 1\n"), 0600)).Should(Succeed())
		Expect(fs.WriteFile(filepath.Join(systemConnectionsDir, "wired2.nmconnection"), []byte("[connection]\nid=Wired connection 2\n"), 0600)).Should(Succeed())

		applicator, err := networkConfigurator.GetNetworkConfigApplicator(wantNetworkConfig)
		Expect(err).ShouldNot(HaveOccurred())

		// Test the variable substitution took place when generating the nmc config from the template
		compareFiles(fs, filepath.Join(nmcDesiredStatesDir, nmcAllConfigName), "_testdata/digested-intermediate.yaml")

		stage, found := applicator.Stages["initramfs"]
		Expect(found).To(BeTrue(), "Config should be applied at initramfs stage")

		Expect(len(stage[0].Files)).To(Equal(2), "Two nmconnection files must have been copied")
		Expect(stage[0].Files).To(ContainElements(wantFiles))
	})
	It("should reset network", func() {
		//prepare a dummy nmconnection file to highlight the need to reset network
		Expect(vfs.MkdirAll(fs, systemConnectionsDir, 0700)).Should(Succeed())
		Expect(fs.WriteFile(filepath.Join(systemConnectionsDir, "wired1.nmconnection"), []byte("[connection]\nid=Wired connection 1\n"), 0600)).Should(Succeed())

		gomock.InOrder(
			runner.EXPECT().Run("find", systemConnectionsDir, "-name", "*.nmconnection", "-type", "f", "-delete").Return(nil),
			runner.EXPECT().Run("nmcli", "connection", "reload").Return(nil),
			runner.EXPECT().Run("systemctl", "restart", "NetworkManager.service").Return(nil),
			runner.EXPECT().Run("systemctl", "start", "NetworkManager-wait-online.service").Return(nil),
			runner.EXPECT().Run("systemctl", "restart", "elemental-system-agent.service").Return(nil),
		)

		Expect(networkConfigurator.ResetNetworkConfig()).Should(Succeed())
	})
	It("should consider reset network done if no nmconnection files", func() {
		Expect(vfs.MkdirAll(fs, systemConnectionsDir, 0700)).Should(Succeed())

		Expect(networkConfigurator.ResetNetworkConfig()).Should(Succeed())
	})
})

func compareFiles(fs vfs.FS, got string, want string) {
	gotFile, err := fs.ReadFile(got)
	Expect(err).ToNot(HaveOccurred())
	wantFile, err := os.ReadFile(want)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(gotFile)).To(Equal(string(wantFile)))
}
