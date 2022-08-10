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

package operatorCmd

import (
	"time"

	"github.com/rancher/elemental-operator/pkg/operator"
	"github.com/rancher/elemental-operator/pkg/services/syncer"
	"github.com/rancher/elemental-operator/pkg/types"
	"github.com/rancher/elemental-operator/pkg/version"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type rootConfig struct {
	Debug           bool
	SyncInterval    time.Duration
	Namespace       string
	DefaultRegistry string
	OperatorImage   string
	SyncNamespaces  []string
}

func NewOperatorCommand() *cobra.Command {
	var config rootConfig

	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Run the Kubernetes operator.",
		Run: func(_ *cobra.Command, _ []string) {
			if config.Debug {
				logrus.SetLevel(logrus.DebugLevel)
			}
			logrus.Infof("Operator version %s, commit %s, commit date %s", version.Version, version.Commit, version.CommitDate)
			operatorRun(&config)
		},
	}

	viper.AutomaticEnv()

	cmd.PersistentFlags().StringVar(&config.Namespace, "namespace", "", "namespace to run the operator on")
	_ = viper.BindPFlag("namespace", cmd.PersistentFlags().Lookup("namespace"))
	_ = cobra.MarkFlagRequired(cmd.PersistentFlags(), "namespace")

	cmd.PersistentFlags().StringArrayVar(&config.SyncNamespaces, "sync-namespaces", []string{}, "namespace to watch for machine registrations")
	_ = viper.BindPFlag("sync-namespaces", cmd.PersistentFlags().Lookup("sync-namespaces"))

	cmd.PersistentFlags().StringVar(&config.OperatorImage, "operator-image", "", "Operator image. Used to gather the results from the syncer by running the 'display' command")
	_ = viper.BindPFlag("operator-image", cmd.PersistentFlags().Lookup("operator-image"))
	_ = cobra.MarkFlagRequired(cmd.PersistentFlags(), "operator-image")

	cmd.PersistentFlags().StringVar(&config.DefaultRegistry, "default-registry", "", "default registry to prepend to os images")
	_ = viper.BindPFlag("default-registry", cmd.PersistentFlags().Lookup("default-registry"))

	cmd.PersistentFlags().DurationVar(&config.SyncInterval, "sync-interval", 60*time.Minute, "how often to check for new os versions")
	_ = viper.BindPFlag("sync-interval", cmd.PersistentFlags().Lookup("sync-interval"))

	cmd.PersistentFlags().BoolVar(&config.Debug, "debug", false, "enable debug logging")
	_ = viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))

	return cmd
}

func operatorRun(config *rootConfig) {
	ctx := signals.SetupSignalContext()

	logrus.Infof("Starting controller at namespace %s. Upgrade sync interval at: %s", config.Namespace, config.SyncInterval)

	// We do want a stack for requeuer here, but we want the syncer to
	// tick sequentially. We can turn the behavior the other way around
	// by setting UpgradeChannelSync concurrent to true.
	requeuer := types.ConcurrentRequeuer(100)

	if err := operator.Run(ctx,
		operator.WithRequeuer(requeuer),
		operator.WithNamespace(config.Namespace),
		operator.WithDefaultRegistry(config.DefaultRegistry),
		operator.WithServices(syncer.UpgradeChannelSync(config.SyncInterval, requeuer, config.OperatorImage, false, config.SyncNamespaces...)),
	); err != nil {
		logrus.Fatal(err)
	}

	<-ctx.Done()
}
