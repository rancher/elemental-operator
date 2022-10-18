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

package ctrlruntimeoperator

import (
	"net/http"
	"os"
	"time"

	elementalv1 "github.com/rancher/elemental-operator/api/v1beta1"
	"github.com/rancher/elemental-operator/controllers"
	"github.com/rancher/elemental-operator/pkg/version"
	managementv3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

// IMPORTANT: The RBAC permissions below should be reviewed after old code is deprecated.

// +kubebuilder:rbac:groups="",resources=events,verbs=patch;create
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;create;delete;list;watch
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get
// +kubebuilder:rbac:groups="fleet.cattle.io",resources=bundles,verbs=*
// +kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=*

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

type rootConfig struct {
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
}

func init() {
	utilruntime.Must(elementalv1.AddToScheme(scheme))
	utilruntime.Must(managementv3.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
}

func NewOperatorCtrlRuntimeCommand() *cobra.Command {
	var config rootConfig

	cmd := &cobra.Command{
		Use:   "operator-kubebuilder",
		Short: "Run the Kubernetes operator using kubebuilder.",
		Run: func(_ *cobra.Command, _ []string) {
			logrus.Infof("Operator version %s, commit %s, commit date %s", version.Version, version.Commit, version.CommitDate)
			operatorRun(&config)
		},
	}

	viper.AutomaticEnv()

	cmd.PersistentFlags().StringVar(&config.profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")
	_ = viper.BindPFlag("profiler-address", cmd.PersistentFlags().Lookup("profiler-address"))

	cmd.PersistentFlags().StringVar(&config.metricsBindAddr, "metrics-bind-addr", ":8080",
		"The address the metric endpoint binds to.")
	_ = viper.BindPFlag("profiler-address", cmd.PersistentFlags().Lookup("profiler-address"))

	cmd.PersistentFlags().DurationVar(&config.syncPeriod, "sync-period", 10*time.Minute,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")
	_ = viper.BindPFlag("sync-interval", cmd.PersistentFlags().Lookup("sync-interval"))

	cmd.PersistentFlags().DurationVar(&config.leaderElectionLeaseDuration, "leader-elect-lease-duration", 15*time.Second,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)")
	_ = viper.BindPFlag("sync-interval", cmd.PersistentFlags().Lookup("sync-interval"))

	cmd.PersistentFlags().DurationVar(&config.leaderElectionRenewDeadline, "leader-elect-renew-deadlined", 10*time.Second,
		"Duration that the leading controller manager will retry refreshing leadership before giving up (duration string)")
	_ = viper.BindPFlag("sync-interval", cmd.PersistentFlags().Lookup("sync-interval"))

	cmd.PersistentFlags().DurationVar(&config.leaderElectionRetryPeriod, "leader-elect-retry-period", 2*time.Second,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)")
	_ = viper.BindPFlag("sync-interval", cmd.PersistentFlags().Lookup("sync-interval"))

	cmd.PersistentFlags().BoolVar(&config.enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	_ = viper.BindPFlag("leader-elect", cmd.PersistentFlags().Lookup("leader-elect"))

	cmd.PersistentFlags().IntVar(&config.webhookPort, "webhook-port", 9443,
		"Webhook Server port.")
	_ = viper.BindPFlag("leader-elect", cmd.PersistentFlags().Lookup("leader-elect"))

	cmd.PersistentFlags().StringVar(&config.webhookCertDir, "webhook-cert-dir", ":8080",
		"Webhook cert dir, only used when webhook-port is specified.")
	_ = viper.BindPFlag("profiler-address", cmd.PersistentFlags().Lookup("profiler-address"))

	cmd.PersistentFlags().StringVar(&config.healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")
	_ = viper.BindPFlag("profiler-address", cmd.PersistentFlags().Lookup("profiler-address"))

	return cmd
}

func operatorRun(config *rootConfig) {
	ctrl.SetLogger(klogr.New())

	if config.profilerAddress != "" {
		klog.Infof("Profiler listening for requests at %s", config.profilerAddress)
		go func() {
			klog.Info(http.ListenAndServe(config.profilerAddress, nil))
		}()
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: config.metricsBindAddr,
		LeaderElection:     config.enableLeaderElection,
		LeaderElectionID:   "controller-leader-election-elemental-operator",
		LeaseDuration:      &config.leaderElectionLeaseDuration,
		RenewDeadline:      &config.leaderElectionRenewDeadline,
		RetryPeriod:        &config.leaderElectionRetryPeriod,
		SyncPeriod:         &config.syncPeriod,
		ClientDisableCacheFor: []client.Object{
			&corev1.ConfigMap{},
			&corev1.Secret{},
		},
		Port:                   config.webhookPort,
		CertDir:                config.webhookCertDir,
		HealthProbeBindAddress: config.healthAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	setupChecks(mgr)
	setupReconcilers(mgr)

	// +kubebuilder:scaffold:builder
	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
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

func setupReconcilers(mgr ctrl.Manager) {
	if err := (&controllers.MachineRegistrationReconciler{
		Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create reconciler", "controller", "MachineRegistration")
		os.Exit(1)
	}
}
