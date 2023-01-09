/*
Copyright © SUSE LLC

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

package clients

import (
	"context"

	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/pkg/clients"
	corecontrollers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1Typed "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
)

type ClientInterface interface {
	EventRecorder(name string) record.EventRecorder
	Start(ctx context.Context) error
	Core() corecontrollers.Interface
}

type Clients struct {
	sharedControllerFactory controller.SharedControllerFactory
	core                    corecontrollers.Interface
	Events                  corev1Typed.EventInterface
}

func (c *Clients) Core() corecontrollers.Interface {
	return c.core
}

// EventRecorder creates an event recorder associated to a controller nome for the schema (arbitrary)
func (c *Clients) EventRecorder(name string) record.EventRecorder {
	// Create event broadcaster
	logrus.Info("Creating event broadcaster for " + name)
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1Typed.EventSinkImpl{Interface: c.Events})
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: name})
}

func NewFromConfig(restConfig *rest.Config) (*Clients, error) {
	c, err := clients.NewFromConfig(restConfig, nil)
	if err != nil {
		return nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return &Clients{
		core:                    c.Core,
		Events:                  kubeClient.CoreV1().Events(""),
		sharedControllerFactory: c.SharedControllerFactory,
	}, nil
}

func (c *Clients) Start(ctx context.Context) error {
	return c.sharedControllerFactory.Start(ctx, 5)
}
