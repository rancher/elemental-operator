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

package syncer

import (
	"context"
	"fmt"
	"strings"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/object"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	jsonType   = "json"
	customType = "custom"
)

type Syncer interface {
	Sync(ctx context.Context, cl client.Client, ch *elementalv1.ManagedOSVersionChannel) ([]elementalv1.ManagedOSVersion, bool, error)
}

type Provider interface {
	NewOSVersionsSyncer(spec elementalv1.ManagedOSVersionChannelSpec, operatorImage string, config *rest.Config) (Syncer, error)
}

type DefaultProvider struct{}

func (sp DefaultProvider) NewOSVersionsSyncer(spec elementalv1.ManagedOSVersionChannelSpec, operatorImage string, config *rest.Config) (Syncer, error) {
	switch strings.ToLower(spec.Type) {
	case jsonType:
		j := &JSONSyncer{}
		err := object.RenderRawExtension(spec.Options, j)
		if err != nil {
			return nil, err
		}
		return j, nil
	case customType:
		kcl, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}
		j := &CustomSyncer{kcl: kcl, operatorImage: operatorImage}
		err = object.RenderRawExtension(spec.Options, j)
		if err != nil {
			return nil, err
		}
		return j, nil
	default:
		return nil, fmt.Errorf("unknown version channel type '%s'", spec.Type)
	}
}
