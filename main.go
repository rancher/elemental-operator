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
	"flag"
	"os"
	"strings"
	"time"

	"github.com/rancher-sandbox/rancheros-operator/pkg/operator"
	"github.com/rancher-sandbox/rancheros-operator/pkg/services"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
)

var (
	namespace  = flag.String("namespace", "cattle-rancheros-operator-system", "Namespace of the pod")
	namespaces = flag.String("namespaces", "", "A comma separated list of namespaces to watch")
	interval   = flag.String("sync-interval", "60m", "Interval for the upgrade channel ticker")
)

func main() {
	flag.Parse()
	logrus.Info("Starting controller")
	ctx := signals.SetupSignalContext()

	if os.Getenv("NAMESPACE") != "" {
		*namespace = os.Getenv("NAMESPACE")
	}

	var ns []string
	if os.Getenv("WATCH_NAMESPACE") != "" {
		*namespaces = os.Getenv("WATCH_NAMESPACE")
	}

	if *namespaces != "" {
		ns = strings.Split(*namespaces, ",")
	}

	ticker, err := time.ParseDuration(*interval)
	if err != nil {
		logrus.Fatalf("sync-interval value cant be parsed as duration: %s", err)
	}

	if err := operator.Run(ctx,
		operator.WithNamespace(*namespace),
		operator.WithServices(services.UpgradeChannelSync(ticker, ns...)),
	); err != nil {
		logrus.Fatalf("Error starting: %s", err.Error())
	}

	<-ctx.Done()
}
