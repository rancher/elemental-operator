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
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"gotest.tools/v3/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const existingToken = "l6mf7sfx4wbvhm8fcmbnlznsczfgm7q7f9cz4kfbkg9k4r92k9kwqc"

func TestApiSeedImage(t *testing.T) {
	server := NewInventoryServer(&FakeAuthServer{})
	loadSeedImageResources(t, server)

	testCases := []struct {
		token        string
		errMsg       string
		splittedPath []string
		shouldPass   bool
	}{
		{existingToken, "", nil, true},
		{"notExisting", "failed to find seed image with token path", nil, false},
		{"wrongSplittedPath", "unexpected path", []string{"tooshort"}, false},
		{"notReady", "is not ready", nil, false},
		{"noMatchingService", "failed to get service for seed image", nil, false},
	}

	for _, test := range testCases {
		t.Logf("Testing token %s\n", test.token)
		req := httptest.NewRequest(
			"GET",
			fmt.Sprintf("%s/%s/foo.bar", "http://localhost/elemental/seedimage/", test.token),
			nil)
		resp := httptest.NewRecorder()
		splittedPath := test.splittedPath
		if splittedPath == nil {
			splittedPath = []string{"seedimage", test.token, "foo.bar"}
		}

		err := server.apiSeedImage(resp, req, splittedPath)

		if test.shouldPass {
			assert.NilError(t, err, err)
		} else {
			assert.ErrorContains(t, err, test.errMsg, err)
		}
	}
}
func TestGetSeedImage(t *testing.T) {

	testCases := []struct {
		token      string
		errMsg     string
		shouldPass bool
	}{
		{existingToken, "", true},
		{"notExisting", "failed to find seed image with token path notExisting", false},
		{"notReady", "seedimage fleet-default/beta is not ready", false},
		{"\r\nnoMatchingService\n\r", "", true},
	}

	server := NewInventoryServer(&FakeAuthServer{})
	loadSeedImageResources(t, server)

	for _, test := range testCases {
		t.Logf("Testing token %s\n", test.token)

		foundSeedImage, err := server.getSeedImage(test.token)
		if test.shouldPass {
			assert.NilError(t, err, err)
			escapedToken := strings.Replace(test.token, "\n", "", -1)
			escapedToken = strings.Replace(escapedToken, "\r", "", -1)
			assert.Equal(t, foundSeedImage.Status.DownloadToken, escapedToken)
		} else {
			assert.Error(t, err, test.errMsg)
		}
	}
}

func loadSeedImageResources(t *testing.T, server *InventoryServer) {
	t.Helper()
	server.Client.Create(context.Background(), &elementalv1.SeedImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alpha",
			Namespace: "fleet-default",
		},
		Status: elementalv1.SeedImageStatus{
			DownloadToken: existingToken,
		},
	})

	server.Client.Create(context.Background(), &elementalv1.SeedImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "beta",
			Namespace: "fleet-default",
		},
		Status: elementalv1.SeedImageStatus{
			DownloadToken: "notReady",
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionFalse},
			},
		},
	})

	server.Client.Create(context.Background(), &elementalv1.SeedImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gamma",
			Namespace: "fleet-default",
		},
		Status: elementalv1.SeedImageStatus{
			DownloadToken: "noMatchingService",
		},
	})

	server.Client.Create(context.TODO(), &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alpha",
			Namespace: "fleet-default",
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "localhost",
		},
	})
}
