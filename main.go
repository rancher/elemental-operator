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

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/rancher-sandbox/rancheros-operator/pkg/operator"
	"github.com/rancher-sandbox/rancheros-operator/pkg/services"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	app := &cli.App{
		Name:        "rancheros-operator",
		Version:     "", // TODO: bind internal.Version to CI while building with ldflags
		Author:      "",
		Usage:       "",
		Description: "",
		Copyright:   "",
		Commands: []cli.Command{
			{
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "namespace",
						EnvVar:   "NAMESPACE",
						Usage:    "Namespace of the pod",
						Required: true,
					},
					&cli.StringSliceFlag{
						Name:     "sync-namespaces",
						EnvVar:   "SYNC_NAMESPACE",
						Usage:    "List of namespaces to watch",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "sync-interval",
						EnvVar:   "SYNC_INTERVAL",
						Usage:    "Interval for the upgrade channel sync daemon",
						Value:    "60m",
						Required: true,
					},
				},
				Name:   "start-operator",
				Action: runOperator,
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		logrus.Error(err)
		os.Exit(1)
	}
}

func runOperator(c *cli.Context) error {

	ctx := signals.SetupSignalContext()

	interval := c.String("sync-interval")
	namespace := c.String("namespace")

	logrus.Infof("Starting controller at namespace %s. Upgrade sync interval at: %s", namespace, interval)

	ticker, err := time.ParseDuration(interval)
	if err != nil {
		logrus.Fatalf("sync-interval value cant be parsed as duration: %s", err)
	}

	if err := operator.Run(ctx,
		operator.WithNamespace(namespace),
		operator.WithServices(services.UpgradeChannelSync(ticker, c.StringSlice("sync-namespaces")...)),
	); err != nil {
		return err
	}

	<-ctx.Done()

	return fmt.Errorf("operator shouldn't return")
}
