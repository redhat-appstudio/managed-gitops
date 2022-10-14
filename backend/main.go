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
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/utilities/db-migration/migrate"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	managedgitopscontrollers "github.com/redhat-appstudio/managed-gitops/backend/controllers/managed-gitops"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/preprocess_event_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/routes"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
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
	var apiExportName string
	flag.StringVar(&apiExportName, "api-export-name", "gitopsrvc-backend-shared", "The name of the APIExport.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":18080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":18081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

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
	go initializeRoutes()

	restConfig, err := sharedutil.GetRESTConfig()
	if err != nil {
		setupLog.Error(err, "unable to get kubeconfig")
		os.Exit(1)
		return
	}

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "5a3f596c.redhat.com",
		LeaderElectionConfig:   restConfig,
	}

	mgr, err := sharedutil.GetControllerManager(ctx, restConfig, &setupLog, apiExportName, options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	preprocessEventLoop := preprocess_event_loop.NewPreprocessEventLoop(apiExportName)

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
	//+kubebuilder:scaffold:builder

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

func initializeRoutes() {

	// Intializing the server for routing endpoints
	router := routes.RouteInit()
	err := router.ListenAndServe()
	if err != http.ErrServerClosed {
		log.Println("Error on ListenAndServe:", err)
	}

}

// nolint
// createPrimaryGitOpsEngineInstance create placeholder values, for development purposes. This should not be used in production.
func createPrimaryGitOpsEngineInstance(k8sclient client.Client, log logr.Logger) error {

	ctx := context.Background()

	dbQueries, err := db.NewSharedProductionPostgresDBQueries(false)
	if err != nil {
		return err
	}

	// Create the fake cluster user if they don't exist
	clusterUser := db.ClusterUser{User_name: "gitops-service-user"}
	if err := dbQueries.GetClusterUserByUsername(ctx, &clusterUser); err != nil {
		if db.IsResultNotFoundError(err) {
			if err = dbQueries.CreateClusterUser(ctx, &clusterUser); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "argocd",
		},
	}
	if err := k8sclient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		return fmt.Errorf("unable to retrieve gitopsengine namespace: %v", err)
	}

	kubeSystemNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}

	gitopsEngineInstance, _, _, err := dbutil.GetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, *namespace, kubeSystemNamespace.Name, dbQueries, log)
	if err != nil {
		return err
	}

	gitopsLocalWorkspaceNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gitops-local",
		},
	}

	managedEnv, _, err := dbutil.GetOrCreateManagedEnvironmentByNamespaceUID(ctx, *gitopsLocalWorkspaceNamespace, dbQueries, log)
	if err != nil {
		return err
	}

	clusterAccess := &db.ClusterAccess{
		Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    managedEnv.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
	}

	if err := dbQueries.CreateClusterAccess(ctx, clusterAccess); err != nil {
		return err
	}

	return nil

}
