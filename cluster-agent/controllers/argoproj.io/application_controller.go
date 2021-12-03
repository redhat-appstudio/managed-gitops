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

package argoprojio

import (
	"context"
	"encoding/json"
	"fmt"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	database "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	util "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Application object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile

func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	contextLog := log.FromContext(ctx)

	// creating databaseQueries object to fetch all the queries functionality
	// TODO: DEBT - Convert Unsafe methods to default and unchecked
	databaseQueries, err := database.NewUnsafePostgresDBQueries(true, false)
	if err != nil {
		return ctrl.Result{}, err
	}

	// retrieve the application CR from the databases and store the value in app object
	var allApplicationinDb *[]database.Application

	// TODO: DEBT - Convert Unsafe methods to default and unchecked
	errFetchAllApp := databaseQueries.UnsafeListAllApplications(ctx, allApplicationinDb)
	if errFetchAllApp != nil {
		return ctrl.Result{}, errFetchAllApp
	}

	app := appv1.Application{}
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		contextLog.Error(err, "Unable to find Application: "+app.Name)

		// This will check whether the app.Name exists in the fetched lists from Application databases
		checkAppinDb, dbAppId := contains(*allApplicationinDb, app.Name)
		if checkAppinDb {
			// Todo: to decide what will be Application_id from Namespace Application CR perspective
			// To propose: fetch the list of application from the database and store it in an array, similarly if there is a way to fetch all the application in namespace find that
			// store that in an array, if the database.name dosen't exists in application namespace create an entry there! (But the follow up question being what if the Application Name is repicated,
			// should we set Application Name entry in database to be UNIQUE?)

			// Case 1: If nil, then Application CR is present in the database, hence create a new Application CR in the namespace
			dbApplication := &database.Application{
				Application_id: dbAppId,
			}
			// this proves that the application does exists, create in namespace
			errGetApp := databaseQueries.UnsafeGetApplicationById(ctx, dbApplication)
			if errGetApp != nil {
				return ctrl.Result{}, errGetApp
			}

			application, errConvert := dbAppToApplicationCR(dbApplication)
			if errConvert != nil {
				return ctrl.Result{}, errConvert
			}
			errAddToNamespace := r.Create(ctx, application, &client.CreateOptions{})
			if errAddToNamespace != nil {
				return ctrl.Result{}, errAddToNamespace
			}

			// Update the application state after application is pushed to Namespace
			// if ApplicationState Row exists no change, else create!

			dbApplicationState := &database.ApplicationState{Applicationstate_application_id: string(app.UID)}
			// TODO: DEBT - Convert Unsafe methods to default and unchecked
			errAppState := databaseQueries.UnsafeGetApplicationStateById(ctx, dbApplicationState)
			if errAppState != nil {
				dbApplicationState := &database.ApplicationState{
					Applicationstate_application_id: dbApplication.Application_id,
					Health:                          string(application.Status.Health.Status),
					Sync_Status:                     string(application.Status.Sync.Status),
				}

				errCreateApplicationState := databaseQueries.UnsafeCreateApplicationState(ctx, dbApplicationState)
				if errCreateApplicationState != nil {
					return ctrl.Result{}, errCreateApplicationState
				}
			}
		}
		return ctrl.Result{}, err
	}

	// Case 2: If Application CR dosen't exist in database, remove from the namespace as well

	argocdNamespace := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "argocd",
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&argocdNamespace), &argocdNamespace); err != nil {
		return ctrl.Result{}, err
	}
	// TODO: DEBT - Convert Unsafe methods to default and unchecked
	managedEnv, err := util.UncheckedGetOrCreateManagedEnvironmentByNamespaceUID(ctx, argocdNamespace, databaseQueries, contextLog)

	clusterCreds := database.ClusterCredentials{}
	{
		var clusterCredsList []database.ClusterCredentials
		// TODO: DEBT - Convert Unsafe methods to default and unchecked
		if err := databaseQueries.UnsafeListAllClusterCredentials(ctx, &clusterCredsList); err != nil {
			return ctrl.Result{}, err
		}

		if (clusterCreds == database.ClusterCredentials{}) {
			clusterCreds := database.ClusterCredentials{
				Host:                        "host",
				Kube_config:                 "kube_config",
				Kube_config_context:         "kube_config_context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "serviceaccount_ns",
			}
			if err := databaseQueries.CreateClusterCredentials(ctx, &clusterCreds); err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to create cluster creds for managed env: %v", err)
			}
		}
	}

	gitopsEngineNamespace := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: argocdNamespace.Name,
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&gitopsEngineNamespace), &gitopsEngineNamespace); err != nil {
		return ctrl.Result{}, err
	}

	kubesystemNamespace := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&kubesystemNamespace), &kubesystemNamespace); err != nil {
		return ctrl.Result{}, err
	}

	gitopsEngineInstance, _, err := util.UncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, gitopsEngineNamespace, string(kubesystemNamespace.UID), clusterCreds, databaseQueries, contextLog)

	checkApp := database.Application{
		Name:                    app.Name,
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnv.Managedenvironment_id,
	}

	var applications []database.Application
	applications = append(applications, checkApp)
	// TODO: DEBT - Convert Unsafe methods to default and unchecked
	err = databaseQueries.UnsafeListAllApplications(ctx, &applications)
	if err != nil { // if err is nil then shows the occurance of application cr in database
		// no occurence of Application CR in database
		// Case 2: If Application CR dosen't exist in database, remove from the namespace as well
		if err != nil {
			if errors.IsNotFound(err) {
				errDel := r.Delete(ctx, &app, &client.DeleteAllOfOptions{})
				if errDel != nil {
					return ctrl.Result{}, fmt.Errorf("error, %v ", errDel)
				}

			}
		}
	}

	// Case 3: If Application CR exists in database + namspace, compare!
	specDetails, _ := json.Marshal(app.Spec)
	if checkApp.Spec_field != string(specDetails) {
		newSpecErr := json.Unmarshal([]byte(checkApp.Spec_field), &app.Spec)
		if newSpecErr != nil {
			return ctrl.Result{}, newSpecErr
		}

		errUpdate := r.Update(ctx, &app, &client.UpdateOptions{})
		if errUpdate != nil {
			return ctrl.Result{}, errUpdate
		}
	}
	contextLog.Info("Application event seen in reconciler: " + app.Name)

	return ctrl.Result{}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.Application{}).
		Complete(r)
}

// getUniqueClusterID returns a unique UID that can be used to identify one individual cluster instance from another.
//
// Since I'm not aware of an exist K8s API that gives us a value like this, I am using the uid of kube-system, which is
// unlikely to change under normal circumstances.
func getUniqueClusterID(ctx context.Context, clientClient client.Client) (string, error) {

	namespace := corev1.Namespace{}

	if err := clientClient.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "kube-system"}, &namespace); err != nil {
		return "", err
	}

	return string(namespace.UID), nil
}

func getGitOpsEngineInstanceForThisCluster(ctx context.Context, gitopsEngineInstanceParam *db.GitopsEngineInstance, gitopsEngineNamespace corev1.Namespace, clientClient client.Client, ownerId string) error {

	// create cluster user (does it seems strange that an argo cd instances needs a cluster user?)
	// create cluster credentials
	// create gitops engine cluster
	// create gitops engine instance (this is fake data)

	if gitopsEngineInstanceParam == nil {
		return fmt.Errorf("nil pointer param")
	}

	dbQueries, err := db.NewProductionPostgresDBQueries(true)
	if err != nil {
		return err
	}

	// Get the ID that uniquely identifies the cluster we are running on
	uniqueClusterID, err := getUniqueClusterID(ctx, clientClient)
	if err != nil {
		return err
	}

	// Get the GitOpsEngineCluster that corresponds to this namespace
	kdb := db.KubernetesToDBResourceMapping{
		KubernetesResourceType: "Namespace",
		KubernetesResourceUID:  uniqueClusterID,
		DBRelationType:         "GitOpsEngineCluster",
	}
	// TODO: Whose job is it to create this?
	if err := dbQueries.GetDBResourceMappingForKubernetesResource(ctx, &kdb); err != nil {
		return err
	}

	gitopsEngineCluster := db.GitopsEngineCluster{
		Gitopsenginecluster_id: kdb.DBRelationKey,
	}
	if err := dbQueries.GetGitopsEngineClusterById(ctx, &gitopsEngineCluster, ownerId); err != nil {
		return err
	}

	var gitopsEngineInstances []db.GitopsEngineInstance
	if err := dbQueries.ListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx, gitopsEngineCluster.Gitopsenginecluster_id, ownerId, &gitopsEngineInstances); err != nil {
		return err
	}

	if len(gitopsEngineInstances) == 0 {
		return fmt.Errorf("no GitOpsEngineInstances were found for %v, owner of %v", gitopsEngineCluster.Gitopsenginecluster_id, ownerId)
	}

	for idx, gitopsEngineInstance := range gitopsEngineInstances {
		if gitopsEngineInstance.Namespace_name == gitopsEngineNamespace.Name &&
			gitopsEngineInstance.Namespace_uid == string(gitopsEngineNamespace.UID) {
			// Match found: return it
			*gitopsEngineInstanceParam = gitopsEngineInstances[idx]
			return nil
		}
	}

	return fmt.Errorf("GitOpsEngineInstances were found, but no matches for namespace %v, for %v, owner of %v",
		gitopsEngineNamespace.UID, gitopsEngineCluster.Gitopsenginecluster_id, ownerId)
}

func dbAppToApplicationCR(dbApp *database.Application) (*appv1.Application, error) {

	newApplicationEntry := &appv1.Application{
		//  Todo: should come from gitopsengineinstance!
		ObjectMeta: metav1.ObjectMeta{Name: dbApp.Name, Namespace: "argocd"},
	}
	// assigning spec
	newSpecErr := json.Unmarshal([]byte(dbApp.Spec_field), &newApplicationEntry.Spec)
	if newSpecErr != nil {
		return nil, newSpecErr
	}

	return newApplicationEntry, nil
}

func contains(listAllApps []database.Application, str string) (bool, string) {
	for _, dbApp := range listAllApps {
		if dbApp.Name == str {
			return true, dbApp.Application_id
		}
	}
	return false, ""
}
