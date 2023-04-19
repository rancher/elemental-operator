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

func (i *InventoryServer) apiSeedImage(resp http.ResponseWriter, req *http.Request, splittedPath []string) error {
	var err error
	var seedImg *elementalv1.SeedImage

	// expected splittedPath = {"seedimage", {token}}
	if len(splittedPath) != 2 {
		err = fmt.Errorf("seedimage not found")
		http.Error(resp, err.Error(), http.StatusNotFound)
		return err
	}
	token := splittedPath[1]

	if seedImg, err = i.getSeedImage(token); err != nil {
		http.Error(resp, err.Error(), http.StatusNotFound)
		return err
	}

	svc := &corev1.Service{}
	if err = i.Get(i, types.NamespacedName{Namespace: seedImg.Namespace, Name: seedImg.Name}, svc); err != nil {
		errMsg := fmt.Errorf("failed to get service for seed image %s/%s: %w", seedImg.Namespace, seedImg.Name, err)
		http.Error(resp, errMsg.Error(), http.StatusInternalServerError)
		return errMsg
	}

	rawURL := fmt.Sprintf("http://%s", svc.Spec.ClusterIP)
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

func (i *InventoryServer) getSeedImage(token string) (*elementalv1.SeedImage, error) {
	escapedToken := strings.Replace(token, "\n", "", -1)
	escapedToken = strings.Replace(escapedToken, "\r", "", -1)

	seedImgList := &elementalv1.SeedImageList{}
	if err := i.List(i, seedImgList); err != nil {
		return nil, fmt.Errorf("failed to list seed images")
	}

	var seedImg *elementalv1.SeedImage

	for _, s := range seedImgList.Items {
		if path.Base(s.Status.DownloadToken) == escapedToken {
			seedImg = (&s).DeepCopy()
			break
		}
	}

	if seedImg == nil {
		return nil, fmt.Errorf("failed to find seed image with token path %s", escapedToken)
	}

	for _, condition := range seedImg.Status.Conditions {
		if condition.Status != "True" {
			return nil, fmt.Errorf("seedimage %s/%s is not ready", seedImg.Namespace, seedImg.Name)
		}
	}

	return seedImg, nil
}
