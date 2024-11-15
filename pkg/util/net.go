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
	"net"
)

// GetIPByIfName returns a string containing the IP address of the network
// interface name (ifName) passed as argument.
// The returned IP address is the IPv4 one but the IPv6 one is returned if
// the IPv4 address is not available.
// This function cannot fail: if no address can be retrieved an empty string
// is returned.
func GetIPByIfName(ifName string) string {
	var (
		iface   *net.Interface
		addrs   []net.Addr
		ip, ip6 net.IP
		err     error
	)

	if iface, err = net.InterfaceByName(ifName); err != nil {
		return ""
	}
	if addrs, err = iface.Addrs(); err != nil {
		return ""
	}
	for _, addr := range addrs {
		var (
			ipNet *net.IPNet
			ok    bool
		)

		if ipNet, ok = addr.(*net.IPNet); !ok {
			continue
		}

		if ip = ipNet.IP.To4(); ip != nil {
			return ip.String()
		}

		// Let's keep an IPv6 address as fallback if the interface has no IPv4 addresses
		if ip6 == nil {
			ip6 = ipNet.IP.To16()
		}
	}
	if ip6 != nil {
		return ip6.String()
	}
	return ""
}
