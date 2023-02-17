/*
Copyright © 2022 - 2023 SUSE LLC

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

package hostinfo

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/jaypipes/ghw"
	"github.com/jaypipes/ghw/pkg/baseboard"
	"github.com/jaypipes/ghw/pkg/bios"
	"github.com/jaypipes/ghw/pkg/block"
	"github.com/jaypipes/ghw/pkg/chassis"
	"github.com/jaypipes/ghw/pkg/context"
	"github.com/jaypipes/ghw/pkg/cpu"
	"github.com/jaypipes/ghw/pkg/gpu"
	"github.com/jaypipes/ghw/pkg/memory"
	"github.com/jaypipes/ghw/pkg/net"
	"github.com/jaypipes/ghw/pkg/product"
	"github.com/jaypipes/ghw/pkg/topology"
)

// HostInfo represents all the host info minus the PCI devices
// We cannot use ghw.HostInfo directly as that tries to gather the local pci-ids database, which we don't have
// And fallbacks to download it from the network. Unfortunately that returns an error instead of a warning and
// prevents us from gathering the rest of the data. Thus, the necessity of having our own struct :/
// We could drop this if we include the hwdata package in the base OS
type HostInfo struct {
	ctx       *context.Context
	Memory    *memory.Info    `json:"memory"`
	Block     *block.Info     `json:"block"`
	CPU       *cpu.Info       `json:"cpu"`
	Topology  *topology.Info  `json:"topology"`
	Network   *net.Info       `json:"network"`
	GPU       *gpu.Info       `json:"gpu"`
	Chassis   *chassis.Info   `json:"chassis"`
	BIOS      *bios.Info      `json:"bios"`
	Baseboard *baseboard.Info `json:"baseboard"`
	Product   *product.Info   `json:"product"`
}

// Host returns a pointer to a HostInfo struct that contains fields with
// information about the host system's CPU, memory, network devices, etc
func Host(opts ...*ghw.WithOption) (*HostInfo, error) {
	ctx := context.New(opts...)

	memInfo, err := memory.New(opts...)
	if err != nil {
		return nil, err
	}
	blockInfo, err := block.New(opts...)
	if err != nil {
		return nil, err
	}
	cpuInfo, err := cpu.New(opts...)
	if err != nil {
		return nil, err
	}
	topologyInfo, err := topology.New(opts...)
	if err != nil {
		return nil, err
	}
	netInfo, err := net.New(opts...)
	if err != nil {
		return nil, err
	}
	gpuInfo, err := gpu.New(opts...)
	if err != nil {
		return nil, err
	}
	chassisInfo, err := chassis.New(opts...)
	if err != nil {
		return nil, err
	}
	biosInfo, err := bios.New(opts...)
	if err != nil {
		return nil, err
	}
	baseboardInfo, err := baseboard.New(opts...)
	if err != nil {
		return nil, err
	}
	productInfo, err := product.New(opts...)
	if err != nil {
		return nil, err
	}
	return &HostInfo{
		ctx:       ctx,
		CPU:       cpuInfo,
		Memory:    memInfo,
		Block:     blockInfo,
		Topology:  topologyInfo,
		Network:   netInfo,
		GPU:       gpuInfo,
		Chassis:   chassisInfo,
		BIOS:      biosInfo,
		Baseboard: baseboardInfo,
		Product:   productInfo,
	}, nil
}

func FillData(data []byte) (map[string]interface{}, error) {
	systemData := &HostInfo{}
	if err := json.Unmarshal(data, &systemData); err != nil {
		return nil, err
	}

	memory := map[string]interface{}{}
	if systemData.Memory != nil {
		memory["Total Physical Bytes"] = strconv.Itoa(int(systemData.Memory.TotalPhysicalBytes))
	}

	// Both checks below are due to ghw not detecting aarch64 cores/threads properly, so it ends up in a label
	// with 0 value, which is not useful at all
	// tracking bug: https://github.com/jaypipes/ghw/issues/199
	cpu := map[string]interface{}{}
	if systemData.CPU != nil {
		if systemData.CPU.TotalCores > 0 {
			cpu["Total Cores"] = strconv.Itoa(int(systemData.CPU.TotalCores))
		}
		if systemData.CPU.TotalThreads > 0 {
			cpu["Total Threads"] = strconv.Itoa(int(systemData.CPU.TotalThreads))
		}

		// This should never happen but just in case
		if len(systemData.CPU.Processors) > 0 {
			// Model still looks weird, maybe there is a way of getting it differently as we need to sanitize a lot of data in there?
			// Currently, something like "Intel(R) Core(TM) i7-7700K CPU @ 4.20GHz" ends up being:
			// "Intel-R-Core-TM-i7-7700K-CPU-4-20GHz"
			cpu["Model"] = sanitizeString(systemData.CPU.Processors[0].Model)
			cpu["Vendor"] = sanitizeString(systemData.CPU.Processors[0].Vendor)
			// Capabilities available here at systemData.CPU.Processors[X].Capabilities
		}
	}

	gpu := map[string]interface{}{}
	// This could happen so always check.
	if systemData.GPU != nil && len(systemData.GPU.GraphicsCards) > 0 && systemData.GPU.GraphicsCards[0].DeviceInfo != nil {
		gpu["Model"] = sanitizeString(systemData.GPU.GraphicsCards[0].DeviceInfo.Product.Name)
		gpu["Vendor"] = sanitizeString(systemData.GPU.GraphicsCards[0].DeviceInfo.Vendor.Name)
	}

	network := map[string]interface{}{}
	if systemData.Network != nil {
		network["Number Interfaces"] = strconv.Itoa(len(systemData.Network.NICs))
		for _, iface := range systemData.Network.NICs {
			network[iface.Name] = map[string]interface{}{
				"Name":      iface.Name,
				"IsVirtual": strconv.FormatBool(iface.IsVirtual),
				// Capabilities available here at iface.Capabilities
				// interesting to store anything in here or show it on the docs? Difficult to use it afterwards as its a list...
			}
		}
	}

	block := map[string]interface{}{}
	if systemData.Block != nil {
		block["Number Devices"] = strconv.Itoa(len(systemData.Block.Disks)) // This includes removable devices like cdrom/usb
		for _, disk := range systemData.Block.Disks {
			block[disk.Name] = map[string]interface{}{
				"Size":               strconv.Itoa(int(disk.SizeBytes)),
				"Name":               disk.Name,
				"Drive Type":         disk.DriveType.String(),
				"Storage Controller": disk.StorageController.String(),
				"Removable":          strconv.FormatBool(disk.IsRemovable),
			}
			// Vendor and model also available here, useful?
		}
	}

	labels := map[string]interface{}{}
	labels["System Data"] = map[string]interface{}{
		"Memory":        memory,
		"CPU":           cpu,
		"GPU":           gpu,
		"Network":       network,
		"Block Devices": block,
	}

	// Also available but not used:
	// systemData.Product -> name, vendor, serial,uuid,sku,version. Kind of smbios data
	// systemData.BIOS -> info about the bios. Useless IMO
	// systemData.Baseboard -> asset, serial, vendor,version,product. Kind of useless?
	// systemData.Chassis -> asset, serial, vendor,version,product, type. Maybe be useful depending on the provider.
	// systemData.Topology -> CPU/memory and cache topology. No idea if useful.

	return labels, nil
}

// sanitizeString will sanitize a given string by:
// replacing all invalid chars as set on the sanitize regex by dashes
// removing any double dashes resulted from the above method
// removing prefix+suffix if they are a dash
func sanitizeString(s string) string {
	sanitize := regexp.MustCompile("[^0-9a-zA-Z_]")
	doubleDash := regexp.MustCompile("--+")

	s1 := sanitize.ReplaceAllString(s, "-")
	s2 := doubleDash.ReplaceAllString(s1, "-")
	return strings.TrimSuffix(strings.TrimPrefix(s2, "-"), "-")
}
