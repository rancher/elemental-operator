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

package upgrade

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/elemental-operator/pkg/elementalcli"
	"github.com/twpayne/go-vfs"
	"github.com/twpayne/go-vfs/vfst"
	"go.uber.org/mock/gomock"

	climocks "github.com/rancher/elemental-operator/pkg/elementalcli/mocks"
	utilmocks "github.com/rancher/elemental-operator/pkg/util/mocks"
)

func TestUpgrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upgrade suite")
}

var correlationIDFixture = "just a correlation id"
var stateFixture = elementalcli.State{
	StatePartition: elementalcli.PartitionState{
		Snapshots: map[int]*elementalcli.Snapshot{
			0: {
				Active: true,
				Labels: map[string]string{
					correlationIDLabelKey: correlationIDFixture,
				},
			},
		},
	},
}

var _ = Describe("elemental upgrade", Label("upgrade"), func() {
	var fs *vfst.TestFS
	var err error
	var fsCleanup func()
	var runner *climocks.MockRunner
	var cmd *utilmocks.MockCommandRunner
	var upgrade Upgrader
	BeforeEach(func() {
		fs, fsCleanup, err = vfst.NewTestFS(map[string]interface{}{"/tmp/init": ""})
		Expect(err).ToNot(HaveOccurred())
		mockCtrl := gomock.NewController(GinkgoT())
		runner = climocks.NewMockRunner(mockCtrl)
		cmd = utilmocks.NewMockCommandRunner(mockCtrl)
		upgrade = &upgrader{
			fs:     fs,
			runner: runner,
			cmd:    cmd,
		}
		DeferCleanup(fsCleanup)
	})
	When("handling upgrade cloud config", func() {
		It("should write upgrade config", func() {
			// Prepare dummy config first
			wantUpgradeConfigPath := "/tmp/cloud-config.yaml"
			wantUpgradeConfig := []byte("just a test config")
			Expect(fs.WriteFile(wantUpgradeConfigPath, wantUpgradeConfig, os.ModePerm)).Should(Succeed())
			Expect(vfs.MkdirAll(fs, "/oem", os.ModePerm)).To(Succeed())

			// Prepare upgrade environment
			environment := Environment{
				Config: elementalcli.UpgradeConfig{
					CorrelationID: "new correlation id",
				},
				CloudConfigPath: wantUpgradeConfigPath,
			}
			runner.EXPECT().GetState().Return(stateFixture, nil)
			cmd.EXPECT().Run(gomock.Any(), gomock.Any()).AnyTimes()
			runner.EXPECT().Upgrade(gomock.Any()).Return(nil)

			_, err := upgrade.UpgradeElemental(environment)
			Expect(err).ShouldNot(HaveOccurred())

			upgradeConfig, err := fs.ReadFile(upgradeCloudConfigPath)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(upgradeConfig).Should(Equal(wantUpgradeConfig))
		})
		It("should override upgrade config if already existing", func() {
			// Prepare dummy config first
			wantUpgradeConfigPath := "/tmp/cloud-config.yaml"
			wantUpgradeConfig := []byte("just a test config")
			Expect(fs.WriteFile(wantUpgradeConfigPath, wantUpgradeConfig, os.ModePerm)).Should(Succeed())
			// Write already existing config on system
			Expect(vfs.MkdirAll(fs, "/oem", os.ModePerm)).To(Succeed())
			Expect(fs.WriteFile(upgradeCloudConfigPath, []byte("just an old config to be replaced"), os.ModePerm)).Should(Succeed())

			// Prepare upgrade environment
			environment := Environment{
				Config: elementalcli.UpgradeConfig{
					CorrelationID: "new correlation id",
				},
				CloudConfigPath: wantUpgradeConfigPath,
			}
			runner.EXPECT().GetState().Return(stateFixture, nil)
			cmd.EXPECT().Run(gomock.Any(), gomock.Any()).AnyTimes()
			runner.EXPECT().Upgrade(gomock.Any()).Return(nil)

			_, err := upgrade.UpgradeElemental(environment)
			Expect(err).ShouldNot(HaveOccurred())

			upgradeConfig, err := fs.ReadFile(upgradeCloudConfigPath)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(upgradeConfig).Should(Equal(wantUpgradeConfig))
		})
		It("should delete system config if upgrade config is missing", func() {
			// Write already existing config on system
			Expect(vfs.MkdirAll(fs, "/oem", os.ModePerm)).To(Succeed())
			Expect(fs.WriteFile(upgradeCloudConfigPath, []byte("just an old config to be replaced"), os.ModePerm)).Should(Succeed())

			// Prepare upgrade environment
			environment := Environment{
				Config: elementalcli.UpgradeConfig{
					CorrelationID: "new correlation id",
				},
				CloudConfigPath: "/tmp/cloud-config.yaml",
			}
			runner.EXPECT().GetState().Return(stateFixture, nil)
			cmd.EXPECT().Run(gomock.Any(), gomock.Any()).AnyTimes()
			runner.EXPECT().Upgrade(gomock.Any()).Return(nil)

			_, err := upgrade.UpgradeElemental(environment)
			Expect(err).ShouldNot(HaveOccurred())

			_, err = fs.ReadFile(upgradeCloudConfigPath)
			Expect(os.IsNotExist(err)).Should(BeTrue())
		})
	})
	When("verifying correlation ID is applied", func() {
		It("should not upgrade if correlation ID found", func() {
			environment := Environment{
				Config: elementalcli.UpgradeConfig{
					CorrelationID: correlationIDFixture,
				},
			}
			runner.EXPECT().GetState().Return(stateFixture, nil)
			cmd.EXPECT().Run(gomock.Any(), gomock.Any()).AnyTimes()

			reboot, err := upgrade.UpgradeElemental(environment)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(reboot).Should(BeFalse())
		})
		It("should not upgrade if correlation ID found on passive snapshot", func() {
			environment := Environment{
				Config: elementalcli.UpgradeConfig{
					CorrelationID: correlationIDFixture,
				},
			}
			stateFixturePassiveSnapshot := stateFixture
			stateFixturePassiveSnapshot.StatePartition.Snapshots[0].Active = false
			runner.EXPECT().GetState().Return(stateFixturePassiveSnapshot, nil)
			cmd.EXPECT().Run(gomock.Any(), gomock.Any()).AnyTimes()

			reboot, err := upgrade.UpgradeElemental(environment)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(reboot).Should(BeFalse())
		})
		It("should upgrade if there are no snapshots", func() {
			environment := Environment{
				HostDir: "/hostdir",
				Config: elementalcli.UpgradeConfig{
					CorrelationID: correlationIDFixture,
				},
				CloudConfigPath: "/tmp/cloud-config.yaml",
			}
			wantConfig := environment.Config
			wantConfig.SnapshotLabels = elementalcli.KeyValuePair{correlationIDLabelKey: environment.Config.CorrelationID}

			stateFixtureNoSnapshots := stateFixture
			stateFixtureNoSnapshots.StatePartition.Snapshots = map[int]*elementalcli.Snapshot{}

			gomock.InOrder(
				cmd.EXPECT().Run("mount", "--rbind", "/hostdir/dev", "/dev").Return(nil),
				cmd.EXPECT().Run("mount", "--rbind", "/hostdir/run", "/run").Return(nil),
				runner.EXPECT().GetState().Return(stateFixtureNoSnapshots, nil),
				runner.EXPECT().Upgrade(wantConfig).Return(nil),
			)

			reboot, err := upgrade.UpgradeElemental(environment)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(reboot).Should(BeTrue())
		})
		It("should upgrade if correlation ID is not found", func() {
			environment := Environment{
				HostDir: "/hostdir",
				Config: elementalcli.UpgradeConfig{
					CorrelationID: "a new correlation id",
				},
				CloudConfigPath: "/tmp/cloud-config.yaml",
			}
			wantConfig := environment.Config
			wantConfig.SnapshotLabels = elementalcli.KeyValuePair{correlationIDLabelKey: environment.Config.CorrelationID}

			gomock.InOrder(
				cmd.EXPECT().Run("mount", "--rbind", "/hostdir/dev", "/dev").Return(nil),
				cmd.EXPECT().Run("mount", "--rbind", "/hostdir/run", "/run").Return(nil),
				runner.EXPECT().GetState().Return(stateFixture, nil),
				runner.EXPECT().Upgrade(wantConfig).Return(nil),
			)

			reboot, err := upgrade.UpgradeElemental(environment)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(reboot).Should(BeTrue())
		})
	})
})
