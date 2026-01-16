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
)

type NetController interface {
	GetInterfaceAddresses(ifName string) [](net.IPNet)
}

func NewNetController() NetController {
	return &unixNet{}
}

var _ NetController = (*unixNet)(nil)

type unixNet struct{}

func (un *unixNet) GetInterfaceAddresses(ifName string) (res []net.IPNet) {
	var (
		iface *net.Interface
		addrs []net.Addr
		err   error
	)
	res = []net.IPNet{}

	if iface, err = net.InterfaceByName(ifName); err != nil {
		return
	}
	if addrs, err = iface.Addrs(); err != nil {
		return
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok {
			res = append(res, *ipNet)
		}
	}
	return
}

// GetIPByIfName returns two slices containing the IPv4 and the IPv6 addresses of
// the network interface name (ifName) passed as argument.
// This function cannot fail: if no address can be retrieved two empty slices are
// returned.
func GetIPsByIfName(ifName string) ([]string, []string) {
	return getIPsByIfName(ifName, NewNetController())
}

func getIPsByIfName(ifName string, nc NetController) (ipv4, ipv6 []string) {
	ipv4 = []string{}
	ipv6 = []string{}

	ipNets := nc.GetInterfaceAddresses(ifName)
	for _, ip := range ipNets {
		if ip2v4 := ip.IP.To4(); ip2v4 != nil {
			ipv4 = append(ipv4, ip2v4.String())
		} else if ip2v6 := ip.IP.To16(); ip2v6 != nil {
			ipv6 = append(ipv6, ip2v6.String())
		}
	}
	return
}
