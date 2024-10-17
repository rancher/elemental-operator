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
	"regexp"
	"testing"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"gotest.tools/v3/assert"
)

func TestInitNewInventory(t *testing.T) {
	const alphanum = "[0-9a-fA-F]"
	// m  '-'  8 alphanum chars  '-'  3 blocks of 4 alphanum chars  '-'  12 alphanum chars
	mUUID := regexp.MustCompile("^m-" + alphanum + "{8}-(" + alphanum + "{4}-){3}" + alphanum + "{12}")
	// e.g., m-66588488-3eb6-4a6d-b642-c994f128c6f1

	testCase := []struct {
		config       elementalv1.Config
		initName     string
		expectedName string
	}{
		{
			config: elementalv1.Config{
				Elemental: elementalv1.Elemental{
					Registration: elementalv1.Registration{
						NoSMBIOS: false,
					},
				},
			},
			initName:     "custom-name",
			expectedName: "custom-name",
		},

		{
			config: elementalv1.Config{
				Elemental: elementalv1.Elemental{
					Registration: elementalv1.Registration{
						NoSMBIOS: false,
					},
				},
			},
		},
		{
			config: elementalv1.Config{
				Elemental: elementalv1.Elemental{
					Registration: elementalv1.Registration{
						NoSMBIOS: true,
					},
				},
			},
		},
		{
			config: elementalv1.Config{},
		},
	}

	for _, test := range testCase {
		registration := &elementalv1.MachineRegistration{
			Spec: elementalv1.MachineRegistrationSpec{
				MachineName: test.initName,
				Config:      test.config,
			},
		}

		inventory := &elementalv1.MachineInventory{}
		initInventory(inventory, registration)

		if test.initName == "" {
			assert.Check(t, mUUID.Match([]byte(inventory.Name)), inventory.Name+" is not UUID based")
		} else {
			assert.Equal(t, inventory.Name, test.expectedName)
		}
	}
}
