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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradeChannelSync returns a service to keep in sync managedosversions available for upgrade
func UpgradeChannelSync(interval time.Duration, namespace string) func(context.Context, *clients.Clients) error {
	fmt.Println("Starting syncer service")
	return func(ctx context.Context, c *clients.Clients) error {
		ticker := time.NewTicker(interval)

		fmt.Println("Ticker starting")
		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context canceled")
			case <-ticker.C:
				err := sync(c, namespace)
				if err != nil {
					fmt.Println(err.Error())
				}
			}
		}
	}
}

func sync(c *clients.Clients, namespace string) error {

	fmt.Printf("sync from service: %s\n", namespace)
	list, err := c.OS.ManagedOSVersionChannel().List(namespace, v1.ListOptions{})
	if err != nil {
		return err
	}

	fmt.Println(list.Items)

	//TODO collect all errors
	versions := []provv1.ManagedOSVersion{}
	for _, c := range list.Items {
		s, err := NewManagedOSVersionChannelSyncer(c.Spec)
		if err != nil {
			return err
		}

		vers, err := s.sync()
		if err != nil {
			return err
		}
		versions = append(versions, vers...)
	}

	// TODO: collect all errors
	for _, v := range versions {
		cli := c.OS.ManagedOSVersion()
		
		_, err := cli.Get(namespace, v.ObjectMeta.Name, metav1.GetOptions{})
		if err == nil {
			//TODO some warning message would be nice
			continue
		}
		vcpy := v.DeepCopy()
		vcpy.ObjectMeta.Namespace = namespace
		_, err = cli.Create(vcpy)
		if err == nil {
			return err
		}
	}
	return nil
}
