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

package util

import (
	"bytes"
	"net"
	"os/exec"
)

type NetController interface {
	GetInterfaceAddresses(ifName string) [](net.IPNet)
	GetNMDeviceState() (*bytes.Buffer, error)
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

func (un *unixNet) GetNMDeviceState() (buf *bytes.Buffer, err error) {
	buf = &bytes.Buffer{}
	cmd := exec.Command("nmcli", "-g", "DEVICE,STATE", "device")
	cmd.Stdout = buf

	err = cmd.Run()

	return
}

// GetIPByIfName returns a string containing the IP address of the network
// interface name (ifName) passed as argument.
// The returned IP address is the IPv4 one but the IPv6 one is returned if
// the IPv4 address is not available.
// This function cannot fail: if no address can be retrieved an empty string
// is returned.
func GetIPByIfName(ifName string) string {
	return getIPByIfName(ifName, NewNetController())
}

func getIPByIfName(ifName string, nc NetController) string {
	var (
		ips  []net.IPNet
		ipv6 net.IP
	)
	if ips = nc.GetInterfaceAddresses(ifName); len(ips) == 0 {
		return ""
	}
	for _, ip := range ips {
		if ipv4 := ip.IP.To4(); ipv4 != nil {
			return ipv4.String()
		}

		// Let's keep an IPv6 address as fallback if the interface has no IPv4 addresses
		if ipv6 == nil {
			ipv6 = ip.IP.To16()
		}
	}
	if ipv6 != nil {
		return ipv6.String()
	}
	return ""
}
