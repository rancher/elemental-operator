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

package hostinfo

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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

	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/runtime"
	"github.com/rancher/elemental-operator/pkg/util"
)

var ErrCouldNotReadHostInfo = errors.New("could not read host info")

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
	Runtime   *runtime.Info   `json:"runtime"`
}

// Host returns a HostInfo struct that contains fields with
// information about the host system's CPU, memory, network devices, etc
func Host() (HostInfo, error) {
	return host(ghw.WithDisableWarnings())
}

func host(opts ...*ghw.WithOption) (HostInfo, error) {
	hostInfoCollectionError := false
	var err error
	ctx := context.New(opts...)
	hostInfo := HostInfo{ctx: ctx}

	if hostInfo.Memory, err = memory.New(opts...); err != nil {
		log.Errorf("Could not collect Memory data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfo.Block, err = block.New(opts...); err != nil {
		log.Errorf("Could not collect Block storage data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfo.CPU, err = cpu.New(opts...); err != nil {
		log.Errorf("Could not collect CPU data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfo.Topology, err = topology.New(opts...); err != nil {
		log.Errorf("Could not collect Topology data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfo.Network, err = net.New(opts...); err != nil {
		log.Errorf("Could not collect Network data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfo.GPU, err = gpu.New(opts...); err != nil {
		log.Errorf("Could not collect GPU data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfo.Chassis, err = chassis.New(opts...); err != nil {
		log.Errorf("Could not collect Chassis data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfo.BIOS, err = bios.New(opts...); err != nil {
		log.Errorf("Could not collect BIOS data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfo.Baseboard, err = baseboard.New(opts...); err != nil {
		log.Errorf("Could not collect Base board data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfo.Product, err = product.New(opts...); err != nil {
		log.Errorf("Could not collect Product data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfo.Runtime, err = runtime.New(); err != nil {
		log.Errorf("Could not collect Runtime data: %s", err.Error())
		hostInfoCollectionError = true
	}

	if hostInfoCollectionError {
		return hostInfo, ErrCouldNotReadHostInfo
	}

	return hostInfo, nil
}

// Deprecated. Remove me together with 'MsgSystemData' type.
func FillData(data []byte) (map[string]interface{}, error) {
	// Also available but not used:
	// systemData.Product -> name, vendor, serial,uuid,sku,version. Kind of smbios data
	// systemData.BIOS -> info about the bios. Useless IMO
	// systemData.Baseboard -> asset, serial, vendor,version,product. Kind of useless?
	// systemData.Chassis -> asset, serial, vendor,version,product, type. Maybe be useful depending on the provider.
	// systemData.Topology -> CPU/memory and cache topology. No idea if useful.

	systemData := &HostInfo{}
	if err := json.Unmarshal(data, &systemData); err != nil {
		return nil, fmt.Errorf("unmarshalling system data payload: %w", err)
	}
	return ExtractLabelsLegacy(*systemData), nil
}

func isBaseReflectKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Bool, reflect.String, reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// reflectValToInterface() converts the passed value to either a string or to a map[string]interface{},
// where the interface{} part could be either a string or an embedded map[string]interface{}.
// This is the core function to translate hostdata (passed as a reflect.Value) to map[string]interface{}
// containing the Template Labels values.
func reflectValToInterface(v reflect.Value) interface{} {
	mapData := map[string]interface{}{}

	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			// Skip exported fields
			if !v.Type().Field(i).IsExported() {
				continue
			}
			// Skip also disabled fields from json serialization.
			// This is needed to avoid loops, like the one in systemData.Block (Disks and Partitions refer to
			// each other)... :-/
			if v.Type().Field(i).Tag == "json:\"-\"" {
				continue
			}

			fieldName := v.Type().Field(i).Name
			fieldVal := v.Field(i)

			if !fieldVal.IsValid() {
				continue
			}
			if v.Type().Field(i).Anonymous {
				return reflectValToInterface(fieldVal)
			}

			mapData[fieldName] = reflectValToInterface(fieldVal)
		}

	case reflect.Pointer:
		if v.IsNil() {
			return ""
		}
		return reflectValToInterface(v.Elem())

	case reflect.Slice:
		if v.Len() == 0 {
			return ""
		}

		if isBaseReflectKind(v.Index(0).Kind()) {
			txt := fmt.Sprintf("%v", v.Index(0))
			for j := 1; j < v.Len(); j++ {
				txt += fmt.Sprintf(", %v", v.Index(j))
			}
			return txt
		}

		for k := 0; k < v.Len(); k++ {
			mapData[strconv.Itoa(k)] = reflectValToInterface(v.Index(k))
		}

	default:
		return fmt.Sprintf("%v", v)
	}
	return mapData
}

// dataToLabelMap ensures the value returned by reflectValToInterface() is a map,
// otherwise returns an empty map (mainly to filter out empty vals returned as empty
// strings)
func dataToLabelMap(val interface{}) map[string]interface{} {
	refVal := reflectValToInterface(reflect.ValueOf(val))
	if labelMap, ok := refVal.(map[string]interface{}); ok {
		return labelMap
	}
	return map[string]interface{}{}
}

// ExtractFullData returns all the available hostinfo data without any check
// or post elaboration as a map[string]interface{}, where the interface{} is
// either a string or an embedded map[string]interface{}
func ExtractFullData(systemData HostInfo) map[string]interface{} {
	labels := dataToLabelMap(systemData)

	return labels
}

// Some data from HostInfo are invalid: drop them
func sanitizeHostInfoVal(data string) string {
	if strings.HasPrefix(data, "Unknown!") {
		return ""
	}
	return data
}

// ExtractLabelsLegacy provides the old (<= 1.7.0) syntax of Label Templates for backward compatibility
func ExtractLabelsLegacy(systemData HostInfo) map[string]interface{} {
	memory := map[string]interface{}{}
	if systemData.Memory != nil {
		memory["Total Physical Bytes"] = strconv.Itoa(int(systemData.Memory.TotalPhysicalBytes))
		memory["Total Usable Bytes"] = strconv.Itoa(int(systemData.Memory.TotalUsableBytes))
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
			cpu["Model"] = systemData.CPU.Processors[0].Model
			cpu["Vendor"] = systemData.CPU.Processors[0].Vendor
			cpu["Capabilities"] = systemData.CPU.Processors[0].Capabilities
		}
	}

	gpu := map[string]interface{}{}
	// This could happen so always check.
	if systemData.GPU != nil && len(systemData.GPU.GraphicsCards) > 0 && systemData.GPU.GraphicsCards[0].DeviceInfo != nil {
		gpu["Model"] = systemData.GPU.GraphicsCards[0].DeviceInfo.Product.Name
		gpu["Vendor"] = systemData.GPU.GraphicsCards[0].DeviceInfo.Vendor.Name
	}

	network := map[string]interface{}{}
	if systemData.Network != nil {
		network["Number Interfaces"] = strconv.Itoa(len(systemData.Network.NICs))
		for _, iface := range systemData.Network.NICs {
			network[iface.Name] = map[string]interface{}{
				"Name":       iface.Name,
				"MacAddress": iface.MacAddress,
				"IsVirtual":  strconv.FormatBool(iface.IsVirtual),
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

	runtime := map[string]interface{}{}
	if systemData.Runtime != nil {
		runtime["Hostname"] = systemData.Runtime.Hostname
	}

	labels := map[string]interface{}{}
	labels["System Data"] = map[string]interface{}{
		"Memory":        memory,
		"CPU":           cpu,
		"GPU":           gpu,
		"Network":       network,
		"Block Devices": block,
		"Runtime":       runtime,
	}

	// Also available but not used:
	// systemData.Product -> name, vendor, serial,uuid,sku,version. Kind of smbios data
	// systemData.BIOS -> info about the bios. Useless IMO
	// systemData.Baseboard -> asset, serial, vendor,version,product. Kind of useless?
	// systemData.Chassis -> asset, serial, vendor,version,product, type. Maybe be useful depending on the provider.
	// systemData.Topology -> CPU/memory and cache topology. No idea if useful.

	return labels
}

// ExtractLabels provide the new (>= 1.8.x) syntax of the Label Templates
func ExtractLabels(systemData HostInfo) map[string]interface{} {
	labels := map[string]interface{}{}

	// SMBIOS DATA
	bios := dataToLabelMap(systemData.BIOS)
	baseboard := dataToLabelMap(systemData.Baseboard)
	chassis := dataToLabelMap(systemData.Chassis)
	product := dataToLabelMap(systemData.Product)
	memory := dataToLabelMap(systemData.Memory)

	// CPU raw data includes extended Cores info, pick all the other values
	cpu := map[string]interface{}{}
	if systemData.CPU != nil {
		if len(systemData.CPU.Processors) > 0 {
			cpuProcsMap := map[string]interface{}{}
			cpu["Processors"] = cpuProcsMap
			for i, processor := range systemData.CPU.Processors {
				cpuProcsMap[strconv.Itoa(i)] = map[string]interface{}{
					"Capabilities": strings.Join(processor.Capabilities, ","),
					"ID":           strconv.Itoa(processor.ID),
					"Model":        processor.Model,
					"NumCores":     strconv.Itoa((int(processor.NumCores))),
					"NumThreads":   strconv.Itoa((int(processor.NumThreads))),
					"Vendor":       processor.Vendor,
				}
			}
			// Handy label for single processor machines (e.g., ${CPU/Processor/Model} vs ${CPU/Processors/0/Model})
			cpu["Processor"] = cpuProcsMap["0"]
		}
	}
	cpu["TotalCores"] = strconv.Itoa(int(systemData.CPU.TotalCores))
	cpu["TotalThreads"] = strconv.Itoa(int(systemData.CPU.TotalThreads))
	cpu["TotalProcessors"] = strconv.Itoa(len(systemData.CPU.Processors))

	// GPU raw data could be huge, just pick few interesting values.
	gpu := map[string]interface{}{}
	if systemData.GPU != nil {
		if len(systemData.GPU.GraphicsCards) > 0 {
			cardNum := 0
			graphCrdsMap := map[string]interface{}{}
			gpu["GraphicsCards"] = graphCrdsMap
			for _, card := range systemData.GPU.GraphicsCards {
				if card.DeviceInfo != nil {
					cardNumMap := map[string]interface{}{
						"Driver": card.DeviceInfo.Driver,
					}
					graphCrdsMap[strconv.Itoa(cardNum)] = cardNumMap

					if card.DeviceInfo.Product != nil {
						cardNumMap["ProductName"] = card.DeviceInfo.Product.Name
					}
					if card.DeviceInfo.Vendor != nil {
						cardNumMap["VendorName"] = card.DeviceInfo.Vendor.Name
					}
					// Handy label for single GPU machines
					// (e.g., ${GPU/Driver} vs ${GPU/GraphicsCards/0/Driver})
					if _, ok := graphCrdsMap["Driver"]; !ok {
						for key, val := range cardNumMap {
							graphCrdsMap[key] = val
						}
					}
					cardNum++
				}
			}
			gpu["TotalCards"] = strconv.Itoa(cardNum)
		}
	}

	// NICs data have capabilities as array items, we don't want them, just pick few values.
	network := map[string]interface{}{}
	if systemData.Network != nil {
		network["TotalNICs"] = strconv.Itoa(len(systemData.Network.NICs))

		nicsMap := map[string]interface{}{}
		network["NICs"] = nicsMap
		for i, iface := range systemData.Network.NICs {
			ifaceNum := strconv.Itoa(i)
			ipv4, ipv6 := util.GetIPsByIfName(iface.Name)
			ipv4add := ""
			ipv4map := map[string]interface{}{}
			for i, ip := range ipv4 {
				ipv4map[strconv.Itoa(i)] = ip
				if ipv4add == "" {
					ipv4add = ip
				}
			}
			ipv6add := ""
			ipv6map := map[string]interface{}{}
			for i, ip := range ipv6 {
				ipv6map[strconv.Itoa(i)] = ip
				if ipv6add == "" {
					ipv6add = ip
				}
			}
			nicsMap[ifaceNum] = map[string]interface{}{
				"AdvertisedLinkModes": strings.Join(iface.AdvertisedLinkModes, ","),
				"Duplex":              sanitizeHostInfoVal(iface.Duplex),
				"IsVirtual":           strconv.FormatBool(iface.IsVirtual),
				"IPv4Address":         ipv4add,
				"IPv4Addresses":       ipv4map,
				"IPv6Address":         ipv6add,
				"IPv6Addresses":       ipv6map,
				"MacAddress":          iface.MacAddress,
				"Name":                iface.Name,
				"PCIAddress":          iface.PCIAddress,
				"Speed":               sanitizeHostInfoVal(iface.Speed),
				"SupportedLinkModes":  strings.Join(iface.SupportedLinkModes, ","),
				"SupportedPorts":      strings.Join(iface.SupportedPorts, ","),
			}
			// handy reference by interface name
			// (e.g., "${Network/NICs/eth0/MacAddress} vs "${Network/NICs/0/MacAddress})
			nicsMap[iface.Name] = nicsMap[ifaceNum]
		}
	}

	// Block data carry extended partitions information for each Disk we don't want.
	// Manually pick the other items.
	block := map[string]interface{}{}
	if systemData.Block != nil {
		block["TotalDisks"] = strconv.Itoa(len(systemData.Block.Disks)) // This includes removable devices like cdrom/usb

		disksMap := map[string]interface{}{}
		block["Disks"] = disksMap
		for i, disk := range systemData.Block.Disks {
			blockNum := strconv.Itoa(i)
			disksMap[blockNum] = map[string]interface{}{
				"Size":              strconv.Itoa(int(disk.SizeBytes)),
				"Name":              disk.Name,
				"DriveType":         disk.DriveType.String(),
				"StorageController": disk.StorageController.String(),
				"Removable":         strconv.FormatBool(disk.IsRemovable),
				"Model":             disk.Model,
				// "Partitions":         reflectValToInterface(reflect.ValueOf(disk.Partitions)),
			}
			// handy reference by disk name
			// (e.g., "${Block/Disks/1/Model} vs "${Block/Disks/sda/Model}"")
			disksMap[disk.Name] = disksMap[blockNum]
		}
	}

	runtime := dataToLabelMap(systemData.Runtime)

	labels["Product"] = product
	labels["BIOS"] = bios
	labels["BaseBoard"] = baseboard
	labels["Chassis"] = chassis
	labels["Memory"] = memory
	labels["CPU"] = cpu
	labels["GPU"] = gpu
	labels["Network"] = network
	labels["Storage"] = block
	labels["Runtime"] = runtime

	// Also available but not used:
	// systemData.Topology -> CPU/memory and cache topology. No idea if useful.

	return labels
}

// Deprecated. Remove me together with 'MsgSystemData' type.
// Prune() filters out new Disks and Controllers introduced in ghw/pkg/block > 0.9.0
// see: https://github.com/rancher/elemental-operator/issues/733
func Prune(data *HostInfo) {
	prunedDisks := []*block.Disk{}
	for i := 0; i < len(data.Block.Disks); i++ {
		if data.Block.Disks[i].DriveType > block.DRIVE_TYPE_SSD {
			continue
		}
		if data.Block.Disks[i].StorageController > block.STORAGE_CONTROLLER_MMC {
			continue
		}
		prunedDisks = append(prunedDisks, data.Block.Disks[i])
	}
	data.Block.Disks = prunedDisks
}
