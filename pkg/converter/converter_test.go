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

package converter

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Convert to legacy configuration", func() {

	Context("received nested cloud-config data as map[string]interface{}", func() {
		cloudConfData := map[string]interface{}{
			"users": []interface{}{
				map[interface{}]interface{}{
					"name":   "bar",
					"passwd": "foo",
					"groups": "users",
					"ssh_authorized_keys": []interface{}{
						"ssh-key qwertyuyio123456",
					},
				},
			},
			"runcmd": []interface{}{
				"foo",
			},
		}

		expectedData := map[string]interface{}{
			"users": []interface{}{
				map[string]interface{}{
					"name":   "bar",
					"passwd": "foo",
					"groups": "users",
					"ssh_authorized_keys": []interface{}{
						"ssh-key qwertyuyio123456",
					},
				},
			},
			"runcmd": []interface{}{
				"foo",
			},
		}

		It("should convert to map[string]runtime.RawExtension and back to legacy map[string]interface{}", func() {

			cloudConf, err := CloudConfigFromLegacy(cloudConfData)
			Expect(err).ShouldNot(HaveOccurred())
			cloudConfLegacy, err := CloudConfigToLegacy(cloudConf)
			Expect(err).ShouldNot(HaveOccurred())

			for k, v := range expectedData {
				val, ok := cloudConfLegacy[k]
				Expect(ok).To(BeTrue(), "key '%s' is missing", k)
				v1 := reflect.ValueOf(v)
				v2 := reflect.ValueOf(val)
				Expect(v1.Type()).To(Equal(v2.Type()), "key '%s' types differ: %s vs %s", k, v1.Type(), v2.Type())
				Expect(reflect.DeepEqual(v, val)).To(BeTrue(), "key '%s' values differ\n%+v\n%+v", k, v, val)
			}
		})
	})
})
