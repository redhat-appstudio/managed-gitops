package eventloop

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/fauxargocd"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	sharedresourceloop "github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	rowBatchSize                      = 100              // Number of rows needs to be fetched in each batch.
	defaultDatabaseReconcilerInterval = 30 * time.Minute // Interval in Minutes to reconcile Database.
	sleepIntervalsOfBatches           = 1 * time.Second  // Interval in Millisecond between each batch.
	waitTimeforRowDelete              = 1 * time.Hour    // Number of hours to wait before deleting DB row
)

// A 'dangling' DB entry (for lack of a better term) is a row in the database that points to a K8s resource that no longer exists
// or a row in database which is missing required entries in other tables.
//
// This usually shouldn't occur: usually we should get informed by K8 when a CR is deleted, but this
// is not guaranteed in all cases (for example, if CRs are deleted while the GitOps service is
// down/or otherwise not running).
//
// Thus, it is useful to have some code that will periodically run to clean up tables, as as a method of
// background self-healing.
//
// This periodic, background self-healing of tables is the responsibility of this file.
//
// See 'docs/self-healing-mechanism.md' for more details.

// DatabaseReconciler reconciles Database entries
type DatabaseReconciler struct {
	client.Client
	DB               db.DatabaseQueries
	K8sClientFactory sharedresourceloop.SRLK8sClientFactory
}

// This function iterates through each entry of DTAM and ACTDM tables in DB and ensures that the required CRs is present in cluster.
func (r *DatabaseReconciler) StartDatabaseReconciler() {
	ctx := context.Background()

	log := log.FromContext(ctx).
		WithName(logutil.LogLogger_managed_gitops).
		WithValues(logutil.Log_Component, logutil.Log_Component_Backend_DatabaseReconciler)

	databaseReconcilerInterval := sharedutil.SelfHealInterval(defaultDatabaseReconcilerInterval, log)
	if databaseReconcilerInterval > 0 {
		r.startTimerForNextCycle(ctx, databaseReconcilerInterval, log)
		log.Info(fmt.Sprintf("Database reconciliation has been scheduled every %s", databaseReconcilerInterval.String()))
	} else {
		log.Info("Database reconciliation has been disabled")
	}
}

func (r *DatabaseReconciler) startTimerForNextCycle(ctx context.Context, databaseReconcilerInterval time.Duration, log logr.Logger) {
	go func() {
		// Timer to trigger Reconciler
		timer := time.NewTimer(databaseReconcilerInterval)
		<-timer.C

		_, _ = sharedutil.CatchPanic(func() error {

			// Clean orphaned entries from DTAM table and other table they relate to (i.e ApplicationState, Application).
			if err := cleanOrphanedEntriesfromTable_DTAM(ctx, r.DB, r.Client, false, log); err != nil {
				log.Error(err, "error from startTimerForNextCycle")
			}

			// Clean orphaned entries from ACTDM table and other table they relate to (i.e ManagedEnvironment, RepositoryCredential, GitOpsDeploymentSync).
			if err := cleanOrphanedEntriesfromTable_ACTDM(ctx, r.DB, r.Client, r.K8sClientFactory, false, log); err != nil {
				log.Error(err, "error from startTimerForNextCycle")
			}

			// Clean orphaned entries from RepositoryCredential, SyncOperation, ManagedEnvironment tables if they dont have related entries in DTAM table.
			if err := cleanOrphanedEntriesfromTable(ctx, r.DB, r.Client, r.K8sClientFactory, false, log); err != nil {
				log.Error(err, "error from startTimerForNextCycle")
			}

			// Clean orphaned entries from Application table if they dont have related entries in ACTDM table.
			if err := cleanOrphanedEntriesfromTable_Application(ctx, r.DB, r.Client, false, log); err != nil {
				log.Error(err, "error from startTimerForNextCycle")
			}

			// Clean orphaned entries from Operation table.
			if err := cleanOrphanedEntriesfromTable_Operation(ctx, r.DB, r.Client, false, log); err != nil {
				log.Error(err, "error from startTimerForNextCycle")
			}

			// Clean orphaned entries from ClusterUser table.
			if err := cleanOrphanedEntriesfromTable_ClusterUser(ctx, r.DB, r.Client, false, log); err != nil {
				log.Error(err, "error from startTimerForNextCycle")
			}

			// Clean orphaned entries from ClusterCredential table.
			if err := cleanOrphanedEntriesfromTable_ClusterCredential(ctx, r.DB, r.Client, false, log); err != nil {
				log.Error(err, "error from startTimerForNextCycle")
			}

			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'databaseReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.startTimerForNextCycle(ctx, databaseReconcilerInterval, log)
	}()

}

///////////////
// Clean-up logic for Deployment To Application Mapping table and utility functions.
// This will clean orphaned entries from DTAM table and other table they relate to (i.e ApplicationState, Application).
///////////////

// cleanOrphanedEntriesfromTable_DTAM loops through the DTAMs in a database and verifies they are still valid. If not, the resources are deleted.
// - The skipDelay can be used to skip the time.Sleep(), but this should true when called from a unit test.
func cleanOrphanedEntriesfromTable_DTAM(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, skipDelay bool, logParam logr.Logger) error {

	var res error

	offSet := 0

	log := logParam.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue, sharedutil.Log_JobTypeKey, "DB_DTAM")

	// Continuously iterate and fetch batches until all entries of DeploymentToApplicationMapping table are processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfdeplToAppMapping []db.DeploymentToApplicationMapping

		// Fetch DeploymentToApplicationMapping table entries in batch size as configured above.​
		if err := dbQueries.GetDeploymentToApplicationMappingBatch(ctx, &listOfdeplToAppMapping, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in DTAM Reconcile while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))

			if res == nil {
				res = fmt.Errorf("error occurred in DTAM Reconcile while fetching batch from Offset: %w", err)
			}
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfdeplToAppMapping) == 0 {
			log.Info("All DeploymentToApplicationMapping entries are processed by DTAM Reconciler.")
			break
		}

		// Iterate over batch received above.
		for i := range listOfdeplToAppMapping {
			deplToAppMappingFromDB := listOfdeplToAppMapping[i] // To avoid "Implicit memory aliasing in for loop." error.
			gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deplToAppMappingFromDB.DeploymentName,
					Namespace: deplToAppMappingFromDB.DeploymentNamespace,
				},
			}

			log := log.WithValues("gitopsDeploymentName", gitopsDeployment.Name)

			if err := client.Get(ctx, types.NamespacedName{Name: deplToAppMappingFromDB.DeploymentName, Namespace: deplToAppMappingFromDB.DeploymentNamespace}, &gitopsDeployment); err != nil {
				if apierr.IsNotFound(err) {
					// A) If GitOpsDeployment CR is not found in cluster, delete related entries from table

					log.Info("GitOpsDeployment not found in Cluster, probably user deleted it, but it still exists in DB, hence deleting related database entries.")

					if err := cleanOrphanedEntriesfromTable_DTAM_DeleteEntry(ctx, deplToAppMappingFromDB, dbQueries, log); err != nil {
						log.Error(err, "Error occurred in DTAM Reconciler while cleaning gitOpsDeployment entries from DB")

						if res == nil {
							res = fmt.Errorf("error occurred in DTAM Reconciler while cleaning gitOpsDeployment entries from DB: %w", err)
						}
					}
				} else {
					// B) Some other unexpected error occurred, so we just skip it until next time
					log.Error(err, "Error occurred in DTAM Reconciler while fetching GitOpsDeployment from cluster")
					if res == nil {
						res = fmt.Errorf("error occurred in DTAM Reconciler while fetching GitOpsDeployment from cluster: %w", err)
					}
				}

				// C) If the GitOpsDeployment does exist, but the UID doesn't match, then we can delete DTAM and Application
			} else if string(gitopsDeployment.UID) != deplToAppMappingFromDB.Deploymenttoapplicationmapping_uid_id {

				// This means that another GitOpsDeployment exists in the namespace with this name.

				if err := cleanOrphanedEntriesfromTable_DTAM_DeleteEntry(ctx, deplToAppMappingFromDB, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in DTAM Reconciler while cleaning gitOpsDeployment entries from DB")
					if res == nil {
						res = fmt.Errorf("error occurred in DTAM Reconciler while cleaning gitOpsDeployment entries from DB: %w", err)
					}
				}

			}

			log.V(logutil.LogLevel_Debug).Info("DTAM Reconcile processed deploymentToApplicationMapping entry: " + deplToAppMappingFromDB.Deploymenttoapplicationmapping_uid_id)
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}

	return res
}

// cleanOrphanedEntriesfromTable_DTAM_DeleteEntry deletes database entries related to a given GitOpsDeployment
func cleanOrphanedEntriesfromTable_DTAM_DeleteEntry(ctx context.Context, deplToAppMapping db.DeploymentToApplicationMapping,
	dbQueries db.DatabaseQueries, log logr.Logger) error {
	dbApplicationFound := true

	// If the gitopsdepl CR doesn't exist, but database row does, then the CR has been deleted, so handle it.
	dbApplication := db.Application{
		Application_id: deplToAppMapping.Application_id,
	}

	if err := dbQueries.GetApplicationById(ctx, &dbApplication); err != nil {
		log.Error(err, "unable to get application by id", "id", deplToAppMapping.Application_id)

		if db.IsResultNotFoundError(err) {
			dbApplicationFound = false
			// Log the error, but continue.
		} else {
			return err
		}
	}

	log = log.WithValues("applicationID", deplToAppMapping.Application_id)

	// 1) Remove the ApplicationState from the database
	if err := deleteDbEntry(ctx, deplToAppMapping.Application_id, dbType_ApplicationState, deplToAppMapping, dbQueries, log); err != nil {
		return err
	}

	// 2) Set the application field of SyncOperations to nil, for all SyncOperations that point to this Application
	// - this ensures that the foreign key constraint of SyncOperation doesn't prevent us from deletion the Application
	rowsUpdated, err := dbQueries.UpdateSyncOperationRemoveApplicationField(ctx, deplToAppMapping.Application_id)
	if err != nil {
		log.Error(err, "unable to update old sync operations")
		return err
	} else if rowsUpdated == 0 {
		log.Info("no SyncOperation rows updated, for updating old syncoperations on GitOpsDeployment deletion")
	} else {
		log.Info("Removed references to Application from all SyncOperations that reference it")
	}

	// 3) Delete DeplToAppMapping row that points to this Application
	if err := deleteDbEntry(ctx, deplToAppMapping.Deploymenttoapplicationmapping_uid_id, dbType_DeploymentToApplicationMapping, deplToAppMapping, dbQueries, log); err != nil {
		return err
	}

	if !dbApplicationFound {
		log.Info("While cleaning up old gitopsdepl entries, the Application row wasn't found. No more work to do.")
		// If the Application CR no longer exists, then our work is done.
		return nil
	}

	// If the Application table entry still exists, finish the cleanup...

	// 4) Remove the Application from the database
	log.Info("GitOpsDeployment was deleted, so deleting Application row from database")
	if err := deleteDbEntry(ctx, deplToAppMapping.Application_id, dbType_Application, deplToAppMapping, dbQueries, log); err != nil {
		return err
	}
	return nil
}

///////////////
// Clean-up logic for API CR To Database Mapping table and utility functions.
// This will clean orphaned entries from ACTDM table and other table they relate to (i.e ManagedEnvironment, RepositoryCredential, GitOpsDeploymentSync).
///////////////

// cleanOrphanedEntriesfromTable_ACTDM loops through the ACTDM in a database and verifies they are still valid. If not, the resources are deleted.
func cleanOrphanedEntriesfromTable_ACTDM(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, skipDelay bool, l logr.Logger) error {
	offset := 0

	log := l.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "DB_ACTDM")

	var res error

	// Continuously iterate and fetch batches until all entries of ACTDM table are processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfApiCrToDbMapping []db.APICRToDatabaseMapping

		// Fetch ACTDMs table entries in batch size as configured above.​
		if err := dbQueries.GetAPICRToDatabaseMappingBatch(ctx, &listOfApiCrToDbMapping, rowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in ACTDM Reconcile while fetching batch from Offset: %d to %d: ",
				offset, offset+rowBatchSize))

			if res == nil {
				res = fmt.Errorf("error occurred in ACTDM Reconcile while fetching batch from Offset: %w", err)
			}
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfApiCrToDbMapping) == 0 {
			log.Info("All ACTDM entries are processed by ACTDM Reconciler.")
			break
		}

		// Iterate over batch received above.
		for i := range listOfApiCrToDbMapping {
			apiCrToDbMappingFromDB := listOfApiCrToDbMapping[i] // To avoid "Implicit memory aliasing in for loop." error.

			objectMeta := metav1.ObjectMeta{
				Name:      apiCrToDbMappingFromDB.APIResourceName,
				Namespace: apiCrToDbMappingFromDB.APIResourceNamespace,
			}

			// Process entry based on type of CR it points to.
			if db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment == apiCrToDbMappingFromDB.APIResourceType {

				// Process if CR is of GitOpsDeploymentManagedEnvironment type.
				if err := cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, k8sClientFactory, log); err != nil && res == nil {
					res = err
				}
			} else if db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential == apiCrToDbMappingFromDB.APIResourceType {

				// Process if CR is of GitOpsDeploymentRepositoryCredential type.
				if err := cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, log); err != nil && res == nil {
					res = err
				}

			} else if db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun == apiCrToDbMappingFromDB.APIResourceType {

				// Process if CR is of GitOpsDeploymentSyncRun type.
				if err := cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, log); err != nil && res == nil {
					res = err
				}
			} else {
				log.Error(nil, "SEVERE: unrecognized APIResourceType", "resourceType", apiCrToDbMappingFromDB.APIResourceType)
				if res == nil {
					res = fmt.Errorf("unrecognized APIResourceType")
				}
			}

			log.V(logutil.LogLevel_Debug).Info("ACTDM Reconcile processed APICRToDatabaseMapping entry: " + apiCrToDbMappingFromDB.APIResourceUID)
		}

		// Skip processed entries in next iteration
		offset += rowBatchSize
	}

	return res
}

func cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment(ctx context.Context, client client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, log logr.Logger) error {
	// Process if CR is of GitOpsDeploymentManagedEnvironment type.
	managedEnvK8s := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{ObjectMeta: objectMeta}

	// Check if required CR is present in cluster
	if isOrphaned, err := isRowOrphaned(ctx, client, apiCrToDbMappingFromDB, &managedEnvK8s, log); err != nil {
		return err
	} else if !isOrphaned {
		return nil
	}

	log = log.WithValues("dbRelationKey", apiCrToDbMappingFromDB.DBRelationKey)

	// If CR is not present in cluster clean ACTDM entry
	if err := deleteDbEntry(ctx, apiCrToDbMappingFromDB.DBRelationKey, dbType_APICRToDatabaseMapping, apiCrToDbMappingFromDB, dbQueries, log); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while deleting APICRToDatabaseMapping entry from DB")
		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while deleting APICRToDatabaseMapping entry from DB: %w", err)
	}

	managedEnvDb := db.ManagedEnvironment{
		Managedenvironment_id: apiCrToDbMappingFromDB.DBRelationKey,
	}
	if err := dbQueries.GetManagedEnvironmentById(ctx, &managedEnvDb); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while fetching ManagedEnvironment from DB")
		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while fetching ManagedEnvironment from DB: %w", err)
	}

	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(context.Background(), &specialClusterUser); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while fetching SpecialClusterUser from DB")
		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while fetching SpecialClusterUser from DB: %w", err)
	}

	if err := sharedresourceloop.DeleteManagedEnvironmentResources(ctx, apiCrToDbMappingFromDB.DBRelationKey, &managedEnvDb, specialClusterUser, k8sClientFactory, dbQueries, log); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while cleaning ManagedEnvironment entry from DB")
		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while cleaning ManagedEnvironment entry from DB: %w", err)
	}
	log.Info("Managed-gitops clean up job for DB deleted ManagedEnvironment entry from DB.")
	return nil
}

func cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential(ctx context.Context, client client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, log logr.Logger) error {
	// Process if CR is of GitOpsDeploymentRepositoryCredential type.
	repoCredentialK8s := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{ObjectMeta: objectMeta}

	// Check if required CR is present in cluster
	if isOrphaned, err := isRowOrphaned(ctx, client, apiCrToDbMappingFromDB, &repoCredentialK8s, log); err != nil {
		return err
	} else if !isOrphaned {
		return nil
	}

	// If CR is not present in cluster clean ACTDM entry
	if err := deleteDbEntry(ctx, apiCrToDbMappingFromDB.DBRelationKey, dbType_APICRToDatabaseMapping, apiCrToDbMappingFromDB, dbQueries, log); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential while deleting APICRToDatabaseMapping entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")

		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential while deleting APICRToDatabaseMapping entry: %w", err)
	}

	var err error
	var repoCredentialDb db.RepositoryCredentials

	if repoCredentialDb, err = dbQueries.GetRepositoryCredentialsByID(ctx, apiCrToDbMappingFromDB.DBRelationKey); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential while fetching RepositoryCredentials by ID : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")

		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential while fetching RepositoryCredentials by ID: %w", err)
	}

	// Clean RepositoryCredential table entry
	if err := deleteDbEntry(ctx, apiCrToDbMappingFromDB.DBRelationKey, dbType_RespositoryCredential, repoCredentialK8s, dbQueries, log); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential while deleting RepositoryCredential entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")

		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential while deleting RepositoryCredential entry: %w", err)
	}

	// Create k8s Operation to delete related CRs using Cluster Agent
	return createOperation(ctx, repoCredentialDb.EngineClusterID, repoCredentialDb.RepositoryCredentialsID, repoCredentialK8s.Namespace, db.OperationResourceType_RepositoryCredentials, dbQueries, client, log)
}

func cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun(ctx context.Context, client client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, log logr.Logger) error {
	// Process if CR is of GitOpsDeploymentSyncRun type.
	syncRunK8s := managedgitopsv1alpha1.GitOpsDeploymentSyncRun{ObjectMeta: objectMeta}

	// Check if required CR is present in cluster
	if isOrphaned, err := isRowOrphaned(ctx, client, apiCrToDbMappingFromDB, &syncRunK8s, log); err != nil {
		return err
	} else if !isOrphaned {
		return nil
	}

	// If CR is not present in cluster clean ACTDM entry
	if err := deleteDbEntry(ctx, apiCrToDbMappingFromDB.DBRelationKey, dbType_APICRToDatabaseMapping, apiCrToDbMappingFromDB, dbQueries, log); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while deleting APICRToDatabaseMapping entry: "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while deleting APICRToDatabaseMapping entry: %w", err)
	}

	// Clean GitOpsDeploymentSyncRun table entry
	var applicationDb db.Application

	syncOperationDb := db.SyncOperation{SyncOperation_id: apiCrToDbMappingFromDB.DBRelationKey}
	if err := dbQueries.GetSyncOperationById(ctx, &syncOperationDb); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while fetching SyncOperation by Id : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while fetching SyncOperation by Id: %w", err)
	}

	if err := deleteDbEntry(ctx, apiCrToDbMappingFromDB.DBRelationKey, dbType_SyncOperation, syncRunK8s, dbQueries, log); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while deleting GitOpsDeploymentSyncRun entry: "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")

		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while deleting GitOpsDeploymentSyncRun entry: %w", err)
	}

	// There is no need to create an Operation if the Application ID is empty possibly due to its deletion.
	// We can delete the SyncRun CR and return.
	if syncOperationDb.Application_id == "" {
		log.Info("Application row field was empty for SyncOperation. This is normally because the Application has already been deleted.", "syncOperationID", syncOperationDb.SyncOperation_id)

		if err := client.Delete(ctx, &syncRunK8s); err != nil && !apierr.IsNotFound(err) {

			log.Error(err, "failed to remove GitOpsDeploymentSyncRun when the Application is already deleted")
			return fmt.Errorf("failed to remove GitOpsDeploymentSyncRun when the Application is already deleted: %w", err)
		}
		return nil
	}

	applicationDb = db.Application{Application_id: syncOperationDb.Application_id}
	if err := dbQueries.GetApplicationById(ctx, &applicationDb); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while fetching Application by Id: "+syncOperationDb.Application_id+" from DB.")
		return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while fetching Application by Id: %w", err)
	}

	log.Info("Managed-gitops clean up job for DB deleted SyncOperation: " + apiCrToDbMappingFromDB.DBRelationKey + " entry from DB.")

	// Create k8s Operation to delete related CRs using Cluster Agent
	return createOperation(ctx, applicationDb.Engine_instance_inst_id, syncOperationDb.SyncOperation_id, syncRunK8s.Namespace, db.OperationResourceType_SyncOperation, dbQueries, client, log)
}

// isRowOrphaned function checks if the given CR pointed by APICRToDBMapping is present in the cluster.
func isRowOrphaned(ctx context.Context, k8sClient client.Client, apiCrToDbMapping db.APICRToDatabaseMapping, obj client.Object, logger logr.Logger) (bool, error) {

	if err := k8sClient.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj); err != nil {
		if apierr.IsNotFound(err) {
			// A) If CR is not found in cluster, proceed to delete related entries from table
			logger.Info("Resource " + obj.GetName() + " not found in Cluster, probably user deleted it, " +
				"but it still exists in DB, hence deleting related database entries.")
			return true, nil
		} else {
			// B) Some other unexpected error occurred, so we just skip it until next time
			logger.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM while fetching resource from cluster: ")
			return false, err
		}
		// C) If the CR does exist, but the UID doesn't match, then we can still delete DB entry.
	} else if string(obj.GetUID()) != apiCrToDbMapping.APIResourceUID {
		// This means that another CR exists in the namespace with this name.
		return true, nil
	}

	// CR is present in cluster, no need to delete DB entry
	return false, nil
}

type dbTableName string

const (
	dbType_RespositoryCredential          dbTableName = "RepositoryCredential"
	dbType_ApplicationState               dbTableName = "ApplicationState"
	dbType_DeploymentToApplicationMapping dbTableName = "DeploymentToApplicationMapping"
	dbType_Application                    dbTableName = "Application"
	dbType_SyncOperation                  dbTableName = "SyncOperation"
	dbType_APICRToDatabaseMapping         dbTableName = "APICRToDatabaseMapping"
	dbType_ManagedEnvironment             dbTableName = "ManagedEnvironment"
	dbType_Operation                      dbTableName = "Operation"
	dbType_ClusterAccess                  dbTableName = "ClusterAccess"
	dbType_ClusterUser                    dbTableName = "ClusterUser"
	dbType_GitopsEngineCluster            dbTableName = "GitopsEngineCluster"
	dbType_ClusterCredentials             dbTableName = "ClusterCredentials"
	dbType_ApplicationOwner               dbTableName = "ApplicationOwner"
)

// deleteDbEntry deletes database entry of a given CR
func deleteDbEntry(ctx context.Context, id string, dbRow dbTableName, t interface{}, dbQueries db.DatabaseQueries, log logr.Logger) error {
	var rowsDeleted int
	var err error

	// Delete row according to type
	switch dbRow {

	case dbType_ClusterUser:
		rowsDeleted, err = dbQueries.DeleteClusterUserById(ctx, id)
	case dbType_ClusterCredentials:
		rowsDeleted, err = dbQueries.DeleteClusterCredentialsById(ctx, id)
	case dbType_RespositoryCredential:
		rowsDeleted, err = dbQueries.DeleteRepositoryCredentialsByID(ctx, id)
	case dbType_ApplicationState:
		rowsDeleted, err = dbQueries.DeleteApplicationStateById(ctx, id)
	case dbType_DeploymentToApplicationMapping:
		rowsDeleted, err = dbQueries.DeleteDeploymentToApplicationMappingByDeplId(ctx, id)
	case dbType_ApplicationOwner:
		rowsDeleted, err = dbQueries.DeleteApplicationOwner(ctx, id)
	case dbType_Application:
		rowsDeleted, err = dbQueries.DeleteApplicationById(ctx, id)
	case dbType_SyncOperation:
		rowsDeleted, err = dbQueries.DeleteSyncOperationById(ctx, id)
	case dbType_Operation:
		rowsDeleted, err = dbQueries.DeleteOperationById(ctx, id)
	case dbType_APICRToDatabaseMapping:
		apiCrToDbMapping, ok := t.(db.APICRToDatabaseMapping)
		if ok {
			rowsDeleted, err = dbQueries.DeleteAPICRToDatabaseMapping(ctx, &apiCrToDbMapping)
		} else {
			return fmt.Errorf("SEVERE: invalid APICRToDatabaseMapping type provided in 'deleteDbEntry'")
		}
	default:
		return fmt.Errorf("SEVERE: unrecognized db row type")
	}

	log = log.WithValues("dbRow", string(dbRow), "dbRowID", id)

	if err != nil {
		log.Error(err, "Error occurred in deleteDbEntry while cleaning from DB")
		return err
	} else if rowsDeleted == 0 {
		// Log the warning, but continue
		log.V(logutil.LogLevel_Warn).Info("Row was not found in table, wheen cleaning up after deleted row", "rowsDeleted", rowsDeleted)
	} else {
		log.Info("Clean up job deleted entry from DB.")
	}
	return nil
}

///////////////
// Clean-up logic for RepositoryCredentials, SyncOperation, ManagedEnvironment, Application, Operation, ClusterUser, ClusterCredential tables and utility functions
// This will clean orphaned entries from RepositoryCredential, SyncOperation, ManagedEnvironment, Application tables if they dont have related entries in DTAM/ACTDM tables.
// and clean Operation, ClusterUser, ClusterCredential tables if they are no longer in use.
///////////////

// cleanOrphanedEntriesfromTable loops through ACTDM in database and returns list of resource IDs for each CR type (i.e. RepositoryCredential, ManagedEnvironment, SyncOperation) .
func cleanOrphanedEntriesfromTable(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, skipDelay bool, log logr.Logger) error {

	var res error

	// Get list of RepositoryCredential/ManagedEnvironment/SyncOperation IDs having entry in ACTDM table.
	listOfRowIdsInAPICRToDBMapping, err := getListOfCRIdsFromTable(ctx, dbQueries, "multiple", skipDelay, log)
	if err != nil {
		return fmt.Errorf("unable to 'getListOfCRIdsFromTable': %w", err)
	}

	// Loop through RepositoryCredentials and delete those which are missing entry in ACTDM.
	if err := cleanOrphanedEntriesfromTable_RepositoryCredential(ctx, dbQueries, client, listOfRowIdsInAPICRToDBMapping[dbType_RespositoryCredential], skipDelay, log); err != nil && res == nil {
		res = err
	}

	// Loop through SyncOperations and delete those which are missing entry in ACTDM.
	if err := cleanOrphanedEntriesfromTable_SyncOperation(ctx, dbQueries, client, listOfRowIdsInAPICRToDBMapping[dbType_SyncOperation], skipDelay, log); err != nil && res == nil {
		res = err
	}

	// Loop through ManagedEnvironments and delete those which are missing entry in ACTDM.
	if err := cleanOrphanedEntriesfromTable_ManagedEnvironment(ctx, dbQueries, client, listOfRowIdsInAPICRToDBMapping[dbType_ManagedEnvironment], k8sClientFactory, skipDelay, log); err != nil && res == nil {
		res = err
	}

	return res
}

// cleanOrphanedEntriesfromTable_RepositoryCredential loops through RepositoryCredentials in database and verifies they are still valid (Having entry in ACTDM). If not, the resources are deleted.
func cleanOrphanedEntriesfromTable_RepositoryCredential(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, listOfAppsIdsInDTAM map[string]bool, skipDelay bool, l logr.Logger) error {

	log := l.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "DB_RepositoryCredential")

	offset := 0

	var res error

	// Continuously iterate and fetch batches until all entries of RepositoryCredentials table are processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfRepositoryCredentialsFromDB []db.RepositoryCredentials

		// Fetch RepositoryCredentials table entries in batch size as configured above.​
		if err := dbQueries.GetRepositoryCredentialsBatch(ctx, &listOfRepositoryCredentialsFromDB, rowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_RepositoryCredential while fetching batch from Offset: %d to %d: ",
				offset, offset+rowBatchSize))

			if res == nil {
				res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_RepositoryCredential while fetching batch from Offset: %w", err)
			}

			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfRepositoryCredentialsFromDB) == 0 {
			log.Info("All RepositoryCredentials entries are processed by cleanOrphanedEntriesfromTable_RepositoryCredential.")
			break
		}

		// Iterate over batch received above.
		for _, repCred := range listOfRepositoryCredentialsFromDB {

			// Check if repository credential has entry in ACTDM table, if not then delete the repository credential
			// If created time is less than waitTimeforRowDelete then ignore dont delete even if ACTDM entry is missing.
			if !listOfAppsIdsInDTAM[repCred.RepositoryCredentialsID] &&
				time.Since(repCred.Created_on) > waitTimeforRowDelete {
				// Fetch GitopsEngineInstance as we need namespace name for creating Operation CR.
				gitopsEngineInstance := db.GitopsEngineInstance{
					Gitopsengineinstance_id: repCred.EngineClusterID,
				}

				isEngineInstancePresent := true
				if err := dbQueries.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstance); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_RepositoryCredential, while fetching Engine Instance entry "+repCred.EngineClusterID+" from DB.")
					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_RepositoryCredential, while fetching Engine Instance entry: %w", err)
					}
					isEngineInstancePresent = false
				}

				if err := deleteDbEntry(ctx, repCred.RepositoryCredentialsID, dbType_RespositoryCredential, repCred, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_RepositoryCredential while deleting RepositoryCredential entry: "+repCred.RepositoryCredentialsID+" from DB.")

					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_RepositoryCredential while deleting RepositoryCredential entry: %w", err)
					}
				}

				// Skip if GitOpsEngineInstance was not found, as we need namespace for creating Operation CR.
				if isEngineInstancePresent {
					// Create k8s Operation to delete related CRs using Cluster Agent.
					opErr := createOperation(ctx, repCred.EngineClusterID, repCred.RepositoryCredentialsID, gitopsEngineInstance.Namespace_name, db.OperationResourceType_Application, dbQueries, client, log)

					if opErr != nil && res == nil {
						res = opErr
					}
				}
			}
		}

		// Skip processed entries in next iteration
		offset += rowBatchSize
	}

	return res
}

// cleanOrphanedEntriesfromTable_SyncOperation loops through SyncOperations in database and verifies they are still valid (Having entry in ACTDM). If not, the resources are deleted.
func cleanOrphanedEntriesfromTable_SyncOperation(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, listOfAppsIdsInDTAM map[string]bool, skipDelay bool, l logr.Logger) error {

	log := l.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "DB_SyncOperation")

	var res error

	offSet := 0
	// Continuously iterate and fetch batches until all entries of RepositoryCredentials table are processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfSyncOperationFromDB []db.SyncOperation

		// Fetch SyncOperation table entries in batch size as configured above.​
		if err := dbQueries.GetSyncOperationsBatch(ctx, &listOfSyncOperationFromDB, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_SyncOperation while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))

			res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_SyncOperation while fetching batch from Offset: %w", err)
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfSyncOperationFromDB) == 0 {
			log.Info("All SyncOperation entries are processed by cleanOrphanedEntriesfromTable_SyncOperation.")
			break
		}

		// Iterate over batch received above.
		for _, syncOperation := range listOfSyncOperationFromDB {

			// Check if SyncOperation has entry in ACTDM table, if not then delete the repository credential
			// If created time is less than waitTimeforRowDelete then ignore dont delete even if ACTDM entry is missing.
			if !listOfAppsIdsInDTAM[syncOperation.SyncOperation_id] &&
				time.Since(syncOperation.Created_on) > waitTimeforRowDelete {

				var appArgo fauxargocd.FauxApplication

				isApplicationPresent := true

				applicationDb := db.Application{Application_id: syncOperation.Application_id}
				if err := dbQueries.GetApplicationById(ctx, &applicationDb); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_SyncOperation while fetching Application by ID: "+syncOperation.Application_id+" from DB.")
					isApplicationPresent = false

					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_SyncOperation while fetching Application by ID: "+syncOperation.Application_id+" from DB: %w", err)
					}

				} else {
					// no error occurred

					// Fetch Application from DB and convert Spec into an Object as we need namespace for creating Operations CR.
					if err := yaml.Unmarshal([]byte(applicationDb.Spec_field), &appArgo); err != nil {
						log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_SyncOperation while unmarshalling application: "+applicationDb.Application_id)
						isApplicationPresent = false

						if res == nil {
							res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_SyncOperation while unmarshalling application: %w", err)
						}
					}

				}

				if err := deleteDbEntry(ctx, syncOperation.SyncOperation_id, dbType_SyncOperation, syncOperation, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_SyncOperation while deleting SyncOperation entry: "+syncOperation.SyncOperation_id+" from DB.")

					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_SyncOperation while deleting SyncOperation entry: %w", err)
					}
				}

				// Skip if Application was not found, as we need namespace for creating Operation CR.
				if isApplicationPresent && appArgo.Namespace != "" {
					// Create k8s Operation to delete related CRs using Cluster Agent
					opErr := createOperation(ctx, applicationDb.Engine_instance_inst_id, syncOperation.SyncOperation_id, appArgo.Namespace, db.OperationResourceType_Application, dbQueries, client, log)
					if opErr != nil && res == nil {
						res = opErr
					}

				}
			}
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}

	return res
}

// cleanOrphanedEntriesfromTable_ManagedEnvironment loops through ManagedEnvironments in database and verifies they are still valid (Having entry in ACTDM). If not, the resources are deleted.
func cleanOrphanedEntriesfromTable_ManagedEnvironment(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, listOfRowIdsInAPICRToDBMapping map[string]bool, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, skipDelay bool, l logr.Logger) error {

	var res error

	log := l.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "DB_ManagedEnvironment")

	// Retrieve the list of K8sToDBResourceMappings, so that we don't delete any ManagedEnvironmentes referenced in them
	allK8sToDBResourceMappings, err := getListOfK8sToDBResourceMapping(ctx, dbQueries, skipDelay, l)
	if err != nil {
		return fmt.Errorf("unable to retrieve list of K8sToDBResourceMappings: %w", err)
	}

	// The set of ManagedEnvironments that are referenced in the KubernetesToDBResourceMapping table
	// map: key is Managed Environment ID, value is unused
	managedEnvIDsInK8sToDBTable := map[string]bool{}

	// Iterate through all the K8sToDBResourceMappings, looking for those that match ManagedEnvironments
	for _, k8sToDBResourceMapping := range allK8sToDBResourceMappings {
		if k8sToDBResourceMapping.DBRelationType != db.K8sToDBMapping_ManagedEnvironment {
			continue
		}

		managedEnvIDsInK8sToDBTable[k8sToDBResourceMapping.DBRelationKey] = true

	}

	offset := 0
	// Continuously iterate and fetch batches until all entries of the ManagedEnvironment table are processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfManagedEnvironmentFromDB []db.ManagedEnvironment

		// Fetch ManagedEnvironment table entries in batch size as configured above.​
		if err := dbQueries.GetManagedEnvironmentBatch(ctx, &listOfManagedEnvironmentFromDB, rowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ManagedEnvironment while fetching batch from Offset: %d to %d: ",
				offset, offset+rowBatchSize))

			if res == nil {
				res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ManagedEnvironment while fetching batch from Offset: %w", err)
			}
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfManagedEnvironmentFromDB) == 0 {
			log.Info("All ManagedEnvironment entries are processed by cleanOrphanedEntriesfromTable_ManagedEnvironment.")
			break
		}

		// Iterate over batch received above.
		for i := range listOfManagedEnvironmentFromDB {
			managedEnvironment := listOfManagedEnvironmentFromDB[i] // To avoid "Implicit memory aliasing in for loop." error.

			_, isManagedEnvInK8sToDBResourceMappingTable := managedEnvIDsInK8sToDBTable[managedEnvironment.Managedenvironment_id]

			// We should only delete a ManagedEnvironment if all of the following are true:
			// - ManagedEnvironment does NOT have an entry in APICRToDBMapping table
			// - ManagedEnvironment does NOT have an entry in the K8sToDBResourceMapping table
			// - The time that has elapsed from when the managed environment was created is greater than waitTimeforRowDelete
			if !listOfRowIdsInAPICRToDBMapping[managedEnvironment.Managedenvironment_id] &&
				!isManagedEnvInK8sToDBResourceMappingTable &&
				time.Since(managedEnvironment.Created_on) > waitTimeforRowDelete {

				var specialClusterUser db.ClusterUser
				if err := dbQueries.GetOrCreateSpecialClusterUser(context.Background(), &specialClusterUser); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ManagedEnvironment while fetching SpecialClusterUser from DB.")

					return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ManagedEnvironment while fetching SpecialClusterUser from DB: %w", err)
				}

				if err := sharedresourceloop.DeleteManagedEnvironmentResources(ctx, managedEnvironment.Managedenvironment_id, &managedEnvironment, specialClusterUser, k8sClientFactory, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ManagedEnvironment while cleaning ManagedEnvironment entry "+managedEnvironment.Managedenvironment_id+" from DB.")
					return fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ManagedEnvironment while cleaning ManagedEnvironment entry: %w", err)
				}
				log.Info("Managed-gitops clean up job for DB deleted ManagedEnvironment: " + managedEnvironment.Managedenvironment_id + " entry from DB.")
			}
		}

		// Skip processed entries in next iteration
		offset += rowBatchSize
	}

	return res
}

// cleanOrphanedEntriesfromTable_Application loops through Applications in database and verifies they are still valid (Having entry in DTAM). If not, the resources are deleted.
func cleanOrphanedEntriesfromTable_Application(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, skipDelay bool, l logr.Logger) error {

	log := l.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "DB_Application")

	// Get list of Applications having entry in DTAM table
	listOfAppsIdsInDTAM, err := getListOfCRIdsFromTable(ctx, dbQueries, dbType_Application, skipDelay, log)
	if err != nil {
		return fmt.Errorf("unable to getListOfCRIdsFromTable: %w", err)
	}

	var res error

	offset := 0
	// Continuously iterate and fetch batches until all entries of Application table are processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfApplicationsFromDB []db.Application

		// Fetch Application table entries in batch size as configured above.​
		if err := dbQueries.GetApplicationBatch(ctx, &listOfApplicationsFromDB, rowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_Application while fetching batch from Offset: %d to %d: ",
				offset, offset+rowBatchSize))
			if res == nil {
				res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Application while fetching batch from Offset: %w", err)
			}
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfApplicationsFromDB) == 0 {
			log.Info("All Application entries are processed by cleanOrphanedEntriesfromTable_Application.")
			break
		}

		// Iterate over batch received above.
		for _, appDB := range listOfApplicationsFromDB {

			// Check if application has entry in DTAM table, if not then delete the application
			// If created time is less than waitTimeforRowDelete then ignore dont delete even if ACTDM entry is missing.
			if !listOfAppsIdsInDTAM[dbType_Application][appDB.Application_id] &&
				time.Since(appDB.Created_on) > waitTimeforRowDelete {

				// Fetch Application from DB and convert Spec into an Object as we need namespace for creating Operations CR.
				var appArgo fauxargocd.FauxApplication
				isApplicationPresent := true
				if err := yaml.Unmarshal([]byte(appDB.Spec_field), &appArgo); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while unmarshalling application: "+appDB.Application_id)
					isApplicationPresent = false
					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Application while unmarshalling application: %w", err)
					}
				}

				// Delete ApplicationOwner, if present
				applicationOwner := db.ApplicationOwner{
					ApplicationOwnerApplicationID: appDB.Application_id,
				}

				if err := dbQueries.GetApplicationOwnerByApplicationID(ctx, &applicationOwner); err != nil {

					if !db.IsResultNotFoundError(err) {
						log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while retrieving applicationOwner entry: "+appDB.Application_id)

						if res == nil {
							res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Application while retrieving applicationOwner entry: %w", err)
						}

					}

				} else if err := deleteDbEntry(ctx, applicationOwner.ApplicationOwnerApplicationID, dbType_ApplicationOwner, appDB, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while deleting ApplicationOwner entry : "+appDB.
						Application_id+" from DB.")

					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Application while deleting ApplicationOwner entry: %w", err)
					}
				}

				applicationState := db.ApplicationState{
					Applicationstate_application_id: appDB.Application_id,
				}

				if err := dbQueries.GetApplicationStateById(ctx, &applicationState); err != nil {

					if !db.IsResultNotFoundError(err) {

						log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while retrieving applicationState from DB: "+applicationState.Applicationstate_application_id)

						if res == nil {
							res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Application while retrieving applicationState from DB: %w", err)
						}
					}

				} else if err := deleteDbEntry(ctx, applicationState.Applicationstate_application_id, dbType_ApplicationState, applicationState, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while deleting ApplicationState entry: "+applicationState.Applicationstate_application_id+" from DB.")

					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Application while deleting ApplicationState entry: %w", err)
					}
				}

				if err := deleteDbEntry(ctx, appDB.Application_id, dbType_Application, appDB, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while deleting Application entry: "+appDB.Application_id+" from DB.")

					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Application while deleting Application entry: %w", err)
					}
				}

				// Skip if Application was not found, as we need namespace for creating Operation CR.
				if isApplicationPresent {
					// Create k8s Operation to delete related CRs using Cluster Agent
					opErr := createOperation(ctx, appDB.Engine_instance_inst_id, appDB.Application_id, appArgo.Namespace, db.OperationResourceType_Application, dbQueries, client, log)

					if opErr != nil && res == nil {
						res = opErr
					}

				}
			}
		}

		// Skip processed entries in next iteration
		offset += rowBatchSize
	}

	return res
}

// cleanOrphanedEntriesfromTable_Operation loops through Operations in database and verifies they are still valid (Having CR in Cluster). If not, the resources are deleted/Creates.
func cleanOrphanedEntriesfromTable_Operation(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, skipDelay bool, l logr.Logger) error {

	log := l.WithValues(sharedutil.Log_JobKey, sharedutil.Log_JobKeyValue).
		WithValues(sharedutil.Log_JobTypeKey, "DB_Operation")

	var res error

	offset := 0
	// Continuously iterate and fetch batches until all entries of Operation table are processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfOperationFromDB []db.Operation

		// Fetch Operation table entries in batch size as configured above.​
		if err := dbQueries.GetOperationBatch(ctx, &listOfOperationFromDB, rowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_Operation while fetching batch from Offset: %d to %d: ",
				offset, offset+rowBatchSize))

			if res == nil {
				res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Operation while fetching batch from Offset: %w", err)
			}

			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfOperationFromDB) == 0 {
			log.Info("All Application entries are processed by cleanOrphanedEntriesfromTable_Operation.")
			break
		}

		// Iterate over batch received above.
		for _, opDB := range listOfOperationFromDB {

			// If operation has state "Completed" and created time is more than 24 Hours, then delete entry.
			if opDB.State == db.OperationState_Completed &&
				time.Since(opDB.Created_on) > (24*time.Hour) {

				if err := deleteDbEntry(ctx, opDB.Operation_id, dbType_Operation, opDB, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Operation while deleting Operation entry: "+opDB.Operation_id+" from DB.")

					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Operation while deleting Operation entry: %w", err)
					}
				}

				// No need to check next condition since Operation is deleted.
				continue
			}

			// If operation has state "Waiting" or "In Progress" and created time is more than 1 hour, but CR is missing in cluster, then recreate it.
			if (opDB.State == db.OperationState_Waiting || opDB.State == db.OperationState_In_Progress) &&
				time.Since(opDB.Created_on) > time.Hour*1 {

				gitopsEngineInstance := db.GitopsEngineInstance{
					Gitopsengineinstance_id: opDB.Instance_id,
				}

				if err := dbQueries.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstance); err != nil {
					log.Error(err, fmt.Sprintf("error occurred in cleanOrphanedEntriesfromTable_Operation while fetching gitopsEngineInstance: "+gitopsEngineInstance.Gitopsengineinstance_id))

					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Operation while fetching gitopsEngineInstance: %w", err)
					}

					continue
				}

				// Check if Operation CR exists in Cluster, if not then create it.
				operationCR := &managedgitopsv1alpha1.Operation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      operations.GenerateOperationCRName(opDB),
						Namespace: gitopsEngineInstance.Namespace_name,
					},
					Spec: managedgitopsv1alpha1.OperationSpec{
						OperationID: opDB.Operation_id,
					},
				}

				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(operationCR), operationCR); err != nil {
					if apierr.IsNotFound(err) {
						// A) If CR is not found in cluster, proceed to create CR in cluster
						log.Info("Operation resource " + operationCR.Name + " is not found in Cluster, but in DB entry it is still in " + string(opDB.State) + " state, hence recreating resource in cluster.")

						// Set annotation as an identifier for Operations created by clean up job.
						operationCR.Annotations = map[string]string{operations.IdentifierKey: operations.IdentifierValue}

						if err := k8sClient.Create(ctx, operationCR, &client.CreateOptions{}); err != nil {
							log.Error(err, "Unable to create K8s Operation")

							if res == nil {
								res = fmt.Errorf("unable to create K8s Operation: %w", err)
							}

							continue
						}

						log.Info("Managed-gitops clean up job for DB created K8s Operation CR. Operation CR Name: " + operationCR.Name +
							" Operation CR Namespace" + operationCR.Namespace + ", Operation CR ID" + operationCR.Spec.OperationID)
					} else {
						log.Error(err, "error occurred in cleanOrphanedEntriesfromTable_Operation while fetching Operation: "+operationCR.Name)

						if res == nil {
							res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Operation while fetching Operation: %w", err)
						}
					}
				}
			}
		}

		// Skip processed entries in next iteration
		offset += rowBatchSize
	}

	return res
}

// cleanOrphanedEntriesfromTable_ClusterUser loops through ClusterUsers in database and verifies they are still valid in use. If not, these users are deleted.
func cleanOrphanedEntriesfromTable_ClusterUser(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, skipDelay bool, l logr.Logger) error {
	log := l.WithValues(sharedutil.Log_JobKey, "cleanOrphanedEntriesfromTable_ClusterUser")

	// Retrieve a list of user ids from the other tables that reference ClusterUser

	var res error

	listOfUserIDsFromClusterAccess, err := getListOfUserIDsfromClusterAccessTable(ctx, dbQueries, skipDelay, log)
	if err != nil {
		return fmt.Errorf("unable to getListOfUserIDsfromClusterAccessTable: %w", err)
	}

	listOfUserIDsFromRepoCred, err := getListOfUserIDsFromRespositoryCredentialsTable(ctx, dbQueries, skipDelay, log)
	if err != nil {
		return fmt.Errorf("unable to get getListOfUserIDsFromRespositoryCredentialsTable: %w", err)
	}

	listOfUserIDsFromOperation, err := getListOfUserIDsfromOperationTable(ctx, dbQueries, skipDelay, log)
	if err != nil {
		return fmt.Errorf("unable to get getListOfUserIDsfromOperationTable: %w", err)
	}

	offset := 0
	// Continuously iterate and fetch batches until all entries of ClusterUser table are processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfClusterUserFromDB []db.ClusterUser

		// Fetch ClusterUser table entries in batch size as configured above.​
		if err := dbQueries.GetClusterUserBatch(ctx, &listOfClusterUserFromDB, rowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %d to %d: ",
				offset, offset+rowBatchSize))

			if res == nil {
				res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %w", err)
			}

			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfClusterUserFromDB) == 0 {
			log.Info("All ClusterUser entries are processed by cleanOrphanedEntriesfromTable_ClusterUser.")
			break
		}

		// Iterate over batch received above.
		for _, userDB := range listOfClusterUserFromDB {
			// Check if ClusterUser don't have entry in ClusterAccess or RespositoryCredential or Operation tables,
			// and if user is not 'Special User'
			// and if created time is more than waitTimeforRowDelete
			// then delete the user
			if !(slices.Contains(listOfUserIDsFromClusterAccess[dbType_ClusterAccess], userDB.Clusteruser_id) ||
				slices.Contains(listOfUserIDsFromRepoCred[dbType_RespositoryCredential], userDB.Clusteruser_id) ||
				slices.Contains(listOfUserIDsFromOperation[dbType_Operation], userDB.Clusteruser_id)) &&
				userDB.Clusteruser_id != db.SpecialClusterUserName &&
				time.Since(userDB.Created_on) > waitTimeforRowDelete {

				// 1) Remove the user from database
				if err := deleteDbEntry(ctx, userDB.Clusteruser_id, dbType_ClusterUser, nil, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while deleting ClusterUser entry : "+userDB.Clusteruser_id+" from DB.")

					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterUser while deleting ClusterUser entry: %w", err)
					}
				}
			}
		}

		// Skip processed entries in next iteration
		offset += rowBatchSize
	}

	return res
}

// cleanOrphanedEntriesfromTable_ClusterCredential loops through ClusterCredentials in database and verifies they are still in use. If not, the credentials are deleted.
func cleanOrphanedEntriesfromTable_ClusterCredential(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, skipDelay bool, l logr.Logger) error {

	var res error

	log := l.WithValues(sharedutil.Log_JobKey, "cleanOrphanedEntriesfromTable_ClusterCredential")

	// Retrieve a list of cluster credentials from the other tables that reference ClusterCredential table

	listOfClusterCredsFromManagedEnv, err := getListOfClusterCredentialIDsfromManagedEnvironmentTable(ctx, dbQueries, skipDelay, log)
	if err != nil {
		return fmt.Errorf("unable to getListOfClusterCredentialIDsfromManagedEnvironmentTable: %w", err)
	}

	listOfClusterCredsFromGitOpsEngine, err := getListOfClusterCredentialIDsFromGitopsEngineTable(ctx, dbQueries, skipDelay, log)
	if err != nil {
		return fmt.Errorf("unable to getListOfClusterCredentialIDsFromGitopsEngineTable: %w", err)
	}

	offset := 0
	// Continuously iterate and fetch batches until all entries of ClusterCredentials table are processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfClusterCredentialsFromDB []db.ClusterCredentials

		// Fetch ClusterCredentials table entries in batch size as configured above.​
		if err := dbQueries.GetClusterCredentialsBatch(ctx, &listOfClusterCredentialsFromDB, rowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterCredential while fetching batch from Offset: %d to %d: ",
				offset, offset+rowBatchSize))

			if res == nil {
				res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterCredential while fetching batch from Offset: %w", err)
			}

			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfClusterCredentialsFromDB) == 0 {
			log.Info("All ClusterCredentials entries are processed by cleanOrphanedEntriesfromTable_ClusterCredential.")
			break
		}

		// Iterate over batch received above.
		for _, clusterCred := range listOfClusterCredentialsFromDB {
			// Check if ClusterCredentials don't have entry in ManagedEnvironment or GitopsEngineCluster tables,
			// and if created time is more than waitTimeforRowDelete
			// if not, then delete the entry
			if !(slices.Contains(listOfClusterCredsFromManagedEnv[dbType_ManagedEnvironment], clusterCred.Clustercredentials_cred_id) ||
				slices.Contains(listOfClusterCredsFromGitOpsEngine[dbType_GitopsEngineCluster], clusterCred.Clustercredentials_cred_id)) &&
				time.Since(clusterCred.Created_on) > waitTimeforRowDelete {

				// 1) Remove the ClusterCredentials from the database
				if err := deleteDbEntry(ctx, clusterCred.Clustercredentials_cred_id, dbType_ClusterCredentials, nil, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ClusterCredential while deleting ClusterCredentials entry: "+clusterCred.Clustercredentials_cred_id+" from DB.")

					if res == nil {
						res = fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterCredential while deleting ClusterCredentials entry: %w", err)
					}
				}
			}
		}

		// Skip processed entries in next iteration
		offset += rowBatchSize
	}

	return res
}

func getListOfK8sToDBResourceMapping(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) ([]db.KubernetesToDBResourceMapping, error) {

	offset := 0

	var res []db.KubernetesToDBResourceMapping

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offset != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.KubernetesToDBResourceMapping

		// Fetch K8sToDBResourceMapping table entries in batch size as configured above.​
		if err := dbQueries.GetKubernetesToDBResourceMappingBatch(ctx, &tempList, rowBatchSize, offset); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in getListOfK8sToDBResourceMapping while fetching batch from Offset: %d to %d: ",
				offset, offset+rowBatchSize))
			return []db.KubernetesToDBResourceMapping{}, fmt.Errorf("error occurred in getListOfK8sToDBResourceMapping while fetching batch from Offset: %w", err)
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		res = append(res, tempList...)

		// Skip processed entries in next iteration
		offset += rowBatchSize
	}

	return res, nil
}

// getListOfCRIdsFromTable loops through DTAMs or APICRToDBMappigs in database and returns list of resource IDs for each CR type (i.e. RepositoryCredential, ManagedEnvironment, SyncOperation).
func getListOfCRIdsFromTable(ctx context.Context, dbQueries db.DatabaseQueries, tableType dbTableName, skipDelay bool, log logr.Logger) (map[dbTableName]map[string]bool, error) {

	offSet := 0

	// Create Map of Maps to store resource IDs according to type, Ex: {"RepositoryCredential" : {"id1":true, "id2":true}, "ManagedEnvironment" : {}, "SyncOperation" : {}}
	crIdMap := map[dbTableName]map[string]bool{}
	crIdMap[dbType_Application],
		crIdMap[dbType_RespositoryCredential],
		crIdMap[dbType_ManagedEnvironment],
		crIdMap[dbType_SyncOperation] = map[string]bool{}, map[string]bool{},
		map[string]bool{}, map[string]bool{}

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		// If resource type is Application then get list of Application IDs from DTAM table.
		if tableType == dbType_Application {
			var tempList []db.DeploymentToApplicationMapping

			// Fetch DeploymentToApplicationMapping table entries in batch size as configured above.​
			if err := dbQueries.GetDeploymentToApplicationMappingBatch(ctx, &tempList, rowBatchSize, offSet); err != nil {
				log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_Application while fetching batch from Offset: %d to %d: ",
					offSet, offSet+rowBatchSize))
				return map[dbTableName]map[string]bool{}, fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_Application while fetching batch from Offset: %w", err)
			}

			// Break the loop if no entries are left in table to be processed.
			if len(tempList) == 0 {
				break
			}

			for _, deplToAppMapping := range tempList {
				crIdMap[dbType_Application][deplToAppMapping.Application_id] = true
			}
		} else { // If resource type is RepositoryCredential/ManagedEnvironment/SyncOperation then get list of IDs from ACTDM table.

			var tempList []db.APICRToDatabaseMapping

			// Fetch ACTDM table entries in batch size as configured above.​
			if err := dbQueries.GetAPICRToDatabaseMappingBatch(ctx, &tempList, rowBatchSize, offSet); err != nil {
				log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable while fetching batch from Offset: %d to %d: ",
					offSet, offSet+rowBatchSize))

				return map[dbTableName]map[string]bool{}, fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable while fetching batch from Offset: %w", err)
			}

			// Break the loop if no entries are left in table to be processed.
			if len(tempList) == 0 {
				break
			}

			// Save list of IDs in map according to resource type.
			for _, deplToAppMapping := range tempList {
				if deplToAppMapping.DBRelationType == db.APICRToDatabaseMapping_DBRelationType_RepositoryCredential {
					crIdMap[dbType_RespositoryCredential][deplToAppMapping.DBRelationKey] = true
				} else if deplToAppMapping.DBRelationType == db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment {
					crIdMap[dbType_ManagedEnvironment][deplToAppMapping.DBRelationKey] = true
				} else if deplToAppMapping.DBRelationType == db.APICRToDatabaseMapping_DBRelationType_SyncOperation {
					crIdMap[dbType_SyncOperation][deplToAppMapping.DBRelationKey] = true
				} else {
					log.Error(nil, "SEVERE: unknown database table type", "type", deplToAppMapping.DBRelationType)
				}
			}
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}

	return crIdMap, nil
}

// getListOfUserIDsfromClusterAccessTable loops through ClusterAccess in database and returns list of user IDs.
func getListOfUserIDsfromClusterAccessTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) (map[dbTableName][]string, error) {

	offSet := 0

	// Create Map to store resource IDs according to type, Ex: {"ClusterAccess" : []}
	crIdMap := make(map[dbTableName][]string)

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.ClusterAccess

		// Fetch ClusterAccess table entries in batch size as configured above.​
		if err := dbQueries.GetClusterAccessBatch(ctx, &tempList, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
			return make(map[dbTableName][]string), fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %w", err)
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		for _, clusterAccess := range tempList {
			crIdMap[dbType_ClusterAccess] = append(crIdMap[dbType_ClusterAccess], clusterAccess.Clusteraccess_user_id)
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}

	return crIdMap, nil
}

// getListOfUserIDsFromRespositoryCredentialsTable loops through RepositoryCredentials in database and returns list of resource IDs.
func getListOfUserIDsFromRespositoryCredentialsTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) (map[dbTableName][]string, error) {

	offSet := 0

	// Create Map to store resource IDs according to type, Ex: {"RepositoryCredential" : []}
	crIdMap := make(map[dbTableName][]string)

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.RepositoryCredentials

		// Fetch RepositoryCredentials table entries in batch size as configured above.​
		if err := dbQueries.GetRepositoryCredentialsBatch(ctx, &tempList, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
			return make(map[dbTableName][]string), fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %w", err)
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		for _, repositoryCredentials := range tempList {
			crIdMap[dbType_RespositoryCredential] = append(crIdMap[dbType_RespositoryCredential], repositoryCredentials.UserID)
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
	return crIdMap, nil
}

// getListOfClusterCredentialIDsfromManagedEnvironmentTable loops through ManagedEnvironments in database and returns list of resource IDs.
func getListOfClusterCredentialIDsfromManagedEnvironmentTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) (map[dbTableName][]string, error) {

	offSet := 0

	// Create Map to store resource IDs according to type, Ex: {"ManagedEnvironment" : []}
	crIdMap := make(map[dbTableName][]string)

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.ManagedEnvironment

		// Fetch ManagedEnvironment table entries in batch size as configured above.​
		if err := dbQueries.GetManagedEnvironmentBatch(ctx, &tempList, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))

			return make(map[dbTableName][]string), fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %w", err)
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		for _, managedEnvironment := range tempList {
			crIdMap[dbType_ManagedEnvironment] = append(crIdMap[dbType_ManagedEnvironment], managedEnvironment.Clustercredentials_id)
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
	return crIdMap, nil
}

// getListOfClusterCredentialIDsFromGitopsEngineTable loops through GitopsEngineCluster and returns list of resource IDs.
func getListOfClusterCredentialIDsFromGitopsEngineTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) (map[dbTableName][]string, error) {

	offSet := 0

	// Create Map to store resource IDs according to type, Ex: {"GitopsEngineCluster" : []}
	crIdMap := make(map[dbTableName][]string)

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.GitopsEngineCluster

		// Fetch GitopsEngineCluster table entries in batch size as configured above.​
		if err := dbQueries.GetGitopsEngineClusterBatch(ctx, &tempList, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))

			return make(map[dbTableName][]string), fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %w", err)
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		for _, gitopsEngineCluster := range tempList {
			crIdMap[dbType_GitopsEngineCluster] = append(crIdMap[dbType_GitopsEngineCluster], gitopsEngineCluster.Clustercredentials_id)
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
	return crIdMap, nil
}

// getListOfUserIDsfromOperationTable loops through Operation in database and returns list of resource IDs.
func getListOfUserIDsfromOperationTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) (map[dbTableName][]string, error) {

	offSet := 0

	// Create Map to store resource IDs according to type, Ex: {"Operation" : []}
	crIdMap := make(map[dbTableName][]string)

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.Operation

		// Fetch Operation table entries in batch size as configured above.​
		if err := dbQueries.GetOperationBatch(ctx, &tempList, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
			return make(map[dbTableName][]string), fmt.Errorf("error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %w", err)
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		for _, Operation := range tempList {
			crIdMap[dbType_Operation] = append(crIdMap[dbType_Operation], Operation.Operation_owner_user_id)
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
	return crIdMap, nil
}

///////////////
// Utility functions
///////////////

// createOperation creates a k8s operation to inform Cluster Agent about deletion of related k8s CRs.
func createOperation(ctx context.Context, gitopsengineinstanceId, resourceId, namespace string, resourceType db.OperationResourceType, dbQueries db.DatabaseQueries, k8sClient client.Client, log logr.Logger) error {

	if namespace == "" {
		return fmt.Errorf("namespace parameter is empty in createOperation")
	}

	operationDb := db.Operation{
		Instance_id:   gitopsengineinstanceId,
		Resource_id:   resourceId,
		Resource_type: resourceType,
	}

	// Get Special user created for internal use,
	// because we need ClusterUser for creating Operation and we don't have one.
	// Hence created or get a dummy Cluster User for internal purpose.
	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(context.Background(), &specialClusterUser); err != nil {
		return fmt.Errorf("unable to fetch special cluster user when creating operation: %w", err)
	}

	// Create k8s Operation to inform Cluster Agent to delete related k8s CRs
	if _, _, err := operations.CreateOperation(ctx, false, operationDb,
		specialClusterUser.Clusteruser_id, namespace, dbQueries, k8sClient, log); err != nil {
		return fmt.Errorf("unable to create operation: %w", err)
	}

	return nil
}
