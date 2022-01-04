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

	apierr "k8s.io/apimachinery/pkg/api/errors"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	util "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	corev1 "k8s.io/api/core/v1"
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

const enableApplicationReconciler = false

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	contextLog := log.FromContext(ctx)

	// Disable the reconciling for now: this can be enabled for development via the const above.
	if !enableApplicationReconciler {
		contextLog.Info("Skipping Application reconciler event: ", "event", req)
		return ctrl.Result{}, nil
	}

	kubesystemNamespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&kubesystemNamespace), &kubesystemNamespace); err != nil {
		return ctrl.Result{}, err
	}

	argocdNamespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: util.GitOpsEngineSingleInstanceNamespace,
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&argocdNamespace), &argocdNamespace); err != nil {
		return ctrl.Result{}, err
	}

	databaseQueries, err := db.NewUnsafePostgresDBQueries(true, false)
	if err != nil {
		return ctrl.Result{}, err
	}

	gitopsEngineInstance, _, err := util.UncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, argocdNamespace, string(kubesystemNamespace.UID), databaseQueries, contextLog)
	if err != nil {
		return ctrl.Result{}, err
	}

	// retrieve the application CR from the databases and store the value in app object
	var allApplicationsInDB *[]db.Application

	// TODO: INSECURE - at the moment we are using an unsafe function here, which should be removed once we've firmed up how we are going to retrieve apps.
	if err := databaseQueries.UnsafeListAllApplications(ctx, allApplicationsInDB); err != nil {
		return ctrl.Result{}, err
	}

	// This will check whether the app.Name exists in the fetched lists from Application databases
	existsInDB, dbApp := contains(*allApplicationsInDB, req.Name)

	contextLog.Info("INSECURE: UnsafeListAllApplications is being used, don't forget to fix this once the corresponding story is implemented!!!!!")

	app := appv1.Application{}
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {

		// Usually we are getting a 'not found' error here, if we get any other error we should immediately return because
		// this is unexpected.
		if !apierr.IsNotFound(err) {
			contextLog.Error(err, "unexpected error on attempting to retrieve application")
			return ctrl.Result{}, err
		}

		// Inside this if block, there is no Application CR in the namespace

		if existsInDB {
			// Application entry exists in database, but doesn't exist in namespace, so create it
			if err := r.createApplicationCRFromDB(ctx, dbApp.Application_id, gitopsEngineInstance, databaseQueries, contextLog); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		//  Otherwise, doesn't exist in database, doesn't exist in the namespace, so no work to do.
		contextLog.V(util.LogLevel_Debug).Error(err, "Unable to find Application: "+app.Name)

		return ctrl.Result{}, nil
	}

	// From this point on, the Application CR necessarily exists

	// This will check whether the app.Name exists in the fetched lists from Application databases
	if existsInDB {

		// Case: if Application CR exists in database + namespace, then compare!
		var specDetails []byte
		if specDetails, err = json.Marshal(app.Spec); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to marshal spec field: %v", err)
		}

		if dbApp.Spec_field != string(specDetails) {
			if err := json.Unmarshal([]byte(dbApp.Spec_field), &app.Spec); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.Update(ctx, &app, &client.UpdateOptions{}); err != nil {
				return ctrl.Result{}, err
			}
		}

	} else {
		// Case: if Application CR doesn't exist in database, remove from the namespace as well

		// if it doesn't exist in the database, then delete the Application
		if err := r.Delete(ctx, &app, &client.DeleteOptions{}); err != nil {
			return ctrl.Result{}, fmt.Errorf("unable to delete application, %v ", err)
		}
	}

	return ctrl.Result{}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.Application{}).
		Complete(r)
}

func (r *ApplicationReconciler) createApplicationCRFromDB(ctx context.Context,
	dbAppId string,
	gitopsEngineInstance *db.GitopsEngineInstance,
	databaseQueries db.DatabaseQueries,
	log logr.Logger) error {

	dbApplication := &db.Application{
		Application_id: dbAppId,
	}
	// // this proves that the application does exists, create in namespace
	// errGetApp := databaseQueries.UncheckedGetApplicationById(ctx, dbApplication)
	// if errGetApp != nil {
	// 	return errGetApp
	// }

	application, err := convertDBAppToApplicationCR(dbApplication, gitopsEngineInstance.Namespace_name)
	if err != nil {
		return err
	}

	if err := r.Create(ctx, application, &client.CreateOptions{}); err != nil {
		return err
	}

	// Update the application state after application is pushed to Namespace
	// - If ApplicationState row exists no change, else create!

	dbApplicationState := &db.ApplicationState{Applicationstate_application_id: string(dbAppId)}

	if err := databaseQueries.UncheckedGetApplicationStateById(ctx, dbApplicationState); err != nil {

		// We expect not found error here, any other error should return
		if !apierr.IsNotFound(err) {
			log.Error(err, "unexpected error in createApplicationCR")
			return err
		}

		dbApplicationState := &db.ApplicationState{
			Applicationstate_application_id: dbApplication.Application_id,
			Health:                          string(application.Status.Health.Status),
			Sync_Status:                     string(application.Status.Sync.Status),
		}

		errCreateApplicationState := databaseQueries.UncheckedCreateApplicationState(ctx, dbApplicationState)
		if errCreateApplicationState != nil {
			return errCreateApplicationState
		}
	}

	return nil
}

// func getGitOpsEngineInstanceForThisCluster(ctx context.Context,
// 	gitopsEngineInstanceParam *db.GitopsEngineInstance,
// 	gitopsEngineNamespace corev1.Namespace,
// 	clientClient client.Client /*, ownerId string*/) error {

// 	// create cluster user (does it seems strange that an argo cd instances needs a cluster user?)
// 	// create cluster credentials
// 	// create gitops engine cluster
// 	// create gitops engine instance (this is fake data)

// 	if gitopsEngineInstanceParam == nil {
// 		return fmt.Errorf("nil pointer param")
// 	}

// 	dbQueries, err := db.NewProductionPostgresDBQueries(true)
// 	if err != nil {
// 		return err
// 	}

// 	// Get the ID that uniquely identifies the cluster we are running on
// 	uniqueClusterID, err := getUniqueClusterID(ctx, clientClient)
// 	if err != nil {
// 		return err
// 	}

// 	// Get the GitOpsEngineCluster that corresponds to this namespace
// 	kdb := db.KubernetesToDBResourceMapping{
// 		KubernetesResourceType: "Namespace", // TODO: Convert these to constants
// 		KubernetesResourceUID:  uniqueClusterID,
// 		DBRelationType:         "GitOpsEngineCluster",
// 	}
// 	// TODO: Whose job is it to create this?
// 	if err := dbQueries.GetDBResourceMappingForKubernetesResource(ctx, &kdb); err != nil {
// 		return err
// 	}

// 	gitopsEngineCluster := db.GitopsEngineCluster{
// 		Gitopsenginecluster_id: kdb.DBRelationKey,
// 	}
// 	if err := dbQueries.UncheckedGetGitopsEngineClusterById(ctx, &gitopsEngineCluster); err != nil {
// 		return err
// 	}

// 	var gitopsEngineInstances []db.GitopsEngineInstance
// 	if err := dbQueries.ListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx, gitopsEngineCluster.Gitopsenginecluster_id, ownerId, &gitopsEngineInstances); err != nil {
// 		return err
// 	}

// 	if len(gitopsEngineInstances) == 0 {
// 		return fmt.Errorf("no GitOpsEngineInstances were found for %v", gitopsEngineCluster.Gitopsenginecluster_id)
// 	}

// 	for idx, gitopsEngineInstance := range gitopsEngineInstances {
// 		if gitopsEngineInstance.Namespace_name == gitopsEngineNamespace.Name &&
// 			gitopsEngineInstance.Namespace_uid == string(gitopsEngineNamespace.UID) {
// 			// Match found: return it
// 			*gitopsEngineInstanceParam = gitopsEngineInstances[idx]
// 			return nil
// 		}
// 	}

// 	return fmt.Errorf("GitOpsEngineInstances were found, but no matches for namespace %v, for %v",
// 		gitopsEngineNamespace.UID, gitopsEngineCluster.Gitopsenginecluster_id)
// }

// getUniqueClusterID returns a unique UID that can be used to identify one individual cluster instance from another.
//
// Since I'm not aware of an exist K8s API that gives us a value like this, I am using the uid of kube-system, which is
// unlikely to change under normal circumstances.
// func getUniqueClusterID(ctx context.Context, clientClient client.Client) (string, error) {

// 	namespace := corev1.Namespace{}

// 	if err := clientClient.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "kube-system"}, &namespace); err != nil {
// 		return "", err
// 	}

// 	return string(namespace.UID), nil
// }

func convertDBAppToApplicationCR(dbApp *db.Application, gitopsEngineNamespace string) (*appv1.Application, error) {

	newApplicationEntry := &appv1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: dbApp.Name, Namespace: gitopsEngineNamespace},
	}
	// assigning spec
	newSpecErr := json.Unmarshal([]byte(dbApp.Spec_field), &newApplicationEntry.Spec)
	if newSpecErr != nil {
		return nil, newSpecErr
	}

	return newApplicationEntry, nil
}

func contains(listAllApps []db.Application, str string) (bool, *db.Application) {
	for idx := range listAllApps {
		dbApp := listAllApps[idx]
		if dbApp.Name == str {
			return true, &dbApp
		}
	}
	return false, nil
}
