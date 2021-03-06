/*
Copyright 2020 The Kubernetes Authors.

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
	"errors"
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	infrastructurev1alpha3 "github.com/tinkerbell/cluster-api-provider-tinkerbell/api/v1alpha3"
	"github.com/tinkerbell/cluster-api-provider-tinkerbell/controllers"
	// +kubebuilder:scaffold:imports
)

//nolint:gochecknoglobals
var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

//nolint:wsl,gochecknoinits
func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = infrastructurev1alpha3.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

// optionsFromFlags parse CLI flags and converts them to controller runtime options.
func optionsFromFlags() ctrl.Options {
	klog.InitFlags(nil)

	// Machine and cluster operations can create enough events to trigger the event recorder spam filter
	// Setting the burst size higher ensures all events will be recorded and submitted to the API
	broadcaster := record.NewBroadcasterWithCorrelatorOptions(record.CorrelatorOptions{
		BurstSize: 100, //nolint:gomnd
	})

	var syncPeriod time.Duration

	options := ctrl.Options{
		Scheme:           scheme,
		LeaderElectionID: "controller-leader-election-capt",
		EventBroadcaster: broadcaster,
		SyncPeriod:       &syncPeriod,
	}

	flag.BoolVar(&options.LeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	flag.StringVar(&options.LeaderElectionNamespace, "leader-election-namespace", "",
		"Namespace that the controller performs leader election in. "+
			"If unspecified, the controller will discover which namespace it is running in.",
	)

	flag.StringVar(&options.HealthProbeBindAddress, "health-addr", ":9440", "The address the health endpoint binds to.")

	flag.StringVar(&options.MetricsBindAddress, "metrics-addr", ":8080", "The address the metric endpoint binds to.")

	flag.DurationVar(&syncPeriod, "sync-period", 10*time.Minute, //nolint:gomnd
		"The minimum interval at which watched resources are reconciled (e.g. 15m)",
	)

	flag.StringVar(&options.Namespace, "namespace", "",
		"Namespace that the controller watches to reconcile cluster-api objects. "+
			"If unspecified, the controller watches for cluster-api objects across all namespaces.",
	)

	flag.IntVar(&options.Port, "webhook-port", 0,
		"Webhook Server port, disabled by default. When enabled, the manager will only "+
			"work as webhook server, no reconcilers are installed.",
	)

	flag.Parse()

	return options
}

func validateOptions(options ctrl.Options) error {
	if options.Namespace != "" {
		setupLog.Info("Watching cluster-api objects only in namespace for reconciliation", "namespace", options.Namespace)
	}

	if options.Port != 0 {
		// TODO: add the webhook configuration
		return errors.New("webhook not implemented")
	}

	return nil
}

func main() {
	ctrl.SetLogger(klogr.New())

	options := optionsFromFlags()

	if err := validateOptions(options); err != nil {
		setupLog.Error(err, "validating controllers configuration")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// TODO: Get a Tinkerbell client.

	if err = (&controllers.TinkerbellClusterReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("TinerellCluster"),
		Recorder: mgr.GetEventRecorderFor("tinerellcluster-controller"),
		Scheme:   mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TinkerbellCluster")
		os.Exit(1)
	}

	if err = (&controllers.TinkerbellMachineReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("TinkerbellMachine"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("tinkerbellmachine-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TinkerbellMachine")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to create health check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
