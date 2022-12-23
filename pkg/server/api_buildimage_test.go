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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestApiBuildImage(t *testing.T) {
	testCase := []struct {
		token  string
		method string
		url    string
		body   string
		valid  bool
	}{
		{
			method: http.MethodDelete,
			url:    "/elemental/build-iso",
			valid:  false,
		},
		{
			token:  "abcdefghi",
			method: http.MethodPost,
			url:    "/elemental/build-iso",
			body:   `{"token":"abcdefghi","url":"myisourl"}`,
			valid:  true,
		},
		{
			token:  "abcdefg",
			method: http.MethodPost,
			url:    "/elemental/build-iso",
			body:   `{"token":"abcdefghi","url":"myisourl"}`,
			valid:  false,
		},
		{
			token:  "abcdef",
			method: http.MethodGet,
			url:    "/elemental/build-iso/abcdef",
			body:   `{"status":"Not Started"}`,
			valid:  true,
		},
		{
			token:  "abcdefg",
			method: http.MethodGet,
			url:    "/elemental/build-iso/abcdef",
			valid:  false,
		},
	}

	scheme := runtime.NewScheme()
	elementalv1.AddToScheme(scheme)

	for _, test := range testCase {
		i := New(context.Background(), fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(&elementalv1.MachineRegistration{
				Status: elementalv1.MachineRegistrationStatus{
					RegistrationToken: test.token,
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: "True",
						},
					},
				},
				Spec: elementalv1.MachineRegistrationSpec{},
			}).Build())

		body := strings.NewReader(test.body)
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(test.method, test.url, body)
		err := i.apiBuildImage(resp, req)

		if !test.valid {
			assert.ErrorContains(t, err, "", "\n token: %s\n API ep: %s @ %s\n body: %s", test.token, test.method, test.url, test.body)
			continue
		}

		assert.NilError(t, err, "\n token: %s\n API ep: %s @ %s\n body: %s", test.token, test.method, test.url, test.body)
		assert.Equal(t, test.body+"\n", resp.Body.String(), "response body doesn't match")
		if test.method == http.MethodPost {
			reg, err := i.registrationCache.getRegistrationData(test.token)
			assert.NilError(t, err, "cannot find queued build job for token %s", test.token)
			assert.Equal(t, reg.buildImageStatus, jobStatusStarted, "unexpected queued build job status for token %s", test.token)
		}
	}
}
