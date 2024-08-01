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

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/rancher/elemental-operator/api/v1beta1"
	utilmocks "github.com/rancher/elemental-operator/pkg/util/mocks"
	"github.com/twpayne/go-vfs"
	"github.com/twpayne/go-vfs/vfst"
	"go.uber.org/mock/gomock"
)

func TestInstall(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Network Suite")
}

var _ = Describe("network configurator", Label("network", "configurator"), func() {
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
		networkConfigurator = &nmstateConfigurator{
			fs:     fs,
			runner: runner,
		}
		DeferCleanup(fsCleanup)
	})
	It("should return error on empty network config", func() {
		_, err := networkConfigurator.GetNetworkConfigApplicator(v1beta1.NetworkConfig{})
		Expect(errors.Is(err, ErrEmptyConfig)).Should(BeTrue(), "Empty config must return ErrEmptyConfig")
	})
	It("should return yip config applicator", func() {
		wantNetworkConfig := v1beta1.NetworkConfig{
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

		runner.EXPECT().Run("nmstatectl", "apply", nmstateTempPath).Return(nil)
		//prepare some dummy nmconnection files to simulate `nmstatectl apply` result
		Expect(vfs.MkdirAll(fs, systemConnectionsDir, 0700)).Should(Succeed())
		Expect(fs.WriteFile(filepath.Join(systemConnectionsDir, "wired1.nmconnection"), []byte("[connection]\nid=Wired connection 1\n"), 0600)).Should(Succeed())
		Expect(fs.WriteFile(filepath.Join(systemConnectionsDir, "wired2.nmconnection"), []byte("[connection]\nid=Wired connection 2\n"), 0600)).Should(Succeed())

		applicator, err := networkConfigurator.GetNetworkConfigApplicator(wantNetworkConfig)
		Expect(err).ShouldNot(HaveOccurred())

		// Test the variable substitution took place when generating the nmstate config from the template
		compareFiles(fs, nmstateTempPath, "_testdata/nmstate-intermediate.yaml")

		stage, found := applicator.Stages["initramfs"]
		Expect(found).To(BeTrue(), "Config should be applied at initramfs stage")

		Expect(stage[0].If).To(Equal("[ ! -f /run/elemental/recovery_mode ]"), "Network config must be applied on active or passive systems only (not recovery)")
		Expect(len(stage[0].Files)).To(Equal(2), "Two nmconnection files must have been copied")
		Expect(stage[0].Files[0].Content).To(Equal("[connection]\nid=Wired connection 1\n"))
		Expect(stage[0].Files[0].Path).To(Equal(filepath.Join(systemConnectionsDir, "wired1.nmconnection")))
		Expect(stage[0].Files[1].Content).To(Equal("[connection]\nid=Wired connection 2\n"))
		Expect(stage[0].Files[1].Path).To(Equal(filepath.Join(systemConnectionsDir, "wired2.nmconnection")))
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
