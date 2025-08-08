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
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

func (i *InventoryServer) apiChangelog(resp http.ResponseWriter, req *http.Request, splittedPath []string) error {
	var err error
	var changelog *elementalv1.ManagedOSChangelog

	// expected splittedPath = {"changelog", {ManagedOSVersion.Name}}
	if len(splittedPath) < 2 {
		err = fmt.Errorf("unexpected path: %v", splittedPath)
		http.Error(resp, err.Error(), http.StatusNotFound)
		return err
	}
	osVersion := splittedPath[1]
	filename := ""
	if len(splittedPath) == 3 {
		filename = splittedPath[2]
	}

	if changelog, err = i.getManagedOSChangelog(osVersion); err != nil {
		http.Error(resp, err.Error(), http.StatusNotFound)
		return err
	}

	svc := &corev1.Service{}
	if err = i.Get(i, types.NamespacedName{Namespace: changelog.Namespace, Name: changelog.Name}, svc); err != nil {
		errMsg := fmt.Errorf("failed to get service for changelog %s/%s: %w", changelog.Namespace, changelog.Name, err)
		http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
		return errMsg
	}

	rawURL := fmt.Sprintf("http://%s/%s", svc.Spec.ClusterIP, filename)
	seedImgURL, err := url.Parse(rawURL)
	if err != nil {
		errMsg := fmt.Errorf("failed to parse url '%s'", rawURL)
		http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
		return errMsg
	}
	director := func(r *http.Request) {
		r.URL = seedImgURL
	}

	reverseProxy := &httputil.ReverseProxy{
		Director: director,
	}
	reverseProxy.ServeHTTP(resp, req)

	return nil
}

func (i *InventoryServer) getManagedOSChangelog(osVersion string) (*elementalv1.ManagedOSChangelog, error) {
	escapedToken := strings.Replace(osVersion, "\n", "", -1)
	escapedToken = strings.Replace(escapedToken, "\r", "", -1)

	changelogList := &elementalv1.ManagedOSChangelogList{}
	if err := i.List(i, changelogList); err != nil {
		return nil, fmt.Errorf("failed to list managedoschangelogs")
	}

	var changelog *elementalv1.ManagedOSChangelog

	for _, c := range changelogList.Items {
		if path.Base(c.Spec.ManagedOSVersion) == escapedToken {
			changelog = (&c).DeepCopy()
			break
		}
	}

	if changelog == nil {
		return nil, fmt.Errorf("failed to find changelog with ManagedOSVersion %s", escapedToken)
	}

	for _, condition := range changelog.Status.Conditions {
		if condition.Status != "True" {
			return nil, fmt.Errorf("changelog %s/%s is not ready", changelog.Namespace, changelog.Name)
		}
	}

	return changelog, nil
}
