/*
Copyright © 2022 - 2026 SUSE LLC

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

package util

import (
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	utilmocks "github.com/rancher/elemental-operator/pkg/util/mocks"
	gomock "go.uber.org/mock/gomock"
)

var _ = Describe("GetIPByIfName", func() {
	var ctrl *gomock.Controller
	var netctrl *utilmocks.MockNetController

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		netctrl = utilmocks.NewMockNetController(ctrl)
	})
	It("should return all IPv4 and IPv6 addresses available", func() {
		netctrl.EXPECT().GetInterfaceAddresses("eth0").Return(
			[]net.IPNet{
				{
					IP: []byte{0x20, 0x1, 0x11, 0x11, 0x22, 0x22, 0x33, 0x33,
						0x44, 0x44, 0x55, 0x55, 0x66, 0x66, 0x77, 0x77},
				},
				{
					IP: []byte{192, 168, 1, 10},
				},
				{
					IP: []byte{1, 1, 1, 1},
				},
			},
		)

		ipv4, ipv6 := getIPsByIfName("eth0", netctrl)
		Expect(len(ipv4)).To(Equal(2))
		Expect(len(ipv6)).To(Equal(1))
		Expect(ipv4[0]).To(Equal("192.168.1.10"))
		Expect(ipv4[1]).To(Equal("1.1.1.1"))
		Expect(ipv6[0]).To(Equal("2001:1111:2222:3333:4444:5555:6666:7777"))
	})
	It("should return IPv6 address only if IPv4 is not available", func() {
		netctrl.EXPECT().GetInterfaceAddresses("eth0").Return(
			[]net.IPNet{
				{
					IP: []byte{0x20, 0x1, 0x11, 0x11, 0x22, 0x22, 0x33, 0x33,
						0x44, 0x44, 0x55, 0x55, 0x66, 0x66, 0x77, 0x77},
				},
			},
		)

		ipv4, ipv6 := getIPsByIfName("eth0", netctrl)
		Expect(ipv4).To(BeEmpty())
		Expect(len(ipv6)).To(Equal(1))
		Expect(ipv6[0]).To(Equal("2001:1111:2222:3333:4444:5555:6666:7777"))
	})
	It("should return the empty string when no IP addresses are available", func() {
		netctrl.EXPECT().GetInterfaceAddresses("eth0").Return([]net.IPNet{})
		ipv4, ipv6 := getIPsByIfName("eth0", netctrl)
		Expect(ipv4).To(BeEmpty())
		Expect(ipv6).To(BeEmpty())
	})
	It("should return the empty string if returned addresses are invalid", func() {
		netctrl.EXPECT().GetInterfaceAddresses("eth0").Return(
			[]net.IPNet{
				{
					IP: []byte{0x20, 0x1, 0x11, 0x11, 0x22, 0x22, 0x33, 0x33,
						0x44, 0x44},
				},
			},
		)
		ipv4, ipv6 := getIPsByIfName("eth0", netctrl)
		Expect(ipv4).To(BeEmpty())
		Expect(ipv6).To(BeEmpty())
	})
})
