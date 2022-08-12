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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	values "github.com/rancher/wrangler/pkg/data"
	"gotest.tools/assert"
)

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
			Output: "level1bvalue",
		},
		{
			Format: "${level1B",
			Output: "m-level1b",
		},
		{
			Format: "a${level1B",
			Output: "a-level1b",
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
			Output: "alevel2avaluec",
		},
		{
			Format: "a${level1A/level2B/level3A}c",
			Output: "alevel3avaluec",
		},
		{
			Format: "a${level1A/level2B/level3A}c${level1B}",
			Output: "alevel3avalueclevel1bvalue",
		},
	}

	for _, testCase := range testCase {
		assert.Equal(t, testCase.Output, buildName(data, testCase.Format))
	}
}

func TestSmbios(t *testing.T) {
	dmiEncoded := map[string]interface{}{}
	values.PutValue(dmiEncoded, "Myself", "System Information", "Manufacturer")
	var buf bytes.Buffer
	b64Enc := base64.NewEncoder(base64.StdEncoding, &buf)
	json.NewEncoder(b64Enc).Encode(dmiEncoded)
	_ = b64Enc.Close()

	testCase := []struct {
		header http.Header
		path   []string // Path to the value
		value  string   // Actual value
		gotIt  bool     // Did we get the value
	}{
		{
			http.Header{"X-Cattle-Smbios": {buf.String()}}, // Old header
			[]string{"System Information", "Manufacturer"},
			"Myself",
			true,
		},
		{
			http.Header{"X-Cattle-Smbios-1": {buf.String()}}, // New header
			[]string{"System Information", "Manufacturer"},
			"Myself",
			true,
		},
		{
			http.Header{"X-Cattle-Smbios-2": {buf.String()}}, // New header but missing the first part
			[]string{"System Information", "Manufacturer"},
			"",
			false,
		},
		{
			http.Header{}, // Empty header
			[]string{"System Information", "Manufacturer"},
			"",
			false,
		},
	}

	for _, test := range testCase {
		data, err := getSMBios(&http.Request{Header: test.header})
		assert.Equal(t, err, nil)
		d, gotIt := values.GetValue(data, test.path...)
		assert.Equal(t, gotIt, test.gotIt)
		// Cant compare string and nil and values.GetValue returns either a string or a nil
		if test.value == "" {
			assert.Equal(t, d, nil)
		} else {
			assert.Equal(t, d, test.value)
		}

	}
}
