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

package controllerhelpers

import (
	"context"
	"encoding/json"
	"fmt"

	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/syncer"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// On asynchronous sync this is the number of reconcile loops required to get some data
const syncLoops = 3

type FakeSyncer struct {
	json         string
	asynchronous bool
	loopsCount   int
}

type FakeSyncerProvider struct {
	JSON         string
	UnknownType  string
	Asynchronous bool
	loopsCount   int
}

func (fs FakeSyncer) Sync(_ context.Context, _ client.Client, c *elementalv1.ManagedOSVersionChannel) ([]elementalv1.ManagedOSVersion, error) {
	var errs []error
	res := []elementalv1.ManagedOSVersion{}

	err := json.Unmarshal([]byte(fs.json), &res)
	if err != nil {
		errs = append(errs, err)
	}

	if fs.asynchronous {
		if fs.loopsCount < syncLoops {
			meta.SetStatusCondition(&c.Status.Conditions, metav1.Condition{
				Type:    elementalv1.ReadyCondition,
				Reason:  elementalv1.SyncingReason,
				Status:  metav1.ConditionFalse,
				Message: "On going channel synchronization",
			})
			return nil, nil
		}
	}

	meta.SetStatusCondition(&c.Status.Conditions, metav1.Condition{
		Type:    elementalv1.ReadyCondition,
		Reason:  elementalv1.GotChannelDataReason,
		Status:  metav1.ConditionFalse,
		Message: "Got valid channel data",
	})

	return res, errorutils.NewAggregate(errs)
}

func (sp *FakeSyncerProvider) NewOSVersionsSyncer(spec elementalv1.ManagedOSVersionChannelSpec, _ string, _ *rest.Config) (syncer.Syncer, error) {
	sp.loopsCount++
	if spec.Type == sp.UnknownType {
		return FakeSyncer{}, fmt.Errorf("Unknown type of channel")
	}
	return FakeSyncer{json: sp.JSON, asynchronous: sp.Asynchronous, loopsCount: sp.loopsCount}, nil
}
