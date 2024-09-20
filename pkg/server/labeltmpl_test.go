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

package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaypipes/ghw/pkg/block"
	"github.com/jaypipes/ghw/pkg/cpu"
	"github.com/jaypipes/ghw/pkg/memory"
	"github.com/jaypipes/ghw/pkg/net"
	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/hostinfo"
	elementalruntime "github.com/rancher/elemental-operator/pkg/runtime"
	"github.com/rancher/elemental-operator/pkg/templater"
	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/util/validation"
)

var (
	systemDataLabelsRegistrationFixture = &elementalv1.MachineRegistration{
		Spec: elementalv1.MachineRegistrationSpec{
			MachineInventoryLabels: map[string]string{
				"elemental.cattle.io/Hostname":               "${System Data/Runtime/Hostname}",
				"elemental.cattle.io/TotalMemory":            "${System Data/Memory/Total Physical Bytes}",
				"elemental.cattle.io/AvailableMemory":        "${System Data/Memory/Total Usable Bytes}",
				"elemental.cattle.io/CpuTotalCores":          "${System Data/CPU/Total Cores}",
				"elemental.cattle.io/CpuTotalThreads":        "${System Data/CPU/Total Threads}",
				"elemental.cattle.io/NetIfacesNumber":        "${System Data/Network/Number Interfaces}",
				"elemental.cattle.io/NetIface0-Name":         "${System Data/Network/myNic1/Name}",
				"elemental.cattle.io/NetIface0-MAC":          "${System Data/Network/myNic1/MacAddress}",
				"elemental.cattle.io/NetIface0-IsVirtual":    "${System Data/Network/myNic1/IsVirtual}",
				"elemental.cattle.io/NetIface1-Name":         "${System Data/Network/myNic2/Name}",
				"elemental.cattle.io/BlockDevicesNumber":     "${System Data/Block Devices/Number Devices}",
				"elemental.cattle.io/BlockDevice0-Name":      "${System Data/Block Devices/testdisk1/Name}",
				"elemental.cattle.io/BlockDevice1-Name":      "${System Data/Block Devices/testdisk2/Name}",
				"elemental.cattle.io/BlockDevice0-Size":      "${System Data/Block Devices/testdisk1/Size}",
				"elemental.cattle.io/BlockDevice1-Size":      "${System Data/Block Devices/testdisk2/Size}",
				"elemental.cattle.io/BlockDevice0-Removable": "${System Data/Block Devices/testdisk1/Removable}",
				"elemental.cattle.io/BlockDevice1-Removable": "${System Data/Block Devices/testdisk2/Removable}",
			},
		},
	}
	hostinfoDataLabelsRegistrationFixture = &elementalv1.MachineRegistration{
		Spec: elementalv1.MachineRegistrationSpec{
			MachineInventoryLabels: map[string]string{
				"elemental.cattle.io/Hostname":               "${Runtime/Hostname}",
				"elemental.cattle.io/TotalMemory":            "${Memory/TotalPhysicalBytes}",
				"elemental.cattle.io/AvailableMemory":        "${Memory/TotalUsableBytes}",
				"elemental.cattle.io/CpuTotalCores":          "${CPU/TotalCores}",
				"elemental.cattle.io/CpuTotalThreads":        "${CPU/TotalThreads}",
				"elemental.cattle.io/NetIfacesNumber":        "${Network/TotalNICs}",
				"elemental.cattle.io/NetIface0-Name":         "${Network/NICs/myNic1/Name}",
				"elemental.cattle.io/NetIface0-MAC":          "${Network/NICs/myNic1/MacAddress}",
				"elemental.cattle.io/NetIface0-IsVirtual":    "${Network/NICs/myNic1/IsVirtual}",
				"elemental.cattle.io/NetIface1-Name":         "${Network/NICs/myNic2/Name}",
				"elemental.cattle.io/BlockDevicesNumber":     "${Storage/TotalDisks}",
				"elemental.cattle.io/BlockDevice0-Name":      "${Storage/Disks/testdisk1/Name}",
				"elemental.cattle.io/BlockDevice1-Name":      "${Storage/Disks/testdisk2/Name}",
				"elemental.cattle.io/BlockDevice0-Size":      "${Storage/Disks/testdisk1/Size}",
				"elemental.cattle.io/BlockDevice1-Size":      "${Storage/Disks/testdisk2/Size}",
				"elemental.cattle.io/BlockDevice0-Removable": "${Storage/Disks/testdisk1/Removable}",
				"elemental.cattle.io/BlockDevice1-Removable": "${Storage/Disks/testdisk2/Removable}",
			},
		},
	}
	hostInfoFixture = hostinfo.HostInfo{
		Block: &block.Info{
			Disks: []*block.Disk{
				{
					Name:        "testdisk1",
					SizeBytes:   300,
					IsRemovable: true,
				},
				{
					Name:        "testdisk2",
					SizeBytes:   600,
					IsRemovable: false,
				},
			},
			Partitions: nil,
		},
		Memory: &memory.Info{
			Area: memory.Area{
				TotalPhysicalBytes: 100,
				TotalUsableBytes:   90,
			},
		},
		CPU: &cpu.Info{
			TotalCores:   300,
			TotalThreads: 300,
			Processors: []*cpu.Processor{
				{
					Vendor: "-this_is@broken?TM-][{¬{$h4yh46Ŋ£$⅝ŋg46¬~{~←ħ¬",
					Model:  "-this_is@broken?TM-][{¬{$h4yh46Ŋ£$⅝ŋg46¬~{~←ħ¬",
				},
			},
		},
		Network: &net.Info{
			NICs: []*net.NIC{
				{
					Name:       "myNic1",
					MacAddress: "02:00:00:00:00:01",
					IsVirtual:  true,
				},
				{
					Name: "myNic2",
				},
			},
		},
		Runtime: &elementalruntime.Info{
			Hostname: "machine-1",
		},
	}
)

func TestUpdateInventoryFromSystemData(t *testing.T) {
	inventory := &elementalv1.MachineInventory{}
	tmpl := templater.NewTemplater()

	encodedData, err := json.Marshal(hostInfoFixture)
	assert.NilError(t, err)

	systemData, err := hostinfo.FillData(encodedData)
	assert.NilError(t, err)

	tmpl.Fill(systemData)
	err = updateInventoryLabels(tmpl, inventory, systemDataLabelsRegistrationFixture)
	assert.NilError(t, err)
	assertSystemDataLabels(t, inventory)
}

func TestUpdateInventoryFromSystemDataNG(t *testing.T) {
	inventory := &elementalv1.MachineInventory{}
	tmpl := templater.NewTemplater()

	data := hostinfo.ExtractLabelsLegacy(hostInfoFixture)
	encodedData, err := json.Marshal(data)
	assert.NilError(t, err)

	systemData := map[string]interface{}{}
	err = json.Unmarshal(encodedData, &systemData)
	assert.NilError(t, err)

	tmpl.Fill(systemData)
	err = updateInventoryLabels(tmpl, inventory, systemDataLabelsRegistrationFixture)
	assert.NilError(t, err)
	assertSystemDataLabels(t, inventory)
}

func TestUpdateInventoryFromHostinfoData(t *testing.T) {
	inventory := &elementalv1.MachineInventory{}
	tmpl := templater.NewTemplater()

	data := hostinfo.ExtractLabels(hostInfoFixture)
	encodedData, err := json.Marshal(data)
	assert.NilError(t, err)

	hostinfoData := map[string]interface{}{}
	err = json.Unmarshal(encodedData, &hostinfoData)
	assert.NilError(t, err)
	tmpl.Fill(hostinfoData)
	err = updateInventoryLabels(tmpl, inventory, hostinfoDataLabelsRegistrationFixture)
	assert.NilError(t, err)
	assertSystemDataLabels(t, inventory)
}

// Check that the labels we properly added to the inventory
func assertSystemDataLabels(t *testing.T, inventory *elementalv1.MachineInventory) {
	t.Helper()
	assert.Equal(t, inventory.Labels["elemental.cattle.io/Hostname"], "machine-1")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/TotalMemory"], "100")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/TotalMemory"], "100")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/CpuTotalCores"], "300")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/CpuTotalThreads"], "300")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/NetIfacesNumber"], "2")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/NetIface0-Name"], "myNic1")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/NetIface0-MAC"], "02-00-00-00-00-01")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/NetIface0-IsVirtual"], "true")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/NetIface1-Name"], "myNic2")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevicesNumber"], "2")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice0-Name"], "testdisk1")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice1-Name"], "testdisk2")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice0-Size"], "300")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice1-Size"], "600")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice0-Removable"], "true")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice1-Removable"], "false")
}

func TestUpdateInventoryLabels(t *testing.T) {
	inventory := &elementalv1.MachineInventory{}
	inventory.Name = "${System Data/Runtime/Hostname}"
	inventory.Labels = map[string]string{
		"elemental.cattle.io/Random01": "alreadyFilled",
		"elemental.cattle.io/Random02": "",
	}

	registration := &elementalv1.MachineRegistration{
		Spec: elementalv1.MachineRegistrationSpec{
			MachineInventoryLabels: map[string]string{
				"elemental.cattle.io/TotalMemory":            "${System Data/Memory/Total Physical Bytes}",
				"elemental.cattle.io/CpuTotalCores":          "${System Data/CPU/Total Cores}",
				"elemental.cattle.io/CpuTotalThreads":        "${System Data/CPU/Total Threads}",
				"elemental.cattle.io/NetIfacesNumber":        "${System Data/Network/Number Interfaces}",
				"elemental.cattle.io/NetIface0-Name":         "${System Data/Network/myNic1/Name}",
				"elemental.cattle.io/NetIface0-IsVirtual":    "${System Data/Network/myNic1/IsVirtual}",
				"elemental.cattle.io/NetIface1-Name":         "${System Data/Network/myNic2/Name}",
				"elemental.cattle.io/BlockDevicesNumber":     "${System Data/Block Devices/Number Devices}",
				"elemental.cattle.io/BlockDevice0-Name":      "${System Data/Block Devices/testdisk1/Name}",
				"elemental.cattle.io/BlockDevice1-Name":      "${System Data/Block Devices/testdisk2/Name}",
				"elemental.cattle.io/BlockDevice0-Size":      "${System Data/Block Devices/testdisk1/Size}",
				"elemental.cattle.io/BlockDevice1-Size":      "${System Data/Block Devices/testdisk2/Size}",
				"elemental.cattle.io/BlockDevice0-Removable": "${System Data/Block Devices/testdisk1/Removable}",
				"elemental.cattle.io/BlockDevice1-Removable": "${System Data/Block Devices/testdisk2/Removable}",
				"elemental.cattle.io/UnexistingTemplate":     "${System Data/Not Existing Value}",
				"elemental.cattle.io/Random01":               "my-random-value-${Random/Int/1000}",
				"elemental.cattle.io/Random02":               "${Random/UUID}",
				"elemental.cattle.io/Random03":               "my-random-value-${Random/Hex/5}",
			},
		},
	}
	data := hostInfoFixture
	encodedData, err := json.Marshal(data)
	assert.NilError(t, err)
	systemData, err := hostinfo.FillData(encodedData)
	assert.NilError(t, err)
	tmpl := templater.NewTemplater()
	tmpl.Fill(systemData)
	err = updateInventoryName(tmpl, inventory)
	assert.NilError(t, err)
	err = updateInventoryLabels(tmpl, inventory, registration)
	assert.NilError(t, err)
	// Check that the labels were properly added to the inventory
	assert.Equal(t, inventory.Name, "machine-1")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/TotalMemory"], "100")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/CpuTotalCores"], "300")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/CpuTotalThreads"], "300")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/NetIfacesNumber"], "2")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/NetIface0-Name"], "myNic1")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/NetIface1-Name"], "myNic2")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevicesNumber"], "2")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice0-Name"], "testdisk1")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice1-Name"], "testdisk2")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice0-Size"], "300")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice1-Size"], "600")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice0-Removable"], "true")
	assert.Equal(t, inventory.Labels["elemental.cattle.io/BlockDevice1-Removable"], "false")
	// Check values were sanitized
	assert.Equal(t, len(validation.IsValidLabelValue(inventory.Labels["elemental.cattle.io/CpuModel"])), 0)
	assert.Equal(t, len(validation.IsValidLabelValue(inventory.Labels["elemental.cattle.io/CpuVendor"])), 0)
	// Check Random label templates
	assert.Equal(t, inventory.Labels["elemental.cattle.io/Random01"], "alreadyFilled")
	assert.Equal(t, len(inventory.Labels["elemental.cattle.io/Random02"]), len("e511d5ca-a765-42f2-82b7-264f37ffb329"))
	assert.Equal(t, len(inventory.Labels["elemental.cattle.io/Random03"]), 5+len("my-random-value-"))
}

func TestUpdateInventoryName(t *testing.T) {
	tmplData := map[string]interface{}{
		"Template Data": map[string]interface{}{
			"Data 1":     "value1",
			"Data 2":     "value2",
			"Empty data": "",
			"Bad data":   0,
		},
	}
	tmpl := templater.NewTemplater()
	tmpl.Fill(tmplData)

	testCases := []struct {
		invName  string // initial Inventory.Name
		expName  string // expected Inventory.Name
		isOldInv bool   // the inventory was created previously (do not update the name)
		isError  bool   // the inventory name update will error out
		isFallbk bool   // the inventory name will be set to the fallback (UUID generated)
	}{
		{
			invName: "machine-${Template Data/Data 1}",
			expName: "machine-value1",
		},
		{
			invName:  "don't Touch if not new ${Template Data/Data 2}",
			expName:  "don't Touch if not new ${Template Data/Data 2}",
			isOldInv: true,
		},
		{
			invName: "machine-${Template Data/Empty data}",
			expName: "machine",
		},
		{
			invName:  "machine-${Template Data/Dont Exists}",
			isFallbk: true,
		},
		{
			invName:  "$machine-${Template Data/Bad data}",
			isFallbk: true,
		},
		{
			invName: "${Template Data/Empty data}.-",
			isError: true,
		},
	}

	for _, tc := range testCases {
		inv := &elementalv1.MachineInventory{}
		inv.Name = tc.invName
		if tc.isOldInv {
			inv.CreationTimestamp.Time = time.Now()
		}
		err := updateInventoryName(tmpl, inv)
		if tc.isError {
			assert.Assert(t, err != nil)
			continue
		}
		if tc.isFallbk {
			// Fallback is a m-UUID generated name
			assert.Equal(t, inv.Name[:2], "m-")
			_, err = uuid.Parse(inv.Name[2:])
			assert.NilError(t, err)
			continue
		}
		assert.Equal(t, inv.Name, tc.expName)
	}
}

func TestUpdateInventoryAnnotations(t *testing.T) {
	tmpl := templater.NewTemplater()
	inventory := &elementalv1.MachineInventory{}
	inventory.Annotations = map[string]string{
		"elemental.cattle.io/Random01": "alreadyFilled",
		"elemental.cattle.io/Random02": "",
	}

	registration := &elementalv1.MachineRegistration{
		Spec: elementalv1.MachineRegistrationSpec{
			MachineInventoryAnnotations: map[string]string{
				"elemental.cattle.io/Hostname":               "${Runtime/Hostname}",
				"elemental.cattle.io/TotalMemory":            "${Memory/TotalPhysicalBytes}",
				"elemental.cattle.io/AvailableMemory":        "${Memory/TotalUsableBytes}",
				"elemental.cattle.io/CpuTotalCores":          "${CPU/TotalCores}",
				"elemental.cattle.io/CpuTotalThreads":        "${CPU/TotalThreads}",
				"elemental.cattle.io/ProcessorModel":         "${CPU/Processor/Model}",
				"elemental.cattle.io/NetIfacesNumber":        "${Network/TotalNICs}",
				"elemental.cattle.io/NetIface0-Name":         "${Network/NICs/myNic1/Name}",
				"elemental.cattle.io/NetIface0-MAC":          "${Network/NICs/myNic1/MacAddress}",
				"elemental.cattle.io/NetIface0-IsVirtual":    "${Network/NICs/myNic1/IsVirtual}",
				"elemental.cattle.io/NetIface1-Name":         "${Network/NICs/myNic2/Name}",
				"elemental.cattle.io/BlockDevicesNumber":     "${Storage/TotalDisks}",
				"elemental.cattle.io/BlockDevice0-Name":      "${Storage/Disks/testdisk1/Name}",
				"elemental.cattle.io/BlockDevice1-Name":      "${Storage/Disks/testdisk2/Name}",
				"elemental.cattle.io/BlockDevice0-Size":      "${Storage/Disks/testdisk1/Size}",
				"elemental.cattle.io/BlockDevice1-Size":      "${Storage/Disks/testdisk2/Size}",
				"elemental.cattle.io/BlockDevice0-Removable": "${Storage/Disks/testdisk1/Removable}",
				"elemental.cattle.io/UnexistingTemplate":     "${Storage/NotExistingValue}",
				"elemental.cattle.io/Random01":               "my-random-value-${Random/Int/1000}",
				"elemental.cattle.io/Random02":               "${Random/UUID}",
				"elemental.cattle.io/Random03":               "my-random-value-${Random/Hex/5}",
			},
		},
	}

	data := hostinfo.ExtractLabels(hostInfoFixture)
	encodedData, err := json.Marshal(data)
	assert.NilError(t, err)

	hostinfoData := map[string]interface{}{}
	err = json.Unmarshal(encodedData, &hostinfoData)
	assert.NilError(t, err)
	tmpl.Fill(hostinfoData)

	err = updateInventoryAnnotations(tmpl, inventory, registration)
	assert.NilError(t, err)
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/Hostname"], "machine-1")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/TotalMemory"], "100")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/AvailableMemory"], "90")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/CpuTotalCores"], "300")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/CpuTotalThreads"], "300")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/ProcessorModel"], "-this_is@broken?TM-][{¬{$h4yh46Ŋ£$⅝ŋg46¬~{~←ħ¬")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/NetIfacesNumber"], "2")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/NetIface0-Name"], "myNic1")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/NetIface0-MAC"], "02:00:00:00:00:01")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/NetIface0-IsVirtual"], "true")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/NetIface1-Name"], "myNic2")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/BlockDevicesNumber"], "2")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/BlockDevice0-Name"], "testdisk1")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/BlockDevice1-Name"], "testdisk2")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/BlockDevice0-Size"], "300")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/BlockDevice1-Size"], "600")
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/BlockDevice0-Removable"], "true")
	_, ok := inventory.Annotations["elemental.cattle.io/UnexistingTemplate"]
	assert.Equal(t, ok, false)
	assert.Equal(t, inventory.Annotations["elemental.cattle.io/Random01"], "alreadyFilled")
	assert.Equal(t, len(inventory.Annotations["elemental.cattle.io/Random02"]), len("e511d5ca-a765-42f2-82b7-264f37ffb329"))
	assert.Equal(t, len(inventory.Annotations["elemental.cattle.io/Random03"]), len("my-random-value-12345"))
}
