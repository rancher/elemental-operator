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

package services

import (
	"fmt"
	"strings"

	provv1 "github.com/rancher-sandbox/rancheros-operator/pkg/apis/rancheros.cattle.io/v1"
	object "github.com/rancher-sandbox/rancheros-operator/pkg/object"
)

const (
	jsonType = "json"
)

func NewManagedOSVersionChannelSyncer(spec provv1.ManagedOSVersionChannelSpec) (syncer, error) {
	switch strings.ToLower(spec.Type) {
	case jsonType:
		j := &JSONSyncer{}
		err := object.Render(spec.Options.Data, j)
		if err != nil {
			return nil, err
		}
		return j, nil
	default:
		return nil, fmt.Errorf("Unknown version channel type '%s'", spec.Type)
	}
}
