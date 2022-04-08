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

	provv1 "github.com/rancher-sandbox/rancheros-operator/pkg/apis/rancheros.cattle.io/v1"
	"github.com/rancher-sandbox/rancheros-operator/pkg/clients"
	"github.com/rancher-sandbox/rancheros-operator/pkg/object"
	"github.com/rancher-sandbox/rancheros-operator/pkg/services/syncer/config"
	"github.com/rancher-sandbox/rancheros-operator/pkg/services/syncer/types"
	rosTypes "github.com/rancher-sandbox/rancheros-operator/pkg/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const controllerAgentName = "mos-sync"

// UpgradeChannelSync returns a service to keep in sync managedosversions available for upgrade
func UpgradeChannelSync(interval time.Duration, requeuer rosTypes.Requeuer, image string, concurrent bool, namespace ...string) func(context.Context, *clients.Clients) error {
	reSync := func(c *clients.Clients) {
		recorder := c.EventRecorder(controllerAgentName)

		config := config.Config{
			Requeuer:      requeuer,
			OperatorImage: image,
			Clients:       c,
			Recorder:      recorder,
		}
		if len(namespace) == 0 {
			logrus.Debug("Listing all namespaces")
			err := syncNamespace(config, "")
			if err != nil {
				logrus.Warn(err)
			}
			return
		}

		for _, n := range namespace {
			err := syncNamespace(config, n)
			if err != nil {
				logrus.Warn(err)
			}
		}
	}
	return func(ctx context.Context, c *clients.Clients) error {
		work := func() {
			// Delay few seconds between requeues
			time.Sleep(5 * time.Second)
			reSync(c)
		}
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context canceled")
			case <-ticker.C:
				reSync(c)
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
	list, err := config.Clients.OS.ManagedOSVersionChannel().List(namespace, metav1.ListOptions{})
	if err != nil {
		return err
	}

	versions := map[string][]provv1.ManagedOSVersion{}
	for _, vc := range list.Items {
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

		if _, ok := versions[vc.Namespace]; !ok {
			versions[vc.Namespace] = []provv1.ManagedOSVersion{}
		}

		blockDel := false
		for _, v := range vers {
			vcpy := v.DeepCopy()
			vcpy.ObjectMeta.Namespace = vc.Namespace
			ownRef := *metav1.NewControllerRef(&vc, provv1.SchemeGroupVersion.WithKind("ManagedOSVersionChannel"))
			ownRef.BlockOwnerDeletion = &blockDel
			vcpy.ObjectMeta.OwnerReferences = []metav1.OwnerReference{ownRef}
			if vc.Spec.UpgradeContainer != nil {
				vcpy.Spec.UpgradeContainer = vc.Spec.UpgradeContainer
			}
			versions[vc.Namespace] = append(versions[vc.Namespace], *vcpy)
		}
	}

	// TODO: collect all errors
	for _, vv := range versions {
		for _, v := range vv {
			cli := config.Clients.OS.ManagedOSVersion()

			_, err := cli.Get(namespace, v.ObjectMeta.Name, metav1.GetOptions{})
			if err == nil {
				logrus.Debugf("there is already a version defined for %s(%s)", v.Name, v.Spec.Version)
				continue
			}

			_, err = cli.Create(&v)
			if err != nil {
				// TODO: Need to keep cc for each version
				//config.Recorder.Event(&cc, corev1.EventTypeWarning, "sync", err.Error())
				logrus.Debugf("failed creating %s(%s): %s", v.Name, v.Spec.Version, err.Error())
				continue
			}
		}
	}
	return nil
}

type syncer interface {
	Sync(c config.Config, s provv1.ManagedOSVersionChannel) ([]provv1.ManagedOSVersion, error)
}

const (
	jsonType   = "json"
	customType = "custom"
)

func newManagedOSVersionChannelSyncer(spec provv1.ManagedOSVersionChannelSpec) (syncer, error) {
	switch strings.ToLower(spec.Type) {
	case jsonType:
		j := &types.JSONSyncer{}
		err := object.Render(spec.Options.Data, j)
		if err != nil {
			return nil, err
		}
		return j, nil
	case customType:
		j := &types.CustomSyncer{}
		err := object.Render(spec.Options.Data, j)
		if err != nil {
			return nil, err
		}
		return j, nil
	default:
		return nil, fmt.Errorf("unknown version channel type '%s'", spec.Type)
	}
}
