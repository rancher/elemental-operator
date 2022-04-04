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
	"context"
	"fmt"
	"time"

	provv1 "github.com/rancher-sandbox/rancheros-operator/pkg/apis/rancheros.cattle.io/v1"
	"github.com/rancher-sandbox/rancheros-operator/pkg/clients"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradeChannelSync returns a service to keep in sync managedosversions available for upgrade
func UpgradeChannelSync(interval time.Duration, namespace ...string) func(context.Context, *clients.Clients) error {
	return func(ctx context.Context, c *clients.Clients) error {
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context canceled")
			case <-ticker.C:
				if len(namespace) == 0 {
					logrus.Debug("Listing all namespaces")
					err := sync(c, "")
					if err != nil {
						logrus.Warn(err)
					}
					continue
				}

				for _, n := range namespace {
					err := sync(c, n)
					if err != nil {
						logrus.Warn(err)
					}
				}
			}
		}
	}
}

func sync(c *clients.Clients, namespace string) error {

	list, err := c.OS.ManagedOSVersionChannel().List(namespace, v1.ListOptions{})
	if err != nil {
		return err
	}

	//TODO collect all errors
	versions := map[string][]provv1.ManagedOSVersion{}
	for _, c := range list.Items {
		s, err := NewManagedOSVersionChannelSyncer(c.Spec)
		if err != nil {
			return err
		}

		vers, err := s.sync()
		if err != nil {
			return err
		}
		if _, ok := versions[c.Namespace]; !ok {
			versions[c.Namespace] = []provv1.ManagedOSVersion{}
		}

		versions[c.Namespace] = append(versions[c.Namespace], vers...)
	}

	// TODO: collect all errors
	for ns, vv := range versions {
		for _, v := range vv {
			cli := c.OS.ManagedOSVersion()

			_, err := cli.Get(namespace, v.ObjectMeta.Name, metav1.GetOptions{})
			if err == nil {
				logrus.Debugf("there is already a version defined for %s(%s)", v.Name, v.Spec.Version)
				continue
			}
			vcpy := v.DeepCopy()
			vcpy.ObjectMeta.Namespace = ns
			_, err = cli.Create(vcpy)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
