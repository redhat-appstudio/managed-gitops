/*
Copyright 2021.

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
	"log"
	"net/http"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/redhat-appstudio/managed-gitops/utilities/db-migration/migrate"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	managedgitopscontrollers "github.com/redhat-appstudio/managed-gitops/backend/controllers/managed-gitops"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/preprocess_event_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/routes"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName(logutil.LogLogger_managed_gitops)
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(managedgitopsv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var profilerAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":18080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":18081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&profilerAddr, "profiler-address", ":6060", "The address for serving pprof profiles")

	opts := crzap.Options{
		Development: true,
	}

	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(crzap.New(crzap.UseFlagOptions(&opts)))

	if sharedutil.IsProfilingEnabled() {
		setupLog.Info("Starting pprof profiler server", "address", profilerAddr)
		go sharedutil.StartProfilers(profilerAddr)
	}

	ctx := ctrl.SetupSignalHandler()

	// Default to the backend running from backend folder
	migrationsPath := "file://../utilities/db-migration/migrations/"

	// If the /migrations path exists, when the backend is running in a container, use that instead.
	_, err := os.Stat("/migrations")
	if !os.IsNotExist(err) {
		migrationsPath = "file:///migrations"
	}

	if err := migrate.Migrate("", migrationsPath); err != nil {
		setupLog.Error(err, "Fatal Error: Unsuccessful Migration")
		os.Exit(1)
	}

	restConfig, err := sharedutil.GetRESTConfig()
	if err != nil {
		setupLog.Error(err, "unable to get kubeconfig")
		os.Exit(1)
		return
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "5a3f596c.redhat.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	preprocessEventLoop := preprocess_event_loop.NewPreprocessEventLoop()

	if err = (&managedgitopscontrollers.GitOpsDeploymentReconciler{
		PreprocessEventLoop: preprocessEventLoop,
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitOpsDeployment")
		os.Exit(1)
	}
	if err = (&managedgitopscontrollers.GitOpsDeploymentSyncRunReconciler{
		PreprocessEventLoop: preprocessEventLoop,
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitOpsDeploymentSyncRun")
		os.Exit(1)
	}
	if err = (&managedgitopscontrollers.GitOpsDeploymentRepositoryCredentialReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		PreprocessEventLoop: preprocessEventLoop,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitOpsDeploymentRepositoryCredential")
		os.Exit(1)
	}
	if err = (&managedgitopscontrollers.GitOpsDeploymentManagedEnvironmentReconciler{
		Client:                       mgr.GetClient(),
		Scheme:                       mgr.GetScheme(),
		PreprocessEventLoopProcessor: managedgitopscontrollers.NewDefaultPreProcessEventLoopProcessor(preprocessEventLoop),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitOpsDeploymentManagedEnvironment")
		os.Exit(1)
	}
	if err = (&managedgitopscontrollers.SecretReconciler{
		Client:                       mgr.GetClient(),
		Scheme:                       mgr.GetScheme(),
		PreprocessEventLoopProcessor: managedgitopscontrollers.NewDefaultPreProcessEventLoopProcessor(preprocessEventLoop),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitOpsDeploymentManagedEnvironment")
		os.Exit(1)
	}

	// If the webhook is not disabled, start listening on the webhook URL
	if !strings.EqualFold(os.Getenv("DISABLE_APPSTUDIO_WEBHOOK"), "true") {

		setupLog.Info("setting up webhooks")

		if err = (&managedgitopsv1alpha1.GitOpsDeployment{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "GitOpsDeployment")
			os.Exit(1)
		}
		if err = (&managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "GitOpsDeploymentRepositoryCredential")
			os.Exit(1)
		}
		if err = (&managedgitopsv1alpha1.GitOpsDeploymentSyncRun{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "GitOpsDeploymentSyncRun")
			os.Exit(1)
		}
		if err = (&managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "GitOpsDeploymentManagedEnvironment")
			os.Exit(1)
		}

	}

	//+kubebuilder:scaffold:builder

	startDBReconciler(mgr)
	startRepoCredReconciler(mgr)
	startDBMetricsReconciler(mgr)
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// if err := createPrimaryGitOpsEngineInstance(mgr.GetClient(), setupLog); err != nil {
	// 	setupLog.Error(err, "Unable to create primary GitOps engine instance")
	// 	return
	// }

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

}

func startDBReconciler(mgr ctrl.Manager) {

	dbQueries, err := db.NewSharedProductionPostgresDBQueries(false)
	if err != nil {
		setupLog.Error(err, "never able to connect to database")
		os.Exit(1)
	}

	databaseReconciler := eventloop.DatabaseReconciler{
		DB:               dbQueries,
		Client:           mgr.GetClient(),
		K8sClientFactory: shared_resource_loop.DefaultK8sClientFactory{},
	}

	// Start goroutine for database reconciler
	databaseReconciler.StartDatabaseReconciler()
}

func startRepoCredReconciler(mgr ctrl.Manager) {

	dbQueries, err := db.NewSharedProductionPostgresDBQueries(false)
	if err != nil {
		setupLog.Error(err, "never able to connect to database")
		os.Exit(1)
	}

	repoCredReconciler := eventloop.RepoCredReconciler{
		DB:     dbQueries,
		Client: mgr.GetClient(),
	}

	// Start goroutine for Repository Credential reconciler
	repoCredReconciler.StartRepoCredReconciler()
}

func startDBMetricsReconciler(mgr ctrl.Manager) {

	dbQueries, err := db.NewSharedProductionPostgresDBQueries(false)
	if err != nil {
		setupLog.Error(err, "never able to connect to database")
		os.Exit(1)
	}

	databaseReconciler := eventloop.MetricsReconciler{
		DB:     dbQueries,
		Client: mgr.GetClient(),
	}

	// Start goroutine for database metrics reconciler
	databaseReconciler.StartDBMetricsReconcilerForMetrics()
}

// nolint:unused
func initializeRoutes() {

	// Intializing the server for routing endpoints
	router := routes.RouteInit()
	err := router.ListenAndServe()
	if err != http.ErrServerClosed {
		log.Println("Error on ListenAndServe:", err)
	}

}
