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

package types

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/services/syncer/config"
	"github.com/sirupsen/logrus"
)

type JSONSyncer struct {
	URI     string `json:"uri"`
	Timeout string `json:"timeout"`
}

func (j *JSONSyncer) Sync(c config.Config, s elm.ManagedOSVersionChannel) ([]elm.ManagedOSVersion, error) {
	logrus.Infof("Syncing '%s/%s' (JSON)", s.Namespace, s.Name)

	timeout := time.Second * 30
	if j.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(j.Timeout)
		if err != nil {
			return nil, err
		}
	}
	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(j.URI)
	if err != nil {
		return nil, err
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	res := []elm.ManagedOSVersion{}

	err = json.Unmarshal(buf, &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}
