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

package operator

import (
	"context"

	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	"github.com/rancher/elemental-operator/pkg/controllers/machineinventoryselector"
	"github.com/rancher/elemental-operator/pkg/controllers/managedos"
	"github.com/rancher/elemental-operator/pkg/controllers/managedosversionchannel"
	"github.com/rancher/elemental-operator/pkg/server"
	"github.com/rancher/steve/pkg/aggregation"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func Run(ctx context.Context, settings ...Setting) error {
	o := &options{}

	if err := o.apply(settings...); err != nil {
		return err
	}

	restConfig, err := config.GetConfig()
	if err != nil {
		logrus.Fatalf("failed to find kubeconfig: %v", err)
	}

	cl, err := clients.NewFromConfig(restConfig)
	if err != nil {
		logrus.Fatalf("Error building controller: %s", err.Error())
	}

	factory, err := crd.NewFactoryFromClient(restConfig)
	if err != nil {
		logrus.Fatalf("Failed to create CRD factory: %v", err)
	}

	helmLabels := map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
		"release-name":                 "elemental-operator",
	}

	helmCAPILabels := map[string]string{
		"cluster.x-k8s.io/v1beta1":     "v1beta1",
		"app.kubernetes.io/managed-by": "Helm",
		"release-name":                 "elemental-operator",
	}

	helmAnnotations := map[string]string{
		"meta.helm.sh/release-name":      "elemental-operator",
		"meta.helm.sh/release-namespace": "cattle-elemental-system",
	}

	err = factory.BatchCreateCRDs(ctx,
		crd.CRD{
			SchemaObject: elm.ManagedOSImage{},
			Status:       true,
			Annotations:  helmAnnotations,
			Labels:       helmLabels,
		},
		crd.CRD{
			SchemaObject: elm.ManagedOSVersion{},
			Status:       true,
			Annotations:  helmAnnotations,
			Labels:       helmLabels,
		},
		crd.CRD{
			SchemaObject: elm.ManagedOSVersionChannel{},
			Status:       true,
			Annotations:  helmAnnotations,
			Labels:       helmLabels,
		},
		crd.CRD{
			SchemaObject: elm.MachineInventorySelector{},
			Status:       true,
			Annotations:  helmAnnotations,
			Labels:       helmCAPILabels,
		},
		crd.CRD{
			SchemaObject: elm.MachineInventorySelectorTemplate{},
			Annotations:  helmAnnotations,
			Labels:       helmCAPILabels,
		},
	).BatchWait()
	if err != nil {
		logrus.Fatalf("Failed to create CRDs: %v", err)
	}

	managedos.Register(ctx, cl, o.DefaultRegistry)
	managedosversionchannel.Register(ctx, o.requeuer, cl)
	machineinventoryselector.Register(ctx, cl)

	aggregation.Watch(ctx, cl.Core().Secret(), o.namespace, "elemental-operator", server.New(cl))

	for _, s := range o.services {
		s := s
		go func() {
			err = s(ctx, cl)
			if err != nil {
				logrus.Errorf("Error running services: %s", err)
			}
		}()
	}

	return cl.Start(ctx)
}
