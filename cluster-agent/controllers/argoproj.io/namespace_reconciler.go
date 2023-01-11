package argoprojio

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	cache "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/fauxargocd"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	"gopkg.in/yaml.v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	appRowBatchSize             = 50               // Number of rows needs to be fetched in each batch.
	namespaceReconcilerInterval = 30 * time.Minute // Interval in Minutes to reconcile workspace/namespace.
	sleepIntervalsOfBatches     = 1 * time.Second  // Interval in Millisecond between each batch.
)

// This function iterates through each Workspace/Namespace present in DB and ensures that the state of resources in Cluster is in Sync with DB.
func (r *ApplicationReconciler) StartNamespaceReconciler() {
	r.startTimerForNextCycle()
}

func (r *ApplicationReconciler) startTimerForNextCycle() {
	go func() {
		// Timer to trigger Reconciler
		timer := time.NewTimer(time.Duration(namespaceReconcilerInterval))
		<-timer.C

		ctx := context.Background()
		log := log.FromContext(ctx).WithValues("component", "namespace-reconciler")

		_, _ = sharedutil.CatchPanic(func() error {
			runNamespaceReconcile(ctx, r.DB, r.Client, log)
			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'namespaceReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.startTimerForNextCycle()
	}()

}

func runNamespaceReconcile(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, log logr.Logger) {

	// Fetch list of ArgoCD applications to be used later
	// map: applications IDs seen (string) -> (map value not used)
	processedApplicationIds := make(map[string]any)

	argoApplicationList := appv1.ApplicationList{}
	if err := client.List(ctx, &argoApplicationList); err != nil {
		log.Error(err, "Error occurred in Namespace Reconciler while fetching list of ArgoCD applications.")
	}
	argoApplications := argoApplicationList.Items

	offSet := 0

	// Delete operation resources created during previous run.
	cleanK8sOperations(ctx, dbQueries, client, log)

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
			time.Sleep(sleepIntervalsOfBatches)
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
			processedApplicationIds[applicationRowFromDB.Application_id] = false

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

					_, _, err = operations.CreateOperation(ctx, false, dbOperationInput,
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
			if compareApplications(applicationFromArgoCD, applicationFromDB, log) {
				log.Info("Argo application is not in Sync with DB, updating Argo CD App. Application:" + applicationRowFromDB.Application_id)
			} else {
				log.V(sharedutil.LogLevel_Debug).Info("Argo application is in Sync with DB, Application:" + applicationRowFromDB.Application_id)
				continue
			}

			// At this point application from ArgoCD and DB are not in Sync, so need to update Argo CD Application resource
			// according to DB entry

			// ArgoCD application and DB entry are not in Sync,
			// ArgoCD should use the state of resources present in the database should
			// Create Operation to inform Argo CD to get in Sync with database entry.
			dbOperationInput := db.Operation{
				Instance_id:   applicationRowFromDB.Engine_instance_inst_id,
				Resource_id:   applicationRowFromDB.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			_, _, err = operations.CreateOperation(ctx, false, dbOperationInput,
				specialClusterUser.Clusteruser_id, cache.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, client, log)
			if err != nil {
				log.Error(err, "Namespace Reconciler is unable to create operation: "+dbOperationInput.ShortString())
				continue
			}

			log.Info("Namespace Reconcile processed application: " + applicationRowFromDB.Application_id)
		}

		// Skip processed entries in next iteration
		offSet += appRowBatchSize
	}

	// Start a goroutine, because DeleteArgoCDApplication() function from cluster-agent/controllers may take some time to delete application.
	go deleteOrphanedApplications(argoApplications, processedApplicationIds, ctx, client, log)

	log.Info(fmt.Sprintf("Namespace Reconciler finished an iteration at %s. "+
		"Next iteration will be triggered after %v Minutes", time.Now().String(), namespaceReconcilerInterval))
}

// compareApplications compares Application objects, since both objects are of different types we can not use == operator for comparison.
func compareApplications(appArgo appv1.Application, appDB fauxargocd.FauxApplication, log logr.Logger) bool {

	var isAPIVersionUpdateNeeded bool
	if appArgo.APIVersion != appDB.APIVersion {
		log.Info("APIVersion field in ArgoCD and DB entry is not in Sync.")
		log.Info("APIVersion:= ArgoCD: " + appArgo.APIVersion + "; DB: " + appDB.APIVersion)
		isAPIVersionUpdateNeeded = true
	}

	var isKindUpdateNeeded bool
	if appArgo.Kind != appDB.Kind {
		log.Info("Kind field in ArgoCD and DB entry is not in Sync.")
		log.Info("Kind:= ArgoCD: " + appArgo.Kind + "; DB: " + appDB.Kind)
		isKindUpdateNeeded = true
	}

	var isNameUpdateNeeded bool
	if appArgo.Name != appDB.Name {
		log.Info("Name field in ArgoCD and DB entry is not in Sync.")
		log.Info("Name:= ArgoCD: " + appArgo.Name + "; DB: " + appDB.Name)
		isNameUpdateNeeded = true
	}

	var isNamespaceUpdateNeeded bool
	if appArgo.Namespace != appDB.Namespace {
		log.Info("Namespace field in ArgoCD and DB entry is not in Sync.")
		log.Info("Namespace:= ArgoCD: " + appArgo.Namespace + "; DB: " + appDB.Namespace)
		isNamespaceUpdateNeeded = true
	}

	var isRepoUrlUpdateNeeded bool
	if appArgo.Spec.Source.RepoURL != appDB.Spec.Source.RepoURL {
		log.Info("RepoURL field in ArgoCD and DB entry is not in Sync.")
		log.Info("RepoURL:= ArgoCD: " + appArgo.Spec.Source.RepoURL + "; DB: " + appDB.Spec.Source.RepoURL)
		isRepoUrlUpdateNeeded = true
	}

	var isPathUpdateNeeded bool
	if appArgo.Spec.Source.Path != appDB.Spec.Source.Path {
		log.Info("Path field in ArgoCD and DB entry is not in Sync.")
		log.Info("Path:= ArgoCD: " + appArgo.Spec.Source.Path + "; DB: " + appDB.Spec.Source.Path)
		isPathUpdateNeeded = true
	}

	var isTargetRevisionUpdateNeeded bool
	if appArgo.Spec.Source.TargetRevision != appDB.Spec.Source.TargetRevision {
		log.Info("TargetRevision field in ArgoCD and DB entry is not in Sync.")
		log.Info("TargetRevision:= ArgoCD: " + appArgo.Spec.Source.TargetRevision + "; DB: " + appDB.Spec.Source.TargetRevision)
		isTargetRevisionUpdateNeeded = true
	}

	var isDestinationServerUpdateNeeded bool
	if appArgo.Spec.Destination.Server != appDB.Spec.Destination.Server {
		log.Info("Destination.Server field in ArgoCD and DB entry is not in Sync.")
		log.Info("Destination.Server:= ArgoCD: " + appArgo.Spec.Destination.Server + "; DB: " + appDB.Spec.Destination.Server)
		isDestinationServerUpdateNeeded = true
	}

	var isDestinationNamespaceUpdateNeeded bool
	if appArgo.Spec.Destination.Namespace != appDB.Spec.Destination.Namespace {
		log.Info("Destination.Namespace field in ArgoCD and DB entry is not in Sync.")
		log.Info("Destination.Namespace:= ArgoCD: " + appArgo.Spec.Destination.Namespace + "; DB: " + appDB.Spec.Destination.Namespace)
		isDestinationNamespaceUpdateNeeded = true
	}

	var isDestinationNameUpdateNeeded bool
	if appArgo.Spec.Destination.Name != appDB.Spec.Destination.Name {
		log.Info("Destination.Name field in ArgoCD and DB entry is not in Sync.")
		log.Info("Destination.Name:= ArgoCD: " + appArgo.Spec.Destination.Name + "; DB: " + appDB.Spec.Destination.Name)
		isDestinationNameUpdateNeeded = true
	}

	var isProjectUpdateNeeded bool
	if appArgo.Spec.Project != appDB.Spec.Project {
		log.Info("Project field in ArgoCD and DB entry is not in Sync.")
		log.Info("Project:= ArgoCD: " + appArgo.Spec.Project + "; DB: " + appDB.Spec.Project)
		isProjectUpdateNeeded = true
	}

	var isAutomatedPruneUpdateNeeded, isAutomatedSelfHealUpdateNeeded, isAutomatedAllowEmptyUpdateNeeded,
		canPanic, isNilDbSyncPolicy, isNilArgoSyncPolicy,
		isNilDbSyncPolicyAutomated, isNilArgoSyncPolicyAutomated bool

	// if SyncPolicy is nil in both Argo app and DB entry, so consider fields are in Sync.
	if appArgo.Spec.SyncPolicy == nil && appDB.Spec.SyncPolicy == nil {
		// Do not check nested fields to avoid panic
		canPanic = true
	}

	// if SyncPolicy in nil DB, but not in Argo CD
	if !canPanic && appArgo.Spec.SyncPolicy != nil && appDB.Spec.SyncPolicy == nil {
		log.Info("SyncPolicy field in ArgoCD and DB entry is not in Sync.")
		isNilDbSyncPolicy = true
	}

	// if SyncPolicy in Argo CD app in nil, but not in DB
	if !canPanic && appArgo.Spec.SyncPolicy == nil && appDB.Spec.SyncPolicy != nil {
		log.Info("SyncPolicy field in ArgoCD and DB entry is not in Sync.")
		isNilArgoSyncPolicy = true
	}

	// If SyncPolicy is nil in both Argo CD and DB then no need to check nested fields.
	// If SyncPolicy is nil in one of the objects then consider fields are not in Sync and skip checking for nested fields to avoid panic.
	if !canPanic && !isNilDbSyncPolicy && !isNilArgoSyncPolicy {

		// Now check for nested fields.

		// if SyncPolicy.Automated is nil in both Argo CD and DB, so consider fields are in Sync.
		if appArgo.Spec.SyncPolicy.Automated == nil && appDB.Spec.SyncPolicy.Automated == nil {
			// Do not check nested fields to avoid panic
			canPanic = true
		}

		// SyncPolicy.Automated is nil in DB, but not in Argo CD
		if !canPanic && appArgo.Spec.SyncPolicy.Automated != nil && appDB.Spec.SyncPolicy.Automated == nil {
			log.Info("SyncPolicy.Automated field in ArgoCD and DB entry is not in Sync.")
			isNilDbSyncPolicyAutomated = true
		}

		// SyncPolicy.Automated is nil in Argo CD, but not in DB
		if !canPanic && appArgo.Spec.SyncPolicy.Automated == nil && appDB.Spec.SyncPolicy.Automated != nil {
			log.Info("SyncPolicy.Automated field in ArgoCD and DB entry is not in Sync.")
			isNilArgoSyncPolicyAutomated = true
		}

		// If SyncPolicy.Automated is nil in both Argo CD and DB then no need to check nested fields.
		// If SyncPolicy.Automated is nil in one of the objects then consider fields are not in Sync and skip checking for nested fields to avoid panic.
		if !canPanic && !isNilDbSyncPolicyAutomated && !isNilArgoSyncPolicyAutomated {

			// Parent fields are validated and it can not panic now.

			if appArgo.Spec.SyncPolicy.Automated.Prune != appDB.Spec.SyncPolicy.Automated.Prune {
				log.Info("Prune field in ArgoCD and DB entry is not in Sync.")
				isAutomatedPruneUpdateNeeded = true
			}

			if appArgo.Spec.SyncPolicy.Automated.SelfHeal != appDB.Spec.SyncPolicy.Automated.SelfHeal {
				log.Info("SelfHeal field in ArgoCD and DB entry is not in Sync.")
				isAutomatedSelfHealUpdateNeeded = true
			}

			if appArgo.Spec.SyncPolicy.Automated.AllowEmpty != appDB.Spec.SyncPolicy.Automated.AllowEmpty {
				log.Info("AllowEmpty field in ArgoCD and DB entry is not in Sync.")
				isAutomatedAllowEmptyUpdateNeeded = true
			}
		}
	}

	// If any of the above steps have been performed, then we need to update the application.
	isUpdateNeeded := isAPIVersionUpdateNeeded || isKindUpdateNeeded || isNameUpdateNeeded ||
		isNamespaceUpdateNeeded || isRepoUrlUpdateNeeded || isPathUpdateNeeded || isTargetRevisionUpdateNeeded ||
		isDestinationServerUpdateNeeded || isDestinationNamespaceUpdateNeeded || isDestinationNameUpdateNeeded ||
		isProjectUpdateNeeded || isAutomatedPruneUpdateNeeded || isAutomatedSelfHealUpdateNeeded || isAutomatedAllowEmptyUpdateNeeded ||
		isNilDbSyncPolicy || isNilArgoSyncPolicy || isNilDbSyncPolicyAutomated || isNilArgoSyncPolicyAutomated

	return isUpdateNeeded
}

func cleanK8sOperations(ctx context.Context, dbq db.DatabaseQueries, client client.Client, log logr.Logger) {
	// Get list of Operations from cluster.
	listOfK8sOperation := v1alpha1.OperationList{}
	err := client.List(ctx, &listOfK8sOperation)
	if err != nil {
		log.Error(err, "Unable to fetch list of k8s Operation from cluster.")
		return
	}

	for _, k8sOperation := range listOfK8sOperation.Items {

		// Skip if Operation was not created by Namespace Reconciler.
		if k8sOperation.Annotations[operations.IdentifierKey] != operations.IdentifierValue {
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
		if err := operations.CleanupOperation(ctx, dbOperation, k8sOperation, cache.GetGitOpsEngineSingleInstanceNamespace(),
			dbq, client, log); err != nil {

			log.Error(err, "Unable to Delete k8s Operation"+string(k8sOperation.UID)+" for DbOperation: "+string(k8sOperation.Spec.OperationID))
		} else {
			log.Info("Deleted k8s Operation: " + string(k8sOperation.UID) + " for DbOperation: " + string(k8sOperation.Spec.OperationID))
		}
	}
	log.V(sharedutil.LogLevel_Debug).Info("Cleaned all Operations created by Namespace Reconciler.")
}

func deleteOrphanedApplications(argoApplications []appv1.Application, processedApplicationIds map[string]any,
	ctx context.Context, client client.Client, log logr.Logger) []appv1.Application {

	if len(argoApplications) == 0 {
		return []appv1.Application{}
	}

	shuffledList := argoApplications

	// Shuffle the list of Argo Applications, so that we are not always deleting in the same order.
	// - This is beneficial when we have a long list of Applications to delete, that take longer than namespaceReconcilerInterval.
	rand.Shuffle(len(shuffledList), func(i, j int) {
		shuffledList[i], shuffledList[j] = shuffledList[j], shuffledList[i]
	})

	// Iterate through all Argo CD applications and delete applications which are not having entry in DB.
	var deletedOrphanedApplications []appv1.Application
	for _, application := range shuffledList {

		// Skip Applications not created by the GitOps Service
		if value, exists := application.Labels[controllers.ArgoCDApplicationDatabaseIDLabel]; !exists || value == "" {
			continue
		}

		if _, ok := processedApplicationIds[application.Labels["databaseID"]]; !ok {
			if err := controllers.DeleteArgoCDApplication(ctx, application, client, log); err != nil {
				log.Error(err, "unable to delete an orphaned Argo CD Application "+application.Name)
			} else {
				deletedOrphanedApplications = append(deletedOrphanedApplications, application)
				log.Info("Deleting orphaned Argo CD Application " + application.Name)
			}
		}
	}
	return deletedOrphanedApplications
}
