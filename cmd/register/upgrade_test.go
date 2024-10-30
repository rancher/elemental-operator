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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/rancher/elemental-operator/pkg/elementalcli"
	"github.com/rancher/elemental-operator/pkg/upgrade"
	upgrademocks "github.com/rancher/elemental-operator/pkg/upgrade/mocks"
	utilmocks "github.com/rancher/elemental-operator/pkg/util/mocks"
)

var _ = Describe("elemental-register upgrade", Label("elemental-register", "upgrade"), func() {
	var upgrader *upgrademocks.MockUpgrader
	var nsenter *utilmocks.MockNsEnter
	BeforeEach(func() {
		mockCtrl := gomock.NewController(GinkgoT())
		upgrader = upgrademocks.NewMockUpgrader(mockCtrl)
		nsenter = utilmocks.NewMockNsEnter(mockCtrl)
	})
	It("should error if correlationID missing", func() {
		environment := upgrade.Environment{}
		err := upgradeElemental(nsenter, upgrader, environment)
		Expect(err).To(Equal(ErrMissingCorrelationID))
	})
	It("should not upgrade if system shutting down", func() {
		environment := upgrade.Environment{
			HostDir: "/foo",
			Config: elementalcli.UpgradeConfig{
				CorrelationID: "bar",
			},
		}
		nsenter.EXPECT().IsSystemShuttingDown(environment.HostDir).Return(true, nil)
		err := upgradeElemental(nsenter, upgrader, environment)
		Expect(err).To(Equal(ErrAlreadyShuttingDown))
	})
	It("should upgrade and reboot", func() {
		environment := upgrade.Environment{
			HostDir: "/foo",
			Config: elementalcli.UpgradeConfig{
				CorrelationID: "bar",
			},
		}
		gomock.InOrder(
			nsenter.EXPECT().IsSystemShuttingDown(environment.HostDir).Return(false, nil),
			upgrader.EXPECT().UpgradeElemental(environment).Return(true, nil),
			nsenter.EXPECT().Reboot(environment.HostDir),
		)

		err := upgradeElemental(nsenter, upgrader, environment)
		Expect(err).To(Equal(ErrRebooting))
	})
	It("should upgrade and not reboot", func() {
		environment := upgrade.Environment{
			HostDir: "/foo",
			Config: elementalcli.UpgradeConfig{
				CorrelationID: "bar",
			},
		}
		gomock.InOrder(
			nsenter.EXPECT().IsSystemShuttingDown(environment.HostDir).Return(false, nil),
			upgrader.EXPECT().UpgradeElemental(environment).Return(false, nil),
		)

		Expect(upgradeElemental(nsenter, upgrader, environment)).Should(Succeed())
	})
})
