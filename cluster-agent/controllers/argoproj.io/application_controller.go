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
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"time"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	cache "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	appEventLoop "github.com/redhat-appstudio/managed-gitops/backend/eventloop/application_event_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/util/fauxargocd"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	TaskRetryLoop *sharedutil.TaskRetryLoop
	Cache         *cache.ApplicationInfoCache
	DB            db.DatabaseQueries
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	log := log.FromContext(ctx).WithValues("name", req.Name, "namespace", req.Namespace)

	defer log.V(sharedutil.LogLevel_Debug).Info("Application Reconcile() complete.")

	// TODO: GITOPSRVCE-68 - PERF - this is single-threaded only

	// 1) Retrieve the Application CR using the request vals
	app := appv1.Application{}
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Application deleted '" + req.NamespacedName.String() + "'")
			return ctrl.Result{}, nil
		} else {
			log.Error(err, "Unexpected error on retrieving Application '"+req.NamespacedName.String()+"'")
			return ctrl.Result{}, err
		}
	}

	// 2) Retrieve the Application DB entry, using the 'databaseID' field of the Application.
	// - This is a field we add ourselves to every Application we create, which references the
	//   corresponding an Application table primary key.
	applicationDB := &db.Application{}
	if databaseID, exists := app.Labels["databaseID"]; !exists {
		log.V(sharedutil.LogLevel_Warn).Info("Application CR was missing 'databaseID' label")
		return ctrl.Result{}, nil
	} else {
		applicationDB.Application_id = databaseID
	}
	if _, _, err := r.Cache.GetApplicationById(ctx, applicationDB.Application_id); err != nil {
		if db.IsResultNotFoundError(err) {

			log.V(sharedutil.LogLevel_Warn).Info("Application CR '" + req.NamespacedName.String() + "' missing corresponding database entry: " + applicationDB.Application_id)

			adt := applicationDeleteTask{
				applicationCR: app,
				client:        r.Client,
				log:           log,
			}

			// Add the Application to the task loop, so that it can be deleted.
			r.TaskRetryLoop.AddTaskIfNotPresent(app.Namespace+"/"+app.Name, &adt,
				sharedutil.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true})

			return ctrl.Result{}, nil
		} else {
			log.Error(err, "Unable to retrieve Application from database: "+applicationDB.Application_id)
			return ctrl.Result{}, err
		}
	}

	log = log.WithValues("applicationID", applicationDB.Application_id)

	// 3) Does there exist an ApplicationState for this Application, already?
	applicationState := &db.ApplicationState{
		Applicationstate_application_id: applicationDB.Application_id,
	}
	if _, _, errGet := r.Cache.GetApplicationStateById(ctx, applicationState.Applicationstate_application_id); errGet != nil {
		if db.IsResultNotFoundError(errGet) {

			// 3a) ApplicationState doesn't exist: so create it

			applicationState.Health = db.TruncateVarchar(string(app.Status.Health.Status), db.ApplicationStateHealthLength)
			applicationState.Message = db.TruncateVarchar(app.Status.Health.Message, db.ApplicationStateMessageLength)
			applicationState.Sync_Status = db.TruncateVarchar(string(app.Status.Sync.Status), db.ApplicationStateSyncStatusLength)
			applicationState.Revision = db.TruncateVarchar(app.Status.Sync.Revision, db.ApplicationStateRevisionLength)
			sanitizeHealthAndStatus(applicationState)

			// Get the list of resources created by deployment and convert it into a compressed YAML string.
			var err error
			applicationState.Resources, err = CompressResourceData(app.Status.Resources)
			if err != nil {
				log.Error(err, "unable to compress resource data into byte array.")
				return ctrl.Result{}, err
			}

			if errCreate := r.Cache.CreateApplicationState(ctx, *applicationState); errCreate != nil {
				log.Error(errCreate, "unexpected error on writing new application state")
				return ctrl.Result{}, errCreate
			}
			// Successfully created ApplicationState
			return ctrl.Result{}, nil
		} else {
			log.Error(errGet, "Unable to retrieve ApplicationState from database: "+applicationDB.Application_id)
			return ctrl.Result{}, errGet
		}
	}

	// 4) ApplicationState already exists, so just update it.

	applicationState.Health = db.TruncateVarchar(string(app.Status.Health.Status), db.ApplicationStateHealthLength)
	applicationState.Message = db.TruncateVarchar(app.Status.Health.Message, db.ApplicationStateMessageLength)
	applicationState.Sync_Status = db.TruncateVarchar(string(app.Status.Sync.Status), db.ApplicationStateSyncStatusLength)
	applicationState.Revision = db.TruncateVarchar(app.Status.Sync.Revision, db.ApplicationStateRevisionLength)
	sanitizeHealthAndStatus(applicationState)

	// Get the list of resources created by deployment and convert it into a compressed YAML string.
	var err error
	applicationState.Resources, err = CompressResourceData(app.Status.Resources)
	if err != nil {
		log.Error(err, "unable to compress resource data into byte array.")
		return ctrl.Result{}, err
	}

	if err := r.Cache.UpdateApplicationState(ctx, *applicationState); err != nil {
		log.Error(err, "unexpected error on updating existing application state")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil

}

const (
	initialAppRowOffset         = 0
	appRowBatchSize             = 50  // Number of rows needs to be fetched in each batch.
	namespaceReConcilerInterval = 2   //Interval in Minutes to reconcile workspace/namespace.
	sleepIntervalsOfBatches     = 100 //Interval in Millisecond between each batch.
)

// This function iterates through each Workspace/Namespace present in DB and ensures that the state of resources in Cluster is in Sync with DB.
func (r *ApplicationReconciler) NamespaceReconcile() {
	time.Sleep(sleepIntervalsOfBatches * time.Millisecond) // Wait for Cache to start

	// Timer to trigger Reconciler
	ticker := time.NewTicker(time.Duration(namespaceReConcilerInterval) * time.Minute)

	ctx := context.Background()
	log := log.FromContext(ctx)

	// First Iteration after interval configured for workspace/namespace reconciliation.
	for ; true; <-ticker.C {
		RunNamespaceReconcile(ctx, r.DB, r.Client, log)
	}
}

func RunNamespaceReconcile(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, log logr.Logger) {
	offSet := initialAppRowOffset

	// Delete operation resources created during previous run.
	CleanK8sOperations(ctx, dbQueries, client, log)

	// Get Special user from DB because we need ClusterUser for creating Operation and we don't have one.
	// Hence created a dummy Cluster User for internal purpose.
	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser); err != nil {
		log.Error(err, "Error occurred in Namespace Reconciler while fetching clusterUser.")
		return
	}

	log.Info("Triggered Namespace Reconciler to keep Argo application in sync with DB.")

	// Continuously iterate and fetch batches until all entries of Application table are processed.
	for {
		if offSet != 0 {
			time.Sleep(sleepIntervalsOfBatches * time.Millisecond)
		}

		var listOfApplicationsFromDB []db.Application
		var applicationFromDB fauxargocd.FauxApplication

		// Fetch Application table entries in batch size as configured above.​
		if err := dbQueries.GetApplicationBatch(ctx, &listOfApplicationsFromDB, appRowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in Namespace Reconciler while fetching batch from Offset: %d to %d: ",
				offSet, offSet+appRowBatchSize))
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfApplicationsFromDB) == 0 {
			log.Info("All Application entries are processed by Namespace Reconciler.")
			break
		}

		// Iterate over batch received above.
		for _, applicationRowFromDB := range listOfApplicationsFromDB {

			// Fetch the Application object from DB
			if err := yaml.Unmarshal([]byte(applicationRowFromDB.Spec_field), &applicationFromDB); err != nil {
				log.Error(err, "Error occurred in Namespace Reconciler while unmarshalling application: "+applicationRowFromDB.Application_id)
				continue // Skip to next iteration instead of stopping the entire loop.
			}

			// Fetch the Application object from k8s
			applicationFromArgoCD := appv1.Application{}
			namespacedName := types.NamespacedName{
				Name:      applicationFromDB.Name,
				Namespace: applicationFromDB.Namespace}

			err := client.Get(ctx, namespacedName, &applicationFromArgoCD)
			if err != nil {
				if apierr.IsNotFound(err) {
					log.Info("Application " + applicationRowFromDB.Application_id + " not found in ArgoCD, probably user deleted it, " +
						"but It still exists in DB, hence recreating application in ArgoCD.")

					// We need to recreate ArgoCD Application, to do that create Operation to inform ArgoCD about it.
					dbOperationInput := db.Operation{
						Instance_id:   applicationRowFromDB.Engine_instance_inst_id,
						Resource_id:   applicationRowFromDB.Application_id,
						Resource_type: db.OperationResourceType_Application,
					}

					_, _, err = appEventLoop.CreateOperation(ctx, false, dbOperationInput,
						specialClusterUser.Clusteruser_id, cache.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, client, log)
					if err != nil {
						log.Error(err, "Namespace Reconciler is unable to create operation: "+dbOperationInput.ShortString())
					}
					log.Info("Operation is created to recreateArgoCD  Application " + applicationRowFromDB.Application_id)
					continue
				} else {
					log.Error(err, "Error occurred in Namespace Reconciler while fetching application from cluster: "+applicationRowFromDB.Application_id)
					continue
				}
			}

			// At this point we have the applications from ArgoCD and DB, now compare them to check if they are not in Sync.
			if CompareApplications(applicationFromArgoCD, applicationFromDB, log) {
				log.V(sharedutil.LogLevel_Debug).Info("Argo application is in Sync with DB, Application:" + applicationRowFromDB.Application_id)
				continue
			} else {
				log.Info("Argo application is not in Sync with DB, Updating ArgoCD App. Application:" + applicationRowFromDB.Application_id)
			}

			// At this point application from ArgoCD and DB are not in Sync, so need to update ArgoCD application according to DB entry

			// ArgoCD application and DB entry are not in Sync,
			// ArgoCD should use the state of resources present in the database should
			// Create Operation to inform ArgoCD to get in Sync with database entry.
			dbOperationInput := db.Operation{
				Instance_id:   applicationRowFromDB.Engine_instance_inst_id,
				Resource_id:   applicationRowFromDB.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			_, _, err = appEventLoop.CreateOperation(ctx, false, dbOperationInput,
				specialClusterUser.Clusteruser_id, cache.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, client, log)
			if err != nil {
				log.Error(err, "Namespace Reconciler is unable to create operation: "+dbOperationInput.ShortString())
				continue
			}

			log.Info("Namespace Reconcile processed application : " + applicationRowFromDB.Application_id)
		}

		// Skip processed entries in next iteration
		offSet += appRowBatchSize
	}

	log.Info(fmt.Sprintf("Namespace Reconciler finished an iteration at %s. "+
		"Next iteration will be triggered after %d Minutes", time.Now().String(), namespaceReConcilerInterval))
}

func sanitizeHealthAndStatus(applicationState *db.ApplicationState) {

	if applicationState.Health == "" {
		applicationState.Health = "Unknown"
	}

	if applicationState.Sync_Status == "" {
		applicationState.Sync_Status = "Unknown"
	}

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

// Convert ResourceStatus Array into String and then compress it into Byte Array​
func CompressResourceData(resources []appv1.ResourceStatus) ([]byte, error) {
	var byteArr []byte
	var buffer bytes.Buffer

	// Convert ResourceStatus object into String.
	resourceStr, err := yaml.Marshal(&resources)
	if err != nil {
		return byteArr, fmt.Errorf("Unable to Marshal resource data. %v", err)
	}

	// Compress string data
	gzipWriter, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
	if err != nil {
		return byteArr, fmt.Errorf("Unable to create Buffer writer. %v", err)
	}

	_, err = gzipWriter.Write([]byte(string(resourceStr)))

	if err != nil {
		return byteArr, fmt.Errorf("Unable to compress resource string. %v", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return byteArr, fmt.Errorf("Unable to close gzip writer connection. %v", err)
	}

	return buffer.Bytes(), nil
}

// Compare Application objects, since both objects are of different types we can not use == operator for comparison.
func CompareApplications(applicationFromArgoCD appv1.Application, applicationFromDB fauxargocd.FauxApplication, log logr.Logger) bool {

	if applicationFromArgoCD.APIVersion != applicationFromDB.APIVersion {
		log.Info("APIVersion field in ArgoCD and DB entry is not in Sync.")
		log.Info("APIVersion:= ArgoCD: " + applicationFromArgoCD.APIVersion + "; DB: " + applicationFromDB.APIVersion)
		return false
	}
	if applicationFromArgoCD.Kind != applicationFromDB.Kind {
		log.Info("Kind field in ArgoCD and DB entry is not in Sync.")
		log.Info("Kind:= ArgoCD: " + applicationFromArgoCD.Kind + "; DB: " + applicationFromDB.Kind)
		return false
	}
	if applicationFromArgoCD.Name != applicationFromDB.Name {
		log.Info("Name field in ArgoCD and DB entry is not in Sync.")
		log.Info("Name:= ArgoCD: " + applicationFromArgoCD.Name + "; DB: " + applicationFromDB.Name)
		return false
	}
	if applicationFromArgoCD.Namespace != applicationFromDB.Namespace {
		log.Info("Namespace field in ArgoCD and DB entry is not in Sync.")
		log.Info("Namespace:= ArgoCD: " + applicationFromArgoCD.Namespace + "; DB: " + applicationFromDB.Namespace)
		return false
	}
	if applicationFromArgoCD.Spec.Source.RepoURL != applicationFromDB.Spec.Source.RepoURL {
		log.Info("RepoURL field in ArgoCD and DB entry is not in Sync.")
		log.Info("RepoURL:= ArgoCD: " + applicationFromArgoCD.Spec.Source.RepoURL + "; DB: " + applicationFromDB.Spec.Source.RepoURL)
		return false
	}
	if applicationFromArgoCD.Spec.Source.Path != applicationFromDB.Spec.Source.Path {
		log.Info("Path field in ArgoCD and DB entry is not in Sync.")
		log.Info("Path:= ArgoCD: " + applicationFromArgoCD.Spec.Source.Path + "; DB: " + applicationFromDB.Spec.Source.Path)
		return false
	}
	if applicationFromArgoCD.Spec.Source.TargetRevision != applicationFromDB.Spec.Source.TargetRevision {
		log.Info("TargetRevision field in ArgoCD and DB entry is not in Sync.")
		log.Info("TargetRevision:= ArgoCD: " + applicationFromArgoCD.Spec.Source.TargetRevision + "; DB: " + applicationFromDB.Spec.Source.TargetRevision)
		return false
	}
	if applicationFromArgoCD.Spec.Destination.Server != applicationFromDB.Spec.Destination.Server {
		log.Info("Destination.Server field in ArgoCD and DB entry is not in Sync.")
		log.Info("Destination.Server:= ArgoCD: " + applicationFromArgoCD.Spec.Destination.Server + "; DB: " + applicationFromDB.Spec.Destination.Server)
		return false
	}
	if applicationFromArgoCD.Spec.Destination.Namespace != applicationFromDB.Spec.Destination.Namespace {
		log.Info("Destination.Namespace field in ArgoCD and DB entry is not in Sync.")
		log.Info("Destination.Namespace:= ArgoCD: " + applicationFromArgoCD.Spec.Destination.Namespace + "; DB: " + applicationFromDB.Spec.Destination.Namespace)
		return false
	}
	if applicationFromArgoCD.Spec.Destination.Name != applicationFromDB.Spec.Destination.Name {
		log.Info("Destination.Name field in ArgoCD and DB entry is not in Sync.")
		log.Info("Destination.Name:= ArgoCD: " + applicationFromArgoCD.Spec.Destination.Name + "; DB: " + applicationFromDB.Spec.Destination.Name)
		return false
	}
	if applicationFromArgoCD.Spec.Project != applicationFromDB.Spec.Project {
		log.Info("Project field in ArgoCD and DB entry is not in Sync.")
		log.Info("Project:= ArgoCD: " + applicationFromArgoCD.Spec.Project + "; DB: " + applicationFromDB.Spec.Project)
		return false
	}
	if applicationFromArgoCD.Spec.SyncPolicy.Automated.Prune != applicationFromDB.Spec.SyncPolicy.Automated.Prune {
		log.Info("Prune field in ArgoCD and DB entry is not in Sync.")
		return false
	}
	if applicationFromArgoCD.Spec.SyncPolicy.Automated.SelfHeal != applicationFromDB.Spec.SyncPolicy.Automated.SelfHeal {
		log.Info("SelfHeal field in ArgoCD and DB entry is not in Sync.")
		return false
	}
	if applicationFromArgoCD.Spec.SyncPolicy.Automated.AllowEmpty != applicationFromDB.Spec.SyncPolicy.Automated.AllowEmpty {
		log.Info("AllowEmpty field in ArgoCD and DB entry is not in Sync.")
		return false
	}
	return true
}

func CleanK8sOperations(ctx context.Context, dbq db.DatabaseQueries, client client.Client, log logr.Logger) {
	// Get list of Operaions from cluster.
	listOfK8sOperation := v1alpha1.OperationList{}
	err := client.List(ctx, &listOfK8sOperation)
	if err != nil {
		log.Error(err, "Unable to fetch list of k8s Operation from cluster.")
		return
	}

	for _, k8sOperation := range listOfK8sOperation.Items {

		// Skip if Operation was not created by Namespace Reconciler.
		if k8sOperation.Annotations[appEventLoop.IdentifierKey] != appEventLoop.IdentifierValue {
			continue
		}

		// Fetch corresponding DB entry
		dbOperation := db.Operation{
			Operation_id: k8sOperation.Spec.OperationID,
		}
		if err := dbq.GetOperationById(ctx, &dbOperation); err != nil {
			continue
		}

		if dbOperation.State != db.OperationState_Completed && dbOperation.State != db.OperationState_Failed {
			log.V(sharedutil.LogLevel_Debug).Info("K8s Operation is not ready for cleanup : " + string(k8sOperation.UID) + " DbOperation: " + string(k8sOperation.Spec.OperationID))
			continue
		}

		log.Info("Deleting Operation created by Namespace Reconciler." + string(k8sOperation.UID))

		// Delete the k8s operation now.
		if err := appEventLoop.CleanupOperation(ctx, dbOperation, k8sOperation, cache.GetGitOpsEngineSingleInstanceNamespace(), dbq, client, log); err != nil {
			log.Error(err, "Unable to Delete k8s Operation"+string(k8sOperation.UID)+" for DbOperation: "+string(k8sOperation.Spec.OperationID))
		} else {
			log.Info("Deleted k8s Operation: " + string(k8sOperation.UID) + " for DbOperation: " + string(k8sOperation.Spec.OperationID))
		}
	}
	log.V(sharedutil.LogLevel_Debug).Info("Cleaned all Operations created by Namespace Reconciler.")
}
