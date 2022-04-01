package services

import (
	"context"
	"fmt"
	"time"

	"github.com/rancher-sandbox/rancheros-operator/pkg/clients"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradeChannelSync returns a service to keep in sync managedosversions available for upgrade
func UpgradeChannelSync(interval time.Duration, namespace string) func(context.Context, *clients.Clients) error {
	return func(ctx context.Context, c *clients.Clients) error {
		ticker := time.NewTicker(interval)

		for {
			select {
			case <-ctx.Done():

				return fmt.Errorf("context canceled")
			case <-ticker.C:
				sync(c, namespace)
			}
		}

		return nil
	}
}

func sync(c *clients.Clients, namespace string) error {

	list, err := c.OS.ManagedOSVersionChannel().List(namespace, v1.ListOptions{})
	if err != nil {
		return err
	}

	for _, c := range list.Items {
		t := c.Spec.Type
		opts := c.Spec.Options

		

	}

}
