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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/jaypipes/ghw/pkg/memory"

	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"gopkg.in/yaml.v3"
	"gotest.tools/v3/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/hostinfo"
	"github.com/rancher/elemental-operator/pkg/register"
	"github.com/rancher/elemental-operator/pkg/templater"
)

func TestUnauthenticatedResponse(t *testing.T) {
	testCase := []struct {
		config *elementalv1.Config
		regUrl string
	}{
		{
			config: nil,
			regUrl: "https://rancher/url1",
		},
		{
			config: &elementalv1.Config{
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
		if test.config != nil {
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
			Output: "level1B",
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
			Output: "",
		},
		{
			Format: "a${",
			Output: "a",
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
		{
			Format: "+check-sanitize!",
			Output: "check-sanitize",
		},
		{
			Format: "VeryVeryVeryLongLabelValueThatWillBeCutTo58Chars-xxxxx-End|CUTFROMHERE",
			Output: "VeryVeryVeryLongLabelValueThatWillBeCutTo58Chars-xxxxx-End",
		},
		{
			Format: "double--dash--+-sanitized++--",
			Output: "double-dash-sanitized",
		},
		{
			Format: "AllowedNotAlphaNumChars:_.NotTrailingBTW_.",
			Output: "AllowedNotAlphaNumChars-_.NotTrailingBTW",
		},
		{
			Format: "+.!_-",
			Output: "",
		},
		{
			Format: "",
			Output: "",
		},
	}

	tmpl := templater.NewTemplater()
	tmpl.Fill(data)
	for _, testCase := range testCase {
		t.Run(testCase.Format, func(t *testing.T) {
			str, err := tmpl.Decode(testCase.Format)
			if testCase.Error == "" {
				str = sanitizeLabel(str)
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

func TestMergeInventoryAnnotations(t *testing.T) {
	testCase := []struct {
		data        []byte            // annotations to add to the inventory
		annotations map[string]string // annotations already in the inventory
		fail        bool
		expected    map[string]string
	}{
		{
			[]byte(`{"key2":"val2"}`),
			map[string]string{"key1": "val1"},
			false,
			map[string]string{"key1": "val1", "elemental.cattle.io/key2": "val2"},
		},
		{
			[]byte(`{"key2":2}`),
			map[string]string{"key1": "val1"},
			true,
			map[string]string{"key1": "val1"},
		},
		{
			[]byte(`{"key2":"val2", "key3":"val3"}`),
			map[string]string{"key1": "val1", "elemental.cattle.io/key3": "previous_val"},
			false,
			map[string]string{"key1": "val1", "elemental.cattle.io/key3": "val3", "elemental.cattle.io/key2": "val2"},
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
			map[string]string{"elemental.cattle.io/key2": "val2"},
		},
	}

	for _, test := range testCase {
		inventory := &elementalv1.MachineInventory{}
		inventory.Annotations = test.annotations

		err := mergeInventoryAnnotations(test.data, inventory)
		if test.fail {
			assert.Assert(t, err != nil)
		} else {
			assert.NilError(t, err)
		}
		for k, v := range test.expected {
			val, ok := inventory.Annotations[k]
			assert.Equal(t, ok, true, "annotations: %v\nexpected: %v ", inventory.Annotations, test.expected)
			assert.Equal(t, v, val, "annotations: %v\nexpected: %v ", inventory.Annotations, test.expected)
		}
	}
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

func TestAgentTLSMode(t *testing.T) {
	type test struct {
		name              string
		agentTLSModeValue *string
		wantStrictTLSMode bool
	}

	tests := []test{
		{
			name:              "missing agent-tls-mode",
			agentTLSModeValue: nil,
			wantStrictTLSMode: true,
		},
		{
			name:              "strict agent-tls-mode",
			agentTLSModeValue: ptr.To("strict"),
			wantStrictTLSMode: true,
		},
		{
			name:              "system-store agent-tls-mode",
			agentTLSModeValue: ptr.To("system-store"),
			wantStrictTLSMode: false,
		},
	}

	for _, tt := range tests {
		server := NewInventoryServer(&FakeAuthServer{})

		t.Run(tt.name, func(t *testing.T) {
			if tt.agentTLSModeValue != nil {
				server.Client.Create(context.Background(), &managementv3.Setting{
					ObjectMeta: metav1.ObjectMeta{
						Name: "agent-tls-mode",
					},

					Value: *tt.agentTLSModeValue,
				})
			}

			assert.Equal(t, server.isAgentTLSModeStrict(), tt.wantStrictTLSMode)
		})
	}

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
