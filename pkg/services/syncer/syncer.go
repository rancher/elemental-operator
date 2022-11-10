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
	"time"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/pkg/clients"
	"github.com/rancher/elemental-operator/pkg/object"
	"github.com/rancher/elemental-operator/pkg/services/syncer/config"
	"github.com/rancher/elemental-operator/pkg/services/syncer/types"
	elmTypes "github.com/rancher/elemental-operator/pkg/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const controllerAgentName = "mos-sync"

// UpgradeChannelSync returns a service to keep in sync managedosversions available for upgrade
func UpgradeChannelSync(interval time.Duration, requeuer elmTypes.Requeuer, image string, concurrent bool, namespace ...string) func(context.Context, *clients.Clients, client.Client) error {
	reSync := func(ctx context.Context, c *clients.Clients, runtimeClient client.Client) {
		recorder := c.EventRecorder(controllerAgentName)

		cfg := config.Config{
			Context:       ctx,
			Requeuer:      requeuer,
			OperatorImage: image,
			Clients:       c,
			Recorder:      recorder,
			RuntimeClient: runtimeClient,
		}
		if len(namespace) == 0 {
			err := syncNamespace(cfg, "")
			if err != nil {
				logrus.Warn(err)
			}
			return
		}

		for _, n := range namespace {
			err := syncNamespace(cfg, n)
			if err != nil {
				logrus.Warn(err)
			}
		}
	}
	return func(ctx context.Context, c *clients.Clients, runtimeClient client.Client) error {
		work := func() {
			// Delay few seconds between requeues
			time.Sleep(5 * time.Second)
			reSync(ctx, c, runtimeClient)
		}
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context canceled")
			case <-ticker.C:
				reSync(ctx, c, runtimeClient)
			case <-requeuer.Dequeue():
				if concurrent {
					go work()
				} else {
					work()
				}
			}
		}
	}
}

func syncNamespace(config config.Config, namespace string) error {
	if namespace == "" {
		logrus.Debug("Syncing all namespaces")
	} else {
		logrus.Debug("Syncing namespace: ", namespace)
	}

	managedOSVersionChannelList := &elementalv1.ManagedOSVersionChannelList{}
	if err := config.RuntimeClient.List(config, managedOSVersionChannelList, client.InNamespace(namespace)); err != nil {
		return err
	}

	for _, vc := range managedOSVersionChannelList.Items {
		s, err := newManagedOSVersionChannelSyncer(vc.Spec)
		if err != nil {
			return err
		}

		vers, err := s.Sync(config, vc)
		if err != nil {
			config.Recorder.Event(&vc, corev1.EventTypeWarning, "sync", err.Error())
			logrus.Error(err)
			continue
		}

		for _, v := range vers {
			vcpy := v.DeepCopy()
			vcpy.ObjectMeta.Namespace = vc.Namespace
			vcpy.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: elementalv1.GroupVersion.String(),
					Kind:       "ManagedOSVersionChannel",
					Name:       vc.Name,
					UID:        vc.UID,
					Controller: pointer.Bool(true),
				},
			}

			if vc.Spec.UpgradeContainer != nil {
				vcpy.Spec.UpgradeContainer = vc.Spec.UpgradeContainer
			}

			if err := config.RuntimeClient.Create(config, vcpy); err != nil {
				if apierrors.IsAlreadyExists(err) {
					msg := fmt.Sprintf("there is already a version defined for %s(%s)", vcpy.Name, vcpy.Spec.Version)
					config.Recorder.Event(&vc, corev1.EventTypeWarning, "sync", msg)
					continue
				}
				logrus.Errorf("failed to create ManagedOSVersion %s: %v", vcpy.Name, err)
				config.Recorder.Event(&vc, corev1.EventTypeWarning, "sync", err.Error())
				continue
			}
		}
	}

	return nil
}

type syncer interface {
	Sync(c config.Config, s elementalv1.ManagedOSVersionChannel) ([]elementalv1.ManagedOSVersion, error)
}

const (
	jsonType   = "json"
	customType = "custom"
)

func newManagedOSVersionChannelSyncer(spec elementalv1.ManagedOSVersionChannelSpec) (syncer, error) {
	logrus.Debug("Create a syncer of type: ", spec.Type)
	switch strings.ToLower(spec.Type) {
	case jsonType:
		j := &types.JSONSyncer{}
		err := object.RenderRawExtension(spec.Options, j)
		if err != nil {
			return nil, err
		}
		return j, nil
	case customType:
		j := &types.CustomSyncer{}
		err := object.RenderRawExtension(spec.Options, j)
		if err != nil {
			return nil, err
		}
		return j, nil
	default:
		return nil, fmt.Errorf("unknown version channel type '%s'", spec.Type)
	}
}
