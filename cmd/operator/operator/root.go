/*
Copyright © 2022 - 2024 SUSE LLC

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

package operator

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	goruntime "runtime"
	"time"

	fleetv1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/steve/pkg/aggregation"
	upgradev1 "github.com/rancher/system-upgrade-controller/pkg/apis/upgrade.cattle.io/v1"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/controllers"
	"github.com/rancher/elemental-operator/pkg/clients"
	"github.com/rancher/elemental-operator/pkg/log"
	"github.com/rancher/elemental-operator/pkg/server"
	"github.com/rancher/elemental-operator/pkg/version"
)

// IMPORTANT: The RBAC permissions below should be reviewed after old code is deprecated.
// +kubebuilder:rbac:groups="",resources=events,verbs=patch;create
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;create;delete;list;watch
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get
// +kubebuilder:rbac:groups="",resources=services,verbs=get;create;delete;list;watch
// +kubebuilder:rbac:groups="ipam.cluster.x-k8s.io",resources=ipaddressclaims,verbs=get;create;delete;list;watch

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

type rootConfig struct {
	debug                       bool
	enableLeaderElection        bool
	profilerAddress             string
	metricsBindAddr             string
	syncPeriod                  time.Duration
	leaderElectionLeaseDuration time.Duration
	leaderElectionRenewDeadline time.Duration
	leaderElectionRetryPeriod   time.Duration
	webhookPort                 int
	webhookCertDir              string
	healthAddr                  string
	defaultRegistry             string
	operatorImage               string
	watchNamespace              string
	seedimageImage              string
	seedimageImagePullPolicy    string
}

func init() {
	klog.InitFlags(nil)

	utilruntime.Must(elementalv1.AddToScheme(scheme))
	utilruntime.Must(managementv3.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(upgradev1.AddToScheme(scheme))
	utilruntime.Must(fleetv1.AddToScheme(scheme))
	utilruntime.Must(ipamv1.AddToScheme(scheme))
}

func NewOperatorCommand() *cobra.Command {
	var config rootConfig

	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Run the Kubernetes operator using kubebuilder.",
		Args: func(_ *cobra.Command, _ []string) error {
			if config.seedimageImagePullPolicy != string(corev1.PullAlways) &&
				config.seedimageImagePullPolicy != string(corev1.PullIfNotPresent) &&
				config.seedimageImagePullPolicy != string(corev1.PullNever) {
				return fmt.Errorf("invalid pull policy '%s', valid values: '%s', '%s', '%s'",
					config.seedimageImagePullPolicy,
					corev1.PullAlways, corev1.PullIfNotPresent, corev1.PullNever)
			}
			return nil
		},
		Run: func(_ *cobra.Command, _ []string) {
			if config.debug {
				log.EnableDebugLogging()
			}

			log.Infof("Operator version %s, architecture %s, commit %s, commit date %s", version.Version, goruntime.GOARCH, version.Commit, version.CommitDate)
			operatorRun(&config)
		},
	}

	viper.AutomaticEnv()

	cmd.PersistentFlags().StringVar(&config.profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")
	_ = viper.BindPFlag("profiler-address", cmd.PersistentFlags().Lookup("profiler-address"))

	cmd.PersistentFlags().StringVar(&config.metricsBindAddr, "metrics-bind-addr", ":8080",
		"The address the metric endpoint binds to.")
	_ = viper.BindPFlag("metrics-bind-addr", cmd.PersistentFlags().Lookup("metrics-bind-addr"))

	cmd.PersistentFlags().DurationVar(&config.leaderElectionLeaseDuration, "leader-elect-lease-duration", 15*time.Second,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)")
	_ = viper.BindPFlag("leader-elect-lease-duration", cmd.PersistentFlags().Lookup("leader-elect-lease-duration"))

	cmd.PersistentFlags().DurationVar(&config.leaderElectionRetryPeriod, "leader-elect-retry-period", 10*time.Second,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)")
	_ = viper.BindPFlag("leader-elect-retry-period", cmd.PersistentFlags().Lookup("leader-elect-retry-period"))

	cmd.PersistentFlags().BoolVar(&config.enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	_ = viper.BindPFlag("leader-elect", cmd.PersistentFlags().Lookup("leader-elect"))

	cmd.PersistentFlags().IntVar(&config.webhookPort, "webhook-port", 9443,
		"Webhook Server port.")
	_ = viper.BindPFlag("webhook-port", cmd.PersistentFlags().Lookup("webhook-port"))

	cmd.PersistentFlags().StringVar(&config.webhookCertDir, "webhook-cert-dir", ":8080",
		"Webhook cert dir, only used when webhook-port is specified.")
	_ = viper.BindPFlag("webhook-cert-dir", cmd.PersistentFlags().Lookup("webhook-cert-dir"))

	cmd.PersistentFlags().StringVar(&config.healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")
	_ = viper.BindPFlag("health-addr", cmd.PersistentFlags().Lookup("health-addr"))

	cmd.PersistentFlags().StringVar(&config.defaultRegistry, "default-registry", "", "default registry to prepend to os images")
	_ = viper.BindPFlag("default-registry", cmd.PersistentFlags().Lookup("default-registry"))

	cmd.PersistentFlags().StringVar(&config.operatorImage, "operator-image", "",
		"Operator image. Used to gather the results from the syncer by running the 'display' command")
	_ = viper.BindPFlag("operator-image", cmd.PersistentFlags().Lookup("operator-image"))
	_ = cobra.MarkFlagRequired(cmd.PersistentFlags(), "operator-image")

	cmd.PersistentFlags().StringVar(&config.watchNamespace, "namespace", "", "Namespace that the controller watches to reconcile objects.")
	_ = viper.BindPFlag("namespace", cmd.PersistentFlags().Lookup("namespace"))

	cmd.PersistentFlags().BoolVar(&config.debug, "debug", false, "registration debug logging")
	_ = viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))

	cmd.PersistentFlags().StringVar(&config.seedimageImage, "seedimage-image", "", "SeedImage builder image. Used to build a SeedImage ISO.")
	_ = viper.BindPFlag("seedimage-image", cmd.PersistentFlags().Lookup("seedimage-image"))

	cmd.PersistentFlags().StringVar(&config.seedimageImagePullPolicy, "seedimage-image-pullpolicy", "IfNotPresent", "PullPolicy for the SeedImage builder image.")
	_ = viper.BindPFlag("seedimage-image-pullpolicy", cmd.PersistentFlags().Lookup("seedimage-image-pullpolicy"))

	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	return cmd
}

func operatorRun(config *rootConfig) {
	if config.profilerAddress != "" {
		klog.Infof("Profiler listening for requests at %s", config.profilerAddress)
		go func() {
			klog.Info(http.ListenAndServe(config.profilerAddress, nil))
		}()
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: config.metricsBindAddr,
		},
		LeaderElection:   config.enableLeaderElection,
		LeaderElectionID: "controller-leader-election-elemental-operator",
		LeaseDuration:    &config.leaderElectionLeaseDuration,
		RenewDeadline:    &config.leaderElectionRenewDeadline,
		RetryPeriod:      &config.leaderElectionRetryPeriod,
		Cache: cache.Options{
			SyncPeriod: &config.syncPeriod,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    config.webhookPort,
			CertDir: config.webhookCertDir,
		}),
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
					&elementalv1.ManagedOSVersion{},
				},
			},
		},
		HealthProbeBindAddress: config.healthAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	setupChecks(mgr)
	setupReconcilers(mgr, config)

	// +kubebuilder:scaffold:builder
	runRegistration(ctx, mgr, config.watchNamespace)
	runManager(ctx, mgr)
}

func runManager(ctx context.Context, mgr ctrl.Manager) {
	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func runRegistration(ctx context.Context, mgr ctrl.Manager, namespace string) {
	setupLog.Info("starting registration")
	restConfig, err := runtimeconfig.GetConfig()
	if err != nil {
		setupLog.Error(err, "Failed to find kubeconfig")
		os.Exit(1)
	}

	cl, err := clients.NewFromConfig(restConfig)
	if err != nil {
		setupLog.Error(err, "Error building restconfig")
		os.Exit(1)
	}

	aggregation.Watch(ctx, cl.Core().Secret(), namespace, "elemental-operator", server.New(ctx, mgr.GetClient()))

	if err := cl.Start(ctx); err != nil {
		setupLog.Error(err, "problem running registration")
		os.Exit(1)
	}
}

func setupChecks(mgr ctrl.Manager) {
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create health check")
		os.Exit(1)
	}
}

func setupReconcilers(mgr ctrl.Manager, config *rootConfig) {
	if err := (&controllers.MachineRegistrationReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "MachineRegistration")
		os.Exit(1)
	}
	if err := (&controllers.MachineInventoryReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "MachineInventory")
		os.Exit(1)
	}
	if err := (&controllers.MachineInventorySelectorReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "MachineInventorySelector")
		os.Exit(1)
	}
	if err := (&controllers.ManagedOSImageReconciler{
		Client:          mgr.GetClient(),
		DefaultRegistry: config.defaultRegistry,
		Scheme:          scheme,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "ManagedOSImage")
		os.Exit(1)
	}
	if err := (&controllers.ManagedOSVersionChannelReconciler{
		Client:        mgr.GetClient(),
		OperatorImage: config.operatorImage,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "ManagedOSVersionChannel")
		os.Exit(1)
	}
	if err := (&controllers.SeedImageReconciler{
		Client:                   mgr.GetClient(),
		SeedImageImage:           config.seedimageImage,
		SeedImageImagePullPolicy: corev1.PullPolicy(config.seedimageImagePullPolicy),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "SeedImage")
		os.Exit(1)
	}
	if err := (&controllers.ManagedOSVersionReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "ManagedOSVersion")
		os.Exit(1)
	}
	if err := (&controllers.ManagedOSChangelogReconciler{
		Client:                   mgr.GetClient(),
		Scheme:                   mgr.GetScheme(),
		WorkerPodImage:           config.seedimageImage,
		WorkerPodImagePullPolicy: corev1.PullPolicy(config.seedimageImagePullPolicy),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create reconciler", "controller", "ManagedOSChangelog")
		os.Exit(1)
	}
}
