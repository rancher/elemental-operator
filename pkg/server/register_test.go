/*
Copyright © 2022 SUSE LLC

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
	"net/http"
	"net/url"
	"regexp"
	"testing"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestInitNewInventory(t *testing.T) {
	const alphanum = "[0-9a-fA-F]"
	// m  '-'  8 alphanum chars  '-'  3 blocks of 4 alphanum chars  '-'  12 alphanum chars
	mUUID := regexp.MustCompile("^m-" + alphanum + "{8}-(" + alphanum + "{4}-){3}" + alphanum + "{12}")
	// e.g., m-66588488-3eb6-4a6d-b642-c994f128c6f1

	testCase := []struct {
		noSMBIOS     bool
		initName     string
		expectedName string
	}{
		{
			noSMBIOS:     false,
			initName:     "custom-name",
			expectedName: "custom-name",
		},
		{
			noSMBIOS:     false,
			expectedName: "m-${System Information/UUID}",
		},
		{
			noSMBIOS: true,
		},
	}

	for _, test := range testCase {
		registration := &elementalv1.MachineRegistration{
			Spec: elementalv1.MachineRegistrationSpec{
				MachineName: test.initName,
				Config: &elementalv1.Config{
					Elemental: elementalv1.Elemental{
						Registration: elementalv1.Registration{
							NoSMBIOS: test.noSMBIOS,
						},
					},
				},
			},
		}

		inventory := &elementalv1.MachineInventory{}
		initInventory(inventory, registration)

		if test.noSMBIOS {
			assert.Check(t, mUUID.Match([]byte(inventory.Name)), inventory.Name+" is not UUID based")
		} else {
			assert.Equal(t, inventory.Name, test.expectedName)
		}
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
			Output: "m",
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
			Output: "m",
		},
		{
			Format: "a${level1A}c",
			Output: "ac",
		},
		{
			Format: "a${level1A}",
			Output: "a",
		},
		{
			Format: "${level1A}c",
			Output: "c",
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
	}

	for _, testCase := range testCase {
		assert.Equal(t, testCase.Output, buildStringFromSmbiosData(data, testCase.Format))
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

func TestCreateMachineInventory(t *testing.T) {
	testCases := []struct {
		name                 string
		inputMInventory      *elementalv1.MachineInventory
		existingMInventories []*elementalv1.MachineInventory
		expectedError        bool
	}{
		{
			name: "succesfully create inventory",
			inputMInventory: &elementalv1.MachineInventory{
				ObjectMeta: v1.ObjectMeta{
					Name:      "name",
					Namespace: "namespace",
				},
				Spec: elementalv1.MachineInventorySpec{
					TPMHash: "test",
				},
			},
			existingMInventories: []*elementalv1.MachineInventory{
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name1",
						Namespace: "namespace",
					},
					Spec: elementalv1.MachineInventorySpec{
						TPMHash: "test1",
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name2",
						Namespace: "namespace",
					},
					Spec: elementalv1.MachineInventorySpec{
						TPMHash: "test2",
					},
				},
			},
		},
		{
			name: "fail when machine inventory with similar tpm hash exists",
			inputMInventory: &elementalv1.MachineInventory{
				ObjectMeta: v1.ObjectMeta{
					Name:      "name",
					Namespace: "namespace",
				},
				Spec: elementalv1.MachineInventorySpec{
					TPMHash: "test",
				},
			},
			existingMInventories: []*elementalv1.MachineInventory{
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name1",
						Namespace: "namespace",
					},
					Spec: elementalv1.MachineInventorySpec{
						TPMHash: "test1",
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name2",
						Namespace: "namespace",
					},
					Spec: elementalv1.MachineInventorySpec{
						TPMHash: "test",
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			elementalv1.AddToScheme(scheme)

			objs := []runtime.Object{}
			for _, mInventory := range tc.existingMInventories {
				objs = append(objs, mInventory)
			}
			inventoryServer := InventoryServer{
				Client: fake.NewFakeClientWithScheme(scheme, objs...),
			}

			_, err := inventoryServer.createMachineInventory(tc.inputMInventory)
			if tc.expectedError {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("did not expect error")
				}
			}
		})
	}
}

func TestGetMachineRegistration(t *testing.T) {
	testCases := []struct {
		name                   string
		inputUrl               string
		existingMRegistrations []*elementalv1.MachineRegistration
		expectedError          bool
	}{
		{
			name:     "succesfully find machine registration for the token",
			inputUrl: "https://example.com/token",
			existingMRegistrations: []*elementalv1.MachineRegistration{
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name1",
						Namespace: "namespace",
					},
					Status: elementalv1.MachineRegistrationStatus{
						RegistrationToken: "token",
						Conditions: []metav1.Condition{
							{
								Type:   elementalv1.ReadyCondition,
								Reason: elementalv1.SuccefullyCreatedReason,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name2",
						Namespace: "namespace",
					},
					Status: elementalv1.MachineRegistrationStatus{
						RegistrationToken: "token1",
						Conditions: []metav1.Condition{
							{
								Type:   elementalv1.ReadyCondition,
								Reason: elementalv1.SuccefullyCreatedReason,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
		},
		{
			name:     "fail when two registrations with similar token exist",
			inputUrl: "https://example.com/token",
			existingMRegistrations: []*elementalv1.MachineRegistration{
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name1",
						Namespace: "namespace",
					},
					Status: elementalv1.MachineRegistrationStatus{
						RegistrationToken: "token1",
						Conditions: []metav1.Condition{
							{
								Type:   elementalv1.ReadyCondition,
								Reason: elementalv1.SuccefullyCreatedReason,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name2",
						Namespace: "namespace",
					},
					Status: elementalv1.MachineRegistrationStatus{
						RegistrationToken: "token",
						Conditions: []metav1.Condition{
							{
								Type:   elementalv1.ReadyCondition,
								Reason: elementalv1.SuccefullyCreatedReason,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name:      "name3",
						Namespace: "namespace",
					},
					Status: elementalv1.MachineRegistrationStatus{
						RegistrationToken: "token",
						Conditions: []metav1.Condition{
							{
								Type:   elementalv1.ReadyCondition,
								Reason: elementalv1.SuccefullyCreatedReason,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
			},
			expectedError: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			elementalv1.AddToScheme(scheme)

			objs := []runtime.Object{}
			for _, mInventory := range tc.existingMRegistrations {
				objs = append(objs, mInventory)
			}
			inventoryServer := InventoryServer{
				Client: fake.NewFakeClientWithScheme(scheme, objs...),
			}

			url, err := url.Parse(tc.inputUrl)
			if err != nil {
				t.Fatalf("failed to parse url: %s", err.Error())
			}

			_, err = inventoryServer.getMachineRegistration(&http.Request{
				URL: url,
			})

			if tc.expectedError {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("did not expect error")
				}
			}
		})
	}
}
