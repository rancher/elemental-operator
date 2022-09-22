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

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	elm "github.com/rancher/elemental-operator/pkg/apis/elemental.cattle.io/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	"github.com/rancher/elemental-operator/pkg/controllers/machineinventory"
	"github.com/rancher/elemental-operator/pkg/controllers/managedosversionchannel"
	"github.com/rancher/elemental-operator/pkg/server"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/steve/pkg/aggregation"
	"github.com/rancher/wrangler/pkg/crd"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(elementalv1.AddToScheme(scheme))
	utilruntime.Must(managementv3.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
}

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

	ctrlRuntimeCl, err := client.New(restConfig, client.Options{
		Scheme: scheme,
	})

	if err != nil {
		logrus.Fatalf("Error building controller runtime controller: %s", err.Error())
	}

	factory, err := crd.NewFactoryFromClient(restConfig)
	if err != nil {
		logrus.Fatalf("Failed to create CRD factory: %v", err)
	}

	err = factory.BatchCreateCRDs(ctx,
		crd.CRD{
			SchemaObject: elm.ManagedOSVersion{},
			Status:       true,
		},
		crd.CRD{
			SchemaObject: elm.ManagedOSVersionChannel{},
			Status:       true,
		},
	).BatchWait()
	if err != nil {
		logrus.Fatalf("Failed to create CRDs: %v", err)
	}

	machineinventory.Register(ctx, cl)
	managedosversionchannel.Register(ctx, o.requeuer, cl)
	aggregation.Watch(ctx, cl.Core().Secret(), o.namespace, "elemental-operator", server.New(ctx, ctrlRuntimeCl))

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
