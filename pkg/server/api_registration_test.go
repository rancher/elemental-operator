/*
Copyright © 2022 - 2025 SUSE LLC

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/jaypipes/ghw/pkg/block"
	"github.com/jaypipes/ghw/pkg/cpu"
	"github.com/jaypipes/ghw/pkg/memory"
	"github.com/jaypipes/ghw/pkg/net"
	"k8s.io/apimachinery/pkg/util/validation"

	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"gopkg.in/yaml.v3"
	"gotest.tools/v3/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/hostinfo"
	"github.com/rancher/elemental-operator/pkg/register"
	elementalruntime "github.com/rancher/elemental-operator/pkg/runtime"
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

func TestUnauthenticatedResponse(t *testing.T) {
	testCase := []struct {
		config elementalv1.Config
		regUrl string
	}{
		{
			config: elementalv1.Config{},
			regUrl: "https://rancher/url1",
		},
		{
			config: elementalv1.Config{
				Elemental: elementalv1.Elemental{
					Registration: elementalv1.Registration{
						EmulateTPM:      true,
						EmulatedTPMSeed: 127,
						NoSMBIOS:        true,
					},
				},
			},
			regUrl: "https://rancher/url2",
		},
	}

	for _, test := range testCase {
		scheme := runtime.NewScheme()
		elementalv1.AddToScheme(scheme)
		managementv3.AddToScheme(scheme)

		i := &InventoryServer{
			Context: context.Background(),
			Client:  fake.NewClientBuilder().Build(),
		}
		registration := elementalv1.MachineRegistration{}
		registration.Spec.Config = test.config
		registration.Status.RegistrationURL = test.regUrl
		registration.Status.Conditions = []metav1.Condition{
			{
				Type:               elementalv1.ReadyCondition,
				Reason:             elementalv1.SuccessfullyCreatedReason,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
			},
		}

		buffer := new(bytes.Buffer)

		err := i.unauthenticatedResponse(&registration, buffer)
		assert.NilError(t, err, err)

		conf := elementalv1.Config{}
		err = yaml.NewDecoder(buffer).Decode(&conf)
		assert.NilError(t, err, strings.TrimSpace(buffer.String()))
		assert.Equal(t, conf.Elemental.Registration.URL, test.regUrl)

		confReg := conf.Elemental.Registration
		testReg := elementalv1.Registration{}
		if !reflect.DeepEqual(test.config, elementalv1.Config{}) {
			testReg = test.config.Elemental.Registration
		}
		assert.Equal(t, confReg.EmulateTPM, testReg.EmulateTPM)
		assert.Equal(t, confReg.EmulatedTPMSeed, testReg.EmulatedTPMSeed)
		assert.Equal(t, confReg.NoSMBIOS, testReg.NoSMBIOS)
	}
}

func TestBuildName(t *testing.T) {
	data := map[string]interface{}{
		"level1A": map[string]interface{}{
			"level2A": "level2AValue",
			"level2B": map[string]interface{}{
				"level3A": "level3AValue",
			},
		},
		"level1B": "level1BValue",
	}

	testCase := []struct {
		Format string
		Output string
		Error  string
	}{
		{
			Format: "${level1B}",
			Output: "level1BValue",
		},
		{
			Format: "${level1B",
			Output: "m-level1B",
		},
		{
			Format: "a${level1B",
			Output: "a-level1B",
		},
		{
			Format: "${}",
			Error:  "value not found",
		},
		{
			Format: "${",
			Output: "m-",
		},
		{
			Format: "a${",
			Output: "a-",
		},
		{
			Format: "${level1A}",
			Error:  "value not found",
		},
		{
			Format: "a${level1A}c",
			Error:  "value not found",
		},
		{
			Format: "a${level1A}",
			Error:  "value not found",
		},
		{
			Format: "${level1A}c",
			Error:  "value not found",
		},
		{
			Format: "a${level1A/level2A}c",
			Output: "alevel2AValuec",
		},
		{
			Format: "a${level1A/level2B/level3A}c",
			Output: "alevel3AValuec",
		},
		{
			Format: "a${level1A/level2B/level3A}c${level1B}",
			Output: "alevel3AValueclevel1BValue",
		},
		{
			Format: "a${unknown}",
			Error:  "value not found",
		},
	}

	for _, testCase := range testCase {
		t.Run(testCase.Format, func(t *testing.T) {
			str, err := replaceStringData(data, testCase.Format)
			if testCase.Error == "" {
				str = sanitizeString(str)
				assert.NilError(t, err)
				assert.Equal(t, testCase.Output, str, "'%s' not equal to '%s'", testCase.Output, str)
			} else {
				assert.Equal(t, testCase.Error, err.Error())
			}
		})
	}
}

func TestMergeInventoryLabels(t *testing.T) {
	testCase := []struct {
		data     []byte            // labels to add to the inventory
		labels   map[string]string // labels already in the inventory
		fail     bool
		expected map[string]string
	}{
		{
			[]byte(`{"key2":"val2"}`),
			map[string]string{"key1": "val1"},
			false,
			map[string]string{"key1": "val1"},
		},
		{
			[]byte(`{"key2":2}`),
			map[string]string{"key1": "val1"},
			true,
			map[string]string{"key1": "val1"},
		},
		{
			[]byte(`{"key2":"val2", "key3":"val3"}`),
			map[string]string{"key1": "val1", "key3": "previous_val", "key4": "val4"},
			false,
			map[string]string{"key1": "val1", "key3": "previous_val"},
		},
		{
			[]byte{},
			map[string]string{"key1": "val1"},
			true,
			map[string]string{"key1": "val1"},
		},
		{
			[]byte(`{"key2":"val2"}`),
			nil,
			false,
			map[string]string{},
		},
	}

	for _, test := range testCase {
		inventory := &elementalv1.MachineInventory{}
		inventory.Labels = test.labels

		err := mergeInventoryLabels(inventory, test.data)
		if test.fail {
			assert.Assert(t, err != nil)
		} else {
			assert.Equal(t, err, nil)
		}
		for k, v := range test.expected {
			val, ok := inventory.Labels[k]
			assert.Equal(t, ok, true)
			assert.Equal(t, v, val)
		}

	}
}

func TestUpdateInventoryFromSystemData(t *testing.T) {
	inventory := &elementalv1.MachineInventory{}
	encodedData, err := json.Marshal(hostInfoFixture)
	assert.NilError(t, err)
	err = updateInventoryFromSystemData(encodedData, inventory, systemDataLabelsRegistrationFixture)
	assert.NilError(t, err)

	assertSystemDataLabels(t, inventory)
}

func TestUpdateInventoryFromSystemDataNG(t *testing.T) {
	inventory := &elementalv1.MachineInventory{}
	data, err := hostinfo.ExtractLabels(hostInfoFixture)
	assert.NilError(t, err)
	encodedData, err := json.Marshal(data)
	assert.NilError(t, err)
	err = updateInventoryFromSystemDataNG(encodedData, inventory, systemDataLabelsRegistrationFixture)
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

func TestUpdateInventoryFromSystemDataSanitized(t *testing.T) {
	inventory := &elementalv1.MachineInventory{}
	inventory.Name = "${System Data/Runtime/Hostname}"

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
			},
		},
	}
	data := hostinfo.HostInfo{
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
					Name: "myNic1",
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
	encodedData, err := json.Marshal(data)
	assert.NilError(t, err)
	err = updateInventoryFromSystemData(encodedData, inventory, registration)
	assert.NilError(t, err)
	// Check that the labels we properly added to the inventory
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
}

func TestRegistrationMsgGet(t *testing.T) {
	testCases := []struct {
		name                string
		machineName         string
		protoVersion        register.MessageType
		wantConnectionError bool
		wantRawResponse     bool
		wantMessageType     register.MessageType
	}{
		{
			name:                "returns not-found error for unknown machine",
			machineName:         "unknown",
			wantConnectionError: true,
		},
		{
			name:            "returns raw config for older protoVersion",
			machineName:     "machine-1",
			protoVersion:    register.MsgUndefined,
			wantRawResponse: true,
		},
		{
			name:            "returns MsgConfig for newer protoVersion",
			machineName:     "machine-2",
			protoVersion:    register.MsgError,
			wantRawResponse: false,
			wantMessageType: register.MsgConfig,
		},
		{
			name:            "returns MsgError for newer protoVersion and error",
			machineName:     "machine-2",
			protoVersion:    register.MsgError,
			wantRawResponse: false,
			wantMessageType: register.MsgError,
		},
		{
			name:            "returns MsgError if mregistration status has no URL",
			machineName:     "machine-3",
			protoVersion:    register.MsgError,
			wantRawResponse: false,
			wantMessageType: register.MsgError,
		},
	}

	server := NewInventoryServer(&FakeAuthServer{})

	server.Client.Create(context.Background(), &elementalv1.MachineRegistration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machine-1",
		},
		Spec: elementalv1.MachineRegistrationSpec{
			MachineName: "machine-1",
		},
		Status: elementalv1.MachineRegistrationStatus{
			ServiceAccountRef: &v1.ObjectReference{
				Name: "test-account",
			},
			RegistrationURL:   "https://example.machine-1.org",
			RegistrationToken: "machine-1",
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
		},
	})

	server.Client.Create(context.Background(), &elementalv1.MachineRegistration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machine-2",
		},
		Spec: elementalv1.MachineRegistrationSpec{
			MachineName: "machine-2",
		},
		Status: elementalv1.MachineRegistrationStatus{
			ServiceAccountRef: &v1.ObjectReference{
				Name: "test-account",
			},
			RegistrationURL:   "https://example.machine-2.org",
			RegistrationToken: "machine-2",
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
		},
	})

	server.Client.Create(context.Background(), &elementalv1.MachineRegistration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machine-3",
		},
		Spec: elementalv1.MachineRegistrationSpec{
			MachineName: "machine-3",
		},
		Status: elementalv1.MachineRegistrationStatus{
			ServiceAccountRef: &v1.ObjectReference{
				Name: "test-account",
			},
			RegistrationToken: "machine-3",
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
		},
	})

	createDefaultResources(t, server)

	wsServer := httptest.NewServer(server)
	defer wsServer.Close()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("ws%s/%s", strings.TrimPrefix(wsServer.URL, "http"), "elemental/registration/"+tc.machineName)

			header := http.Header{}
			header.Add("Authorization", tc.machineName)

			ws, _, err := websocket.DefaultDialer.Dial(url, header)
			if tc.wantConnectionError {
				assert.Error(t, err, "websocket: bad handshake")
				return
			} else {
				assert.NilError(t, err)
			}

			defer ws.Close()

			// Read MsgReady
			msgType, _, err := register.ReadMessage(ws)
			assert.NilError(t, err)
			assert.Equal(t, register.MsgReady, msgType)

			// Negotiate version
			if tc.protoVersion != register.MsgUndefined {
				err = register.WriteMessage(ws, register.MsgVersion, []byte{byte(tc.protoVersion)})
				assert.NilError(t, err)

				msgType, _, err = register.ReadMessage(ws)
				assert.NilError(t, err)
				assert.Equal(t, register.MsgVersion, msgType)
			}

			// Actual send MsgGet
			err = register.WriteMessage(ws, register.MsgGet, []byte{})
			assert.NilError(t, err)

			if tc.wantRawResponse {
				msgType, r, err := ws.NextReader()
				assert.NilError(t, err)
				assert.Equal(t, msgType, websocket.BinaryMessage)
				data, _ := io.ReadAll(r)
				assert.Assert(t, strings.HasPrefix(string(data), "elemental:"))
				return
			}

			msgType, data, err := register.ReadMessage(ws)
			assert.NilError(t, err)
			assert.Assert(t, data != nil)
			assert.Equal(t, tc.wantMessageType, msgType)

			config := &elementalv1.Config{}
			err = yaml.Unmarshal(data, &config)
			assert.NilError(t, err)
		})
	}
}

func TestRegistrationMsgUpdate(t *testing.T) {
	testCases := []struct {
		name                string
		machineName         string
		doesInventoryExists bool
		wantMessageType     register.MessageType
	}{
		{
			name:            "returns not-found error for unknown machine inventory",
			machineName:     "machine-1",
			wantMessageType: register.MsgError,
		},
		{
			name:                "returns MsgConfig config on registration update",
			machineName:         "machine-1",
			doesInventoryExists: true,
			wantMessageType:     register.MsgReady,
		},
	}

	authenticator := &FakeAuthServer{}
	server := NewInventoryServer(authenticator)

	server.Client.Create(context.Background(), &elementalv1.MachineRegistration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machine-1",
		},
		Spec: elementalv1.MachineRegistrationSpec{
			MachineName: "machine-1",
		},
		Status: elementalv1.MachineRegistrationStatus{
			ServiceAccountRef: &v1.ObjectReference{
				Name: "test-account",
			},
			RegistrationToken: "machine-1",
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
		},
	})

	createDefaultResources(t, server)

	wsServer := httptest.NewServer(server)
	defer wsServer.Close()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			authenticator.CreateExistingInventory = tc.doesInventoryExists

			url := fmt.Sprintf("ws%s/%s", strings.TrimPrefix(wsServer.URL, "http"), "elemental/registration/"+tc.machineName)

			header := http.Header{}
			header.Add("Authorization", fmt.Sprintf("Bearer TPM%s", tc.machineName))

			ws, _, _ := websocket.DefaultDialer.Dial(url, header)
			defer ws.Close()

			// Read MsgReady
			msgType, _, err := register.ReadMessage(ws)
			assert.NilError(t, err)
			assert.Equal(t, register.MsgReady, msgType)

			// Negotiate version
			err = register.WriteMessage(ws, register.MsgVersion, []byte{byte(register.MsgUpdate)})
			assert.NilError(t, err)
			msgType, _, err = register.ReadMessage(ws)
			assert.NilError(t, err)
			assert.Equal(t, register.MsgVersion, msgType)

			// Actual send MsgUpdate
			err = register.WriteMessage(ws, register.MsgUpdate, []byte{})
			assert.NilError(t, err)

			// Read response message
			msgType, _, err = register.ReadMessage(ws)
			assert.NilError(t, err)
			assert.Equal(t, tc.wantMessageType, msgType)
		})
	}
}

func TestRegistrationDynamicLabels(t *testing.T) {
	server := NewInventoryServer(&FakeAuthServer{})

	server.Client.Create(context.Background(), &elementalv1.MachineRegistration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machine-1",
		},
		Spec: elementalv1.MachineRegistrationSpec{
			MachineName: "machine-1",
			MachineInventoryLabels: map[string]string{
				"uuid":            "${System Information/UUID}",
				"physical_memory": "${System Data/Memory/Total Physical Bytes}",
			},
		},
		Status: elementalv1.MachineRegistrationStatus{
			ServiceAccountRef: &v1.ObjectReference{
				Name: "test-account",
			},
			RegistrationToken: "machine-1",
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
		},
	})

	createDefaultResources(t, server)

	wsServer := httptest.NewServer(server)
	defer wsServer.Close()

	t.Run("generates registration labels from SMBIOS and System data", func(t *testing.T) {
		machineName := "machine-1"
		url := fmt.Sprintf("ws%s/%s", strings.TrimPrefix(wsServer.URL, "http"), "elemental/registration/"+machineName)

		header := http.Header{}
		header.Add("Authorization", machineName)

		ws, _, err := websocket.DefaultDialer.Dial(url, header)
		assert.NilError(t, err)

		defer ws.Close()

		// Read MsgReady
		msgType, _, err := register.ReadMessage(ws)
		assert.NilError(t, err)
		assert.Equal(t, register.MsgReady, msgType)

		// Negotiate version
		err = register.WriteMessage(ws, register.MsgVersion, []byte{byte(register.MsgLast)})
		assert.NilError(t, err)

		msgType, _, err = register.ReadMessage(ws)
		assert.NilError(t, err)
		assert.Equal(t, register.MsgVersion, msgType)

		// Send SMBIOS
		smbios, err := json.Marshal(map[string]interface{}{
			"System Information": map[string]interface{}{
				"UUID": "uuid-123",
			},
		})
		assert.NilError(t, err)
		err = register.WriteMessage(ws, register.MsgSmbios, smbios)
		assert.NilError(t, err)

		msgType, _, err = register.ReadMessage(ws)
		assert.NilError(t, err)
		assert.Equal(t, register.MsgReady, msgType)

		// Send System Data
		systemData := &hostinfo.HostInfo{}
		systemData.Memory = &memory.Info{
			Area: memory.Area{
				TotalPhysicalBytes: 100,
			},
		}
		systemDataJson, err := json.Marshal(systemData)
		assert.NilError(t, err)
		err = register.WriteMessage(ws, register.MsgSystemData, systemDataJson)
		assert.NilError(t, err)

		msgType, _, err = register.ReadMessage(ws)
		assert.NilError(t, err)
		assert.Equal(t, register.MsgReady, msgType)
	})
}

func NewInventoryServer(auth authenticator) *InventoryServer {
	scheme := runtime.NewScheme()
	elementalv1.AddToScheme(scheme)
	clientgoscheme.AddToScheme(scheme)
	managementv3.AddToScheme(scheme)

	return &InventoryServer{
		Context: context.Background(),
		Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
		authenticators: []authenticator{
			auth,
		},
	}
}

type FakeAuthServer struct {
	CreateExistingInventory bool
}

// Authenticate always returns true and a MachineInventory with the TPM-Hash
// set to the machine-name from the URL.
func (a *FakeAuthServer) Authenticate(conn *websocket.Conn, req *http.Request, registerNamespace string) (*elementalv1.MachineInventory, bool, error) {
	token := path.Base(req.URL.Path)

	// A zero CreationTimestamp implies the MachineInventory is new
	creationTimestamp := metav1.Time{}
	if a.CreateExistingInventory {
		creationTimestamp = metav1.Now()
	}

	return &elementalv1.MachineInventory{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: creationTimestamp,
		},
		Spec: elementalv1.MachineInventorySpec{
			TPMHash: token,
		},
	}, true, nil
}

func createDefaultResources(t *testing.T, server *InventoryServer) {
	t.Helper()
	server.Client.Create(context.Background(), &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-account-token",
		},

		Type: v1.SecretTypeServiceAccountToken,
	})

	server.Client.Create(context.Background(), &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-account",
		},
	})

	server.Client.Create(context.Background(), &managementv3.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: "server-url",
		},

		Value: "https://test-server.example.com",
	})

	server.Client.Create(context.Background(), &managementv3.Setting{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cacerts",
		},

		Value: "cacerts",
	})
}
