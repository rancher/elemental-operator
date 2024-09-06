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
	return ExtractLabels(*systemData)
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

// Core function to translate hostdata (passed as a reflect.Value) to map[string]interface{} containing
// the Template Labels values
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

// ExtractFullData returns all the available hostinfo data without any check
// or post elaboration as a map[string]interface{}, where the interface{} is
// either a string or an embedded map[string]interface{}
func ExtractFullData(systemData HostInfo) (map[string]interface{}, error) {
	labels := reflectValToInterface(reflect.ValueOf(systemData)).(map[string]interface{})

	return labels, nil
}

// Some data from HostInfo are invalid: drop them
func sanitizeHostInfoVal(data string) string {
	if strings.HasPrefix(data, "Unknown!") {
		return ""
	}
	return data
}

func ExtractLabels(systemData HostInfo) (map[string]interface{}, error) {
	labels := map[string]interface{}{}

	// SMBIOS DATA
	bios := reflectValToInterface(reflect.ValueOf(systemData.BIOS)).(map[string]interface{})
	baseboard := reflectValToInterface(reflect.ValueOf(systemData.Baseboard)).(map[string]interface{})
	chassis := reflectValToInterface(reflect.ValueOf(systemData.Chassis)).(map[string]interface{})
	product := reflectValToInterface(reflect.ValueOf(systemData.Product)).(map[string]interface{})
	// Legacy SMBIOS names support
	if systemData.Product != nil {
		product["Serial Number"] = systemData.Product.SerialNumber
		product["Manufacturer"] = systemData.Product.Vendor
		product["SKU Number"] = systemData.Product.SKU
		product["Product Name"] = systemData.Product.Name
	}
	if systemData.Baseboard != nil {
		product["Serial Number"] = systemData.Baseboard.SerialNumber
	}
	if systemData.Chassis != nil {
		product["Serial Number"] = systemData.Chassis.SerialNumber
	}

	labels["System Information"] = product
	labels["BIOS Information"] = bios
	labels["Base Board Information"] = baseboard
	labels["Chassis Information"] = chassis

	// System Data
	memory := reflectValToInterface(reflect.ValueOf(systemData.Memory)).(map[string]interface{})
	// Legacy names (with spaces) support
	if systemData.Memory != nil {
		memory["Total Physical Bytes"] = strconv.Itoa(int(systemData.Memory.TotalPhysicalBytes))
		memory["Total Usable Bytes"] = strconv.Itoa(int(systemData.Memory.TotalUsableBytes))
	}

	// Both checks below are due to ghw not detecting aarch64 cores/threads properly, so it ends up in a label
	// with 0 value, which is not useful at all
	// tracking bug: https://github.com/jaypipes/ghw/issues/199
	cpu := map[string]interface{}{}
	if systemData.CPU != nil {
		cpu["Total Cores"] = strconv.Itoa(int(systemData.CPU.TotalCores))
		cpu["Total Threads"] = strconv.Itoa(int(systemData.CPU.TotalThreads))
		// This should never happen but just in case
		if len(systemData.CPU.Processors) > 0 {
			// Model still looks weird, maybe there is a way of getting it differently as we need to sanitize a lot of data in there?
			// Currently, something like "Intel(R) Core(TM) i7-7700K CPU @ 4.20GHz" ends up being:
			// "Intel-R-Core-TM-i7-7700K-CPU-4-20GHz"
			cpu["Number Processors"] = strconv.Itoa(len(systemData.CPU.Processors))
			cpu["Model"] = systemData.CPU.Processors[0].Model
			cpu["Vendor"] = systemData.CPU.Processors[0].Vendor
			cpu["Capabilities"] = strings.Join(systemData.CPU.Processors[0].Capabilities, ",")

			for i, c := range systemData.CPU.Processors {
				cpuNum := strconv.Itoa(i)
				cpu[cpuNum] = map[string]interface{}{
					"Model":        c.Model,
					"Vendor":       c.Vendor,
					"Capabilities": strings.Join(c.Capabilities, ","),
				}
			}
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
		// number of not-virtual cards found
		nicNum := 0
		network["Number Interfaces"] = strconv.Itoa(len(systemData.Network.NICs))
		for i, iface := range systemData.Network.NICs {
			ifaceNum := strconv.Itoa(i)
			network[ifaceNum] = map[string]interface{}{
				"Name":                iface.Name,
				"MacAddress":          iface.MacAddress,
				"IsVirtual":           strconv.FormatBool(iface.IsVirtual),
				"Duplex":              sanitizeHostInfoVal(iface.Duplex),
				"Speed":               sanitizeHostInfoVal(iface.Speed),
				"AdvertisedLinkModes": strings.Join(iface.AdvertisedLinkModes, ","),
				"SupportedLinkModes":  strings.Join(iface.SupportedLinkModes, ","),
				"SupportedPorts":      strings.Join(iface.SupportedPorts, ","),
			}
			network[iface.Name] = network[ifaceNum]
			if !iface.IsVirtual {
				network[fmt.Sprintf("NIC%d", nicNum)] = network[ifaceNum]
				nicNum++
			}
		}
	}

	block := map[string]interface{}{}
	if systemData.Block != nil {
		block["Number Devices"] = strconv.Itoa(len(systemData.Block.Disks)) // This includes removable devices like cdrom/usb
		for i, disk := range systemData.Block.Disks {
			blockNum := strconv.Itoa(i)
			block[blockNum] = map[string]interface{}{
				"Size":               strconv.Itoa(int(disk.SizeBytes)),
				"Name":               disk.Name,
				"Drive Type":         disk.DriveType.String(),
				"Storage Controller": disk.StorageController.String(),
				"Removable":          strconv.FormatBool(disk.IsRemovable),
				"Model":              disk.Model,
				// "Partitions":         reflectValToInterface(reflect.ValueOf(disk.Partitions)),
			}
			block[disk.Name] = block[blockNum]
		}
	}

	runtime := map[string]interface{}{}
	if systemData.Runtime != nil {
		runtime["Hostname"] = systemData.Runtime.Hostname
	}

	labels["System Data"] = map[string]interface{}{
		"Memory":        memory,
		"CPU":           cpu,
		"GPU":           gpu,
		"Network":       network,
		"Block Devices": block,
		"Runtime":       runtime,
	}
	// Also available but not used:
	// systemData.Topology -> CPU/memory and cache topology. No idea if useful.

	return labels, nil
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
