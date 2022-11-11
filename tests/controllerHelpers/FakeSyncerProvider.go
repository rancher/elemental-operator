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

package controllerHelpers

import (
	"context"
	"encoding/json"
	"fmt"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/syncer"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FakeSyncer struct {
	json string
}

type FakeSyncerProvider struct {
	JSON        string
	UnknownType string
}

func (fs FakeSyncer) Sync(ctx context.Context, cl client.Client, c *elementalv1.ManagedOSVersionChannel) ([]elementalv1.ManagedOSVersion, bool, error) {
	res := []elementalv1.ManagedOSVersion{}

	err := json.Unmarshal([]byte(fs.json), &res)
	if err != nil {
		return nil, true, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return res, false, nil
}

func (sp FakeSyncerProvider) NewOSVersionsSyncer(spec elementalv1.ManagedOSVersionChannelSpec, operatorImage string, config *rest.Config) (syncer.Syncer, error) {
	if spec.Type == sp.UnknownType {
		return FakeSyncer{}, fmt.Errorf("Unknown type of channel")
	}
	return FakeSyncer{json: sp.JSON}, nil
}
