package operator_cmd

import (
	"github.com/rancher/elemental-operator/pkg/operator"
	"github.com/rancher/elemental-operator/pkg/services/syncer"
	"github.com/rancher/elemental-operator/pkg/types"
	"github.com/rancher/elemental-operator/pkg/version"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"time"
)

type rootConfig struct {
	Debug            bool
	SyncInterval     time.Duration
	Namespace        string
	DefaultRegistry  string
	RancherServerURL string
	CACert           string
	OperatorImage    string
}

func NewOperatorCommand() *cobra.Command {
	var config rootConfig

	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Run the Kubernetes operator.",
		Run: func(_ *cobra.Command, _ []string) {
			operatorRun(&config)
		},
	}

	viper.AutomaticEnv()
	cmd.PersistentFlags().StringVar(&config.RancherServerURL, "rancher-server-url", "", "URL used to build registration url")
	_ = viper.BindPFlag("rancher-server-url", cmd.PersistentFlags().Lookup("rancher-server-url"))
	_ = cobra.MarkFlagRequired(cmd.PersistentFlags(), "rancher-server-url")

	cmd.PersistentFlags().StringVar(&config.Namespace, "namespace", "", "namespace to watch for machine registrations")
	_ = viper.BindPFlag("namespace", cmd.PersistentFlags().Lookup("namespace"))
	_ = cobra.MarkFlagRequired(cmd.PersistentFlags(), "namespace")

	cmd.PersistentFlags().StringVar(&config.OperatorImage, "operator-image", "rancher/elemental-operator:"+version.Version, "this image")
	_ = viper.BindPFlag("operator-image", cmd.PersistentFlags().Lookup("operator-image"))
	_ = cobra.MarkFlagRequired(cmd.PersistentFlags(), "operator-image")

	cmd.PersistentFlags().StringVar(&config.DefaultRegistry, "default-registry", "", "default registry to prepend to os images")
	_ = viper.BindPFlag("default-registry", cmd.PersistentFlags().Lookup("default-registry"))

	cmd.PersistentFlags().StringVar(&config.CACert, "ca-cert", "", "Rancher URL CA certificate")
	_ = viper.BindPFlag("ca-cert", cmd.PersistentFlags().Lookup("ca-cert"))

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
		operator.WithServerURL(config.RancherServerURL),
		operator.WithCACert(config.CACert),
		operator.WithDefaultRegistry(config.DefaultRegistry),
		operator.WithServices(syncer.UpgradeChannelSync(config.SyncInterval, requeuer, config.OperatorImage, false, config.Namespace)),
	); err != nil {
		logrus.Fatal(err)
	}

	<-ctx.Done()
}
