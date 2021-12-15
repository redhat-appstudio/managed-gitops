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

	v1 "k8s.io/api/core/v1"
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
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"

	managedgitopsv1alpha1operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	managedgitopscontrollers "github.com/redhat-appstudio/managed-gitops/backend/controllers/managed-gitops"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop"
	"github.com/redhat-appstudio/managed-gitops/backend/routes"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(managedgitopsv1alpha1operation.AddToScheme(scheme))

	utilruntime.Must(managedgitopsv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
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

	go initializeRoutes()

	// TODO: GITOPS-1577 - Need to create a manager for each KCP workspace we manage (or switch to lower-level controller API, ala Argo CD gitops engine)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
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

	eventLoop := eventloop.NewEventLoop()

	if err = (&managedgitopscontrollers.GitOpsDeploymentReconciler{
		EventLoop: eventLoop,
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitOpsDeployment")
		os.Exit(1)
	}
	if err = (&managedgitopscontrollers.GitOpsDeploymentSyncRunReconciler{
		EventLoop: eventLoop,
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitOpsDeploymentSyncRun")
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

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	if err := createPrimaryGitOpsEngineInstance(mgr.GetClient(), setupLog); err != nil {
		setupLog.Error(err, "Unable to create primary GitOps engine instance")
		return
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

// createPrimaryGitOpsEngineInstance create placeholder values, for development purposes. This should not be used in production.
func createPrimaryGitOpsEngineInstance(k8sclient client.Client, actionLog logr.Logger) error {

	ctx := context.Background()

	dbQueries, err := db.NewProductionPostgresDBQueries(true)
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

	namespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "argocd",
		},
	}
	if err := k8sclient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		return fmt.Errorf("unable to retrieve gitopsengine namespace: %v", err)
	}

	kubeSystemNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}

	gitopsEngineInstance, _, err := sharedutil.UncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, *namespace, kubeSystemNamespace.Name, dbQueries, actionLog)
	if err != nil {
		return err
	}

	gitopsLocalWorkspaceNamespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gitops-local",
		},
	}

	managedEnv, err := sharedutil.UncheckedGetOrCreateManagedEnvironmentByNamespaceUID(ctx, *gitopsLocalWorkspaceNamespace, dbQueries, actionLog)
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
