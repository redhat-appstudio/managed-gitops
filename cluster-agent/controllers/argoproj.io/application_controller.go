/*
Copyright 2021, 2022

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
	"fmt"
	"reflect"

	"strings"
	"time"

	apierr "k8s.io/apimachinery/pkg/api/errors"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	argosharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/argoproj.io/application_info_cache"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// DeletionTaskRetryLoop maintains a list of active goroutines that are queued to delete Argo CD Applications
	DeletionTaskRetryLoop *sharedutil.TaskRetryLoop

	// Cache maintains an in-memory cache of the Application/ApplicationState database resources
	Cache *application_info_cache.ApplicationInfoCache

	DB db.DatabaseQueries
}

//+kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := log.FromContext(ctx).
		WithName(logutil.LogLogger_managed_gitops).WithValues(
		logutil.Log_K8s_Request_Namespace, req.Namespace,
		logutil.Log_K8s_Request_Name, req.Name,
		logutil.Log_Component, logutil.Log_Component_ClusterAgent)

	defer log.V(logutil.LogLevel_Debug).Info("Application Reconcile() complete.")

	rClient := sharedutil.IfEnabledSimulateUnreliableClient(r.Client)

	// TODO: GITOPSRVCE-68 - PERF - this is single-threaded only

	// 1) Retrieve the Application CR using the request vals
	app := appv1.Application{}
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Application deleted")
			return ctrl.Result{}, nil
		} else {
			log.Error(err, "Unexpected error on retrieving Application")
			return ctrl.Result{}, err
		}
	}

	// 2) Retrieve the Application DB entry, using the 'databaseID' field of the Application.
	// - This is a field we add ourselves to every Application we create, which references the
	//   corresponding Application row primary key.
	applicationDB := &db.Application{}
	if databaseID, exists := app.Labels[controllers.ArgoCDApplicationDatabaseIDLabel]; !exists {
		log.V(logutil.LogLevel_Warn).Info("Application CR was missing 'databaseID' label")
		return ctrl.Result{}, nil
	} else {
		applicationDB.Application_id = databaseID
	}

	log = log.WithValues(logutil.Log_ApplicationID, applicationDB.Application_id)

	if _, _, err := r.Cache.GetApplicationById(ctx, applicationDB.Application_id); err != nil {
		if db.IsResultNotFoundError(err) {

			log.V(logutil.LogLevel_Warn).Info("Application CR missing corresponding database entry")

			adt := applicationDeleteTask{
				applicationCR: app,
				client:        rClient,
				log:           log,
			}

			// Add the Application to the task loop, so that it can be deleted.
			r.DeletionTaskRetryLoop.AddTaskIfNotPresent(app.Namespace+"/"+app.Name, &adt,
				sharedutil.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true})

			return ctrl.Result{}, nil
		} else {
			log.Error(err, "Unable to retrieve Application from database")
			return ctrl.Result{}, err
		}
	}

	log = log.WithValues(logutil.Log_ApplicationID, applicationDB.Application_id)

	var err error
	app.Status.Sync.ComparedTo, err = convertComparedTo(app)
	if err != nil {
		log.Error(err, "Failed to process comparedTo field of Application")
		return ctrl.Result{}, err
	}

	// 3) Does there exist an ApplicationState for this Application, already?
	applicationState := &db.ApplicationState{
		Applicationstate_application_id: applicationDB.Application_id,
	}

	if _, _, errGet := r.Cache.GetApplicationStateById(ctx, applicationState.Applicationstate_application_id); errGet != nil {
		if db.IsResultNotFoundError(errGet) {

			// 3a) ApplicationState doesn't exist: so create it
			appStatusBytes, err := sharedutil.CompressObject(app.Status)
			if err != nil {
				log.Error(err, "Failed to compress the Argo CD Application status", "name", app.Name, "namespace", app.Namespace)
				return ctrl.Result{}, err
			}

			applicationState.ArgoCD_Application_Status = appStatusBytes
			if errCreate := r.Cache.CreateApplicationState(ctx, *applicationState); errCreate != nil {
				log.Error(errCreate, "unexpected error on writing new application state")
				return ctrl.Result{}, errCreate
			}
			// Successfully created ApplicationState
			return ctrl.Result{}, nil
		} else {
			log.Error(errGet, "Unable to retrieve ApplicationState from database")
			return ctrl.Result{}, errGet
		}
	}

	// 4) ApplicationState already exists, so just update it.
	appStatusBytes, err := sharedutil.CompressObject(app.Status)
	if err != nil {
		log.Error(err, "Failed to compress the Argo CD Application status", "name", app.Name, "namespace", app.Namespace)
		return ctrl.Result{}, err
	}
	applicationState.ArgoCD_Application_Status = appStatusBytes
	if err := r.Cache.UpdateApplicationState(ctx, *applicationState); err != nil {

		if strings.Contains(err.Error(), db.ErrorUnexpectedNumberOfRowsAffected) {
			log.V(logutil.LogLevel_Warn).Error(err, "unexpected error on updating existing application state (but the Application might have been deleted)")
		} else {
			log.Error(err, "unexpected error on updating existing application state")
		}

		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil

}

type applicationDeleteTask struct {
	applicationCR appv1.Application
	client        client.Client
	log           logr.Logger
}

func (adt *applicationDeleteTask) PerformTask(taskContext context.Context) (bool, error) {

	err := controllers.DeleteArgoCDApplication(context.Background(), adt.applicationCR, adt.client, adt.log)

	if err != nil {
		adt.log.Error(err, "Unable to delete Argo CD Application: "+adt.applicationCR.Name+"/"+adt.applicationCR.Namespace)
	}

	// Don't retry on fail, just return the error
	return false, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.Application{}).
		Complete(r)
}

// convertComparedTo converts the comparedTo of Application status to the correct form that will
// be used by the GitOpsDeployment
func convertComparedTo(argoCDAppParam appv1.Application) (appv1.ComparedTo, error) {
	comparedToParam := argoCDAppParam.Status.Sync.ComparedTo

	if reflect.DeepEqual(comparedToParam, appv1.ComparedTo{}) {
		return appv1.ComparedTo{}, nil
	}

	// 1) Read the Argo CD Cluster Secret name (which points to an Argo CD Cluster Secret in the Argo CD namespace) from the
	// Argo CD Application, and extract the managed environment ID from it.
	managedEnvID, isLocal, err := argosharedutil.ConvertArgoCDClusterSecretNameToManagedIdDatabaseRowId(comparedToParam.Destination.Name)
	if err != nil {
		return appv1.ComparedTo{}, fmt.Errorf("unable to convert secret name to managed env: %v", err)
	}

	// 2) If the Argo CD cluster secret name is a real cluster secret that exists in the Argo CD namespace, then use
	// the managed environment database ID that we extracted, and store it in comparedTo.
	if !isLocal {
		if managedEnvID == "" {
			return appv1.ComparedTo{}, fmt.Errorf("managed environment id was empty for '%s'", argoCDAppParam.Name)
		}

		// Rather than storing the name of the Argo CD cluster secret in the .Destination.Name field, we instead store
		// the ID of the managed environment in the database.
		comparedToParam.Destination.Name = managedEnvID
	}

	return comparedToParam, nil
}
