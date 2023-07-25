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
		WithValues("component", "database-reconciler")
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
			cleanOrphanedEntriesfromTable_DTAM(ctx, r.DB, r.Client, false, log)

			// Clean orphaned entries from ACTDM table and other table they relate to (i.e ManagedEnvironment, RepositoryCredential, GitOpsDeploymentSync).
			cleanOrphanedEntriesfromTable_ACTDM(ctx, r.DB, r.Client, r.K8sClientFactory, false, log)

			// Clean orphaned entries from RepositoryCredential, SyncOperation, ManagedEnvironment tables if they dont have related entries in DTAM table.
			cleanOrphanedEntriesfromTable(ctx, r.DB, r.Client, r.K8sClientFactory, false, log)

			// Clean orphaned entries from Application table if they dont have related entries in ACTDM table.
			cleanOrphanedEntriesfromTable_Application(ctx, r.DB, r.Client, false, log)

			// Clean orphaned entries from Operation table.
			cleanOrphanedEntriesfromTable_Operation(ctx, r.DB, r.Client, false, log)

			// Clean orphaned entries from ClusterUser table.
			cleanOrphanedEntriesfromTable_ClusterUser(ctx, r.DB, r.Client, false, log)

			// Clean orphaned entries from ClusterCredential table.
			cleanOrphanedEntriesfromTable_ClusterCredential(ctx, r.DB, r.Client, false, log)

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
func cleanOrphanedEntriesfromTable_DTAM(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, skipDelay bool, l logr.Logger) {
	offSet := 0

	log := l.WithValues(sharedutil.JobKey, sharedutil.JobKeyValue).
		WithValues(sharedutil.JobTypeKey, "DB_DTAM")

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
			gitOpsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deplToAppMappingFromDB.DeploymentName,
					Namespace: deplToAppMappingFromDB.DeploymentNamespace,
				},
			}

			if err := client.Get(ctx, types.NamespacedName{Name: deplToAppMappingFromDB.DeploymentName, Namespace: deplToAppMappingFromDB.DeploymentNamespace}, &gitOpsDeployment); err != nil {
				if apierr.IsNotFound(err) {
					// A) If GitOpsDeployment CR is not found in cluster, delete related entries from table

					log.Info("GitOpsDeployment " + gitOpsDeployment.Name + " not found in Cluster, probably user deleted it, " +
						"but It still exists in DB, hence deleting related database entries.")

					if err := cleanOrphanedEntriesfromTable_DTAM_DeleteEntry(ctx, &deplToAppMappingFromDB, dbQueries, log); err != nil {
						log.Error(err, "Error occurred in DTAM Reconciler while cleaning gitOpsDeployment entries from DB: "+gitOpsDeployment.Name)
					}
				} else {
					// B) Some other unexpected error occurred, so we just skip it until next time
					log.Error(err, "Error occurred in DTAM Reconciler while fetching GitOpsDeployment from cluster: "+gitOpsDeployment.Name)
				}

				// C) If the GitOpsDeployment does exist, but the UID doesn't match, then we can delete DTAM and Application
			} else if string(gitOpsDeployment.UID) != deplToAppMappingFromDB.Deploymenttoapplicationmapping_uid_id {

				// This means that another GitOpsDeployment exists in the namespace with this name.

				if err := cleanOrphanedEntriesfromTable_DTAM_DeleteEntry(ctx, &deplToAppMappingFromDB, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in DTAM Reconciler while cleaning gitOpsDeployment entries from DB: "+gitOpsDeployment.Name)
				}

			}

			log.Info("DTAM Reconcile processed deploymentToApplicationMapping entry: " + deplToAppMappingFromDB.Deploymenttoapplicationmapping_uid_id)
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
}

// cleanOrphanedEntriesfromTable_DTAM_DeleteEntry deletes database entries related to a given GitOpsDeployment
func cleanOrphanedEntriesfromTable_DTAM_DeleteEntry(ctx context.Context, deplToAppMapping *db.DeploymentToApplicationMapping,
	dbQueries db.DatabaseQueries, logger logr.Logger) error {
	dbApplicationFound := true

	// If the gitopsdepl CR doesn't exist, but database row does, then the CR has been deleted, so handle it.
	dbApplication := db.Application{
		Application_id: deplToAppMapping.Application_id,
	}

	if err := dbQueries.GetApplicationById(ctx, &dbApplication); err != nil {
		logger.Error(err, "unable to get application by id", "id", deplToAppMapping.Application_id)

		if db.IsResultNotFoundError(err) {
			dbApplicationFound = false
			// Log the error, but continue.
		} else {
			return err
		}
	}

	log := logger.WithValues("applicationID", deplToAppMapping.Application_id)

	// 1) Remove the ApplicationState from the database
	if err := deleteDbEntry(ctx, dbQueries, deplToAppMapping.Application_id, dbType_ApplicationState, log, deplToAppMapping); err != nil {
		return err
	}

	// 2) Set the application field of SyncOperations to nil, for all SyncOperations that point to this Application
	// - this ensures that the foreign key constraint of SyncOperation doesn't prevent us from deletion the Application
	rowsUpdated, err := dbQueries.UpdateSyncOperationRemoveApplicationField(ctx, deplToAppMapping.Application_id)
	if err != nil {
		log.Error(err, "unable to update old sync operations", "applicationId", deplToAppMapping.Application_id)
		return err
	} else if rowsUpdated == 0 {
		log.Info("no SyncOperation rows updated, for updating old syncoperations on GitOpsDeployment deletion")
	} else {
		log.Info("Removed references to Application from all SyncOperations that reference it")
	}

	// 3) Delete DeplToAppMapping row that points to this Application
	if err := deleteDbEntry(ctx, dbQueries, deplToAppMapping.Deploymenttoapplicationmapping_uid_id, dbType_DeploymentToApplicationMapping, log, deplToAppMapping); err != nil {
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
	if err := deleteDbEntry(ctx, dbQueries, deplToAppMapping.Application_id, dbType_Application, log, deplToAppMapping); err != nil {
		return err
	}
	return nil
}

///////////////
// Clean-up logic for API CR To Database Mapping table and utility functions.
// This will clean orphaned entries from ACTDM table and other table they relate to (i.e ManagedEnvironment, RepositoryCredential, GitOpsDeploymentSync).
///////////////

// cleanOrphanedEntriesfromTable_ACTDM loops through the ACTDM in a database and verifies they are still valid. If not, the resources are deleted.
func cleanOrphanedEntriesfromTable_ACTDM(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, skipDelay bool, l logr.Logger) {
	offSet := 0

	log := l.WithValues(sharedutil.JobKey, sharedutil.JobKeyValue).
		WithValues(sharedutil.JobTypeKey, "DB_ACTDM")

	// Continuously iterate and fetch batches until all entries of ACTDM table are processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfApiCrToDbMapping []db.APICRToDatabaseMapping

		// Fetch ACTDMs table entries in batch size as configured above.​
		if err := dbQueries.GetAPICRToDatabaseMappingBatch(ctx, &listOfApiCrToDbMapping, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in ACTDM Reconcile while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
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
				cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, k8sClientFactory, log)
			} else if db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential == apiCrToDbMappingFromDB.APIResourceType {

				// Process if CR is of GitOpsDeploymentRepositoryCredential type.
				cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, log)
			} else if db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun == apiCrToDbMappingFromDB.APIResourceType {

				// Process if CR is of GitOpsDeploymentSyncRun type.
				cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, log)
			} else {
				log.Error(nil, "SEVERE: unrecognized APIResourceType", "resourceType", apiCrToDbMappingFromDB.APIResourceType)
			}

			log.Info("ACTDM Reconcile processed APICRToDatabaseMapping entry: " + apiCrToDbMappingFromDB.APIResourceUID)
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
}

func cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment(ctx context.Context, client client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, log logr.Logger) {
	// Process if CR is of GitOpsDeploymentManagedEnvironment type.
	managedEnvK8s := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{ObjectMeta: objectMeta}

	// Check if required CR is present in cluster
	if isOrphaned := isRowOrphaned(ctx, client, &apiCrToDbMappingFromDB, &managedEnvK8s, log); !isOrphaned {
		return
	}

	// If CR is not present in cluster clean ACTDM entry
	if err := deleteDbEntry(ctx, dbQueries, apiCrToDbMappingFromDB.DBRelationKey, dbType_APICRToDatabaseMapping, log, apiCrToDbMappingFromDB); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while deleting APICRToDatabaseMapping entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	managedEnvDb := db.ManagedEnvironment{
		Managedenvironment_id: apiCrToDbMappingFromDB.DBRelationKey,
	}
	if err := dbQueries.GetManagedEnvironmentById(ctx, &managedEnvDb); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while fetching ManagedEnvironment by Id : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(context.Background(), &specialClusterUser); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while fetching SpecialClusterUser from DB.")
		return
	}

	if err := sharedresourceloop.DeleteManagedEnvironmentResources(ctx, apiCrToDbMappingFromDB.DBRelationKey, &managedEnvDb, specialClusterUser, k8sClientFactory, dbQueries, log); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_ManagedEnvironment while cleaning ManagedEnvironment entry "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}
	log.Info("Managed-gitops clean up job for DB deleted ManagedEnvironment: " + apiCrToDbMappingFromDB.DBRelationKey + " entry from DB.")
}

func cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential(ctx context.Context, client client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, log logr.Logger) {
	// Process if CR is of GitOpsDeploymentRepositoryCredential type.
	repoCredentialK8s := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{ObjectMeta: objectMeta}

	// Check if required CR is present in cluster
	if isOrphaned := isRowOrphaned(ctx, client, &apiCrToDbMappingFromDB, &repoCredentialK8s, log); !isOrphaned {
		return
	}

	// If CR is not present in cluster clean ACTDM entry
	if err := deleteDbEntry(ctx, dbQueries, apiCrToDbMappingFromDB.DBRelationKey, dbType_APICRToDatabaseMapping, log, apiCrToDbMappingFromDB); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential while deleting APICRToDatabaseMapping entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	var err error
	var repoCredentialDb db.RepositoryCredentials

	if repoCredentialDb, err = dbQueries.GetRepositoryCredentialsByID(ctx, apiCrToDbMappingFromDB.DBRelationKey); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential while fetching RepositoryCredentials by ID : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	// Clean RepositoryCredential table entry
	if err := deleteDbEntry(ctx, dbQueries, apiCrToDbMappingFromDB.DBRelationKey, dbType_RespositoryCredential, log, repoCredentialK8s); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_RepositoryCredential while deleting RepositoryCredential entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	// Create k8s Operation to delete related CRs using Cluster Agent
	createOperation(ctx, repoCredentialDb.EngineClusterID, repoCredentialDb.RepositoryCredentialsID, repoCredentialK8s.Namespace, db.OperationResourceType_RepositoryCredentials, dbQueries, client, log)
}

func cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun(ctx context.Context, client client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, log logr.Logger) {
	// Process if CR is of GitOpsDeploymentSyncRun type.
	syncRunK8s := managedgitopsv1alpha1.GitOpsDeploymentSyncRun{ObjectMeta: objectMeta}

	// Check if required CR is present in cluster
	if isOrphaned := isRowOrphaned(ctx, client, &apiCrToDbMappingFromDB, &syncRunK8s, log); !isOrphaned {
		return
	}

	// If CR is not present in cluster clean ACTDM entry
	if err := deleteDbEntry(ctx, dbQueries, apiCrToDbMappingFromDB.DBRelationKey, dbType_APICRToDatabaseMapping, log, apiCrToDbMappingFromDB); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while deleting APICRToDatabaseMapping entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	// Clean GitOpsDeploymentSyncRun table entry
	var applicationDb db.Application

	syncOperationDb := db.SyncOperation{SyncOperation_id: apiCrToDbMappingFromDB.DBRelationKey}
	if err := dbQueries.GetSyncOperationById(ctx, &syncOperationDb); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while fetching SyncOperation by Id : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	if err := deleteDbEntry(ctx, dbQueries, apiCrToDbMappingFromDB.DBRelationKey, dbType_SyncOperation, log, syncRunK8s); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while deleting GitOpsDeploymentSyncRun entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	// There is no need to create an Operation if the Application ID is empty possibly due to its deletion.
	// We can delete the SyncRun CR and return.
	if syncOperationDb.Application_id == "" {
		log.Info("Application row not found for SyncOperation. This is normally because the Application has already been deleted.", "syncOperationID", syncOperationDb.SyncOperation_id, "applicationID", syncOperationDb.Application_id)

		if err := client.Delete(ctx, &syncRunK8s); err != nil {
			log.Error(err, "failed to remove GitOpsDeploymentSyncRun when the Application is already deleted")
			return
		}
		return
	}

	applicationDb = db.Application{Application_id: syncOperationDb.Application_id}
	if err := dbQueries.GetApplicationById(ctx, &applicationDb); err != nil {
		log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM_GitOpsDeploymentSyncRun while fetching Application by Id : "+syncOperationDb.Application_id+" from DB.")
		return
	}

	log.Info("Managed-gitops clean up job for DB deleted SyncOperation: " + apiCrToDbMappingFromDB.DBRelationKey + " entry from DB.")

	// Create k8s Operation to delete related CRs using Cluster Agent
	createOperation(ctx, applicationDb.Engine_instance_inst_id, syncOperationDb.SyncOperation_id, syncRunK8s.Namespace, db.OperationResourceType_SyncOperation, dbQueries, client, log)
}

// isRowOrphaned function checks if the given CR pointed by APICRToDBMapping is present in the cluster.
func isRowOrphaned(ctx context.Context, k8sClient client.Client, apiCrToDbMapping *db.APICRToDatabaseMapping, obj client.Object, logger logr.Logger) bool {

	if err := k8sClient.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj); err != nil {
		if apierr.IsNotFound(err) {
			// A) If CR is not found in cluster, proceed to delete related entries from table
			logger.Info("Resource " + obj.GetName() + " not found in Cluster, probably user deleted it, " +
				"but It still exists in DB, hence deleting related database entries.")
			return true
		} else {
			// B) Some other unexpected error occurred, so we just skip it until next time
			logger.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ACTDM while fetching resource from cluster: ")
			return false
		}
		// C) If the CR does exist, but the UID doesn't match, then we can still delete DB entry.
	} else if string(obj.GetUID()) != apiCrToDbMapping.APIResourceUID {
		// This means that another CR exists in the namespace with this name.
		return true
	}

	// CR is present in cluster, no need to delete DB entry
	return false
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
func deleteDbEntry(ctx context.Context, dbQueries db.DatabaseQueries, id string, dbRow dbTableName, logger logr.Logger, t interface{}) error {
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

	if err != nil {
		logger.Error(err, "Error occurred in deleteDbEntry while cleaning "+string(dbRow)+" entry "+id+" from DB.")
		return err
	} else if rowsDeleted == 0 {
		// Log the warning, but continue
		logger.Info("No rows were found in table, while cleaning up after deleted "+string(dbRow), "rowsDeleted", rowsDeleted)
	} else {
		logger.Info("Managed-gitops clean up job for DB deleted" + string(dbRow) + " : " + id + " entry from DB.")
	}
	return nil
}

///////////////
// Clean-up logic for RepositoryCredentials, SyncOperation, ManagedEnvironment, Application, Operation, ClusterUser, ClusterCredential tables and utility functions
// This will clean orphaned entries from RepositoryCredential, SyncOperation, ManagedEnvironment, Application tables if they dont have related entries in DTAM/ACTDM tables.
// and clean Operation, ClusterUser, ClusterCredential tables if they are no longer in use.
///////////////

// cleanOrphanedEntriesfromTable loops through ACTDM in database and returns list of resource IDs for each CR type (i.e. RepositoryCredential, ManagedEnvironment, SyncOperation) .
func cleanOrphanedEntriesfromTable(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, skipDelay bool, log logr.Logger) {

	// Get list of RepositoryCredential/ManagedEnvironment/SyncOperation IDs having entry in ACTDM table.
	listOfRowIdsInAPICRToDBMapping := getListOfCRIdsFromTable(ctx, dbQueries, "multiple", skipDelay, log)

	// Loop through RepositoryCredentials and delete those which are missing entry in ACTDM.
	cleanOrphanedEntriesfromTable_RepositoryCredential(ctx, dbQueries, client, listOfRowIdsInAPICRToDBMapping[dbType_RespositoryCredential], skipDelay, log)

	// Loop through SyncOperations and delete those which are missing entry in ACTDM.
	cleanOrphanedEntriesfromTable_SyncOperation(ctx, dbQueries, client, listOfRowIdsInAPICRToDBMapping[dbType_SyncOperation], skipDelay, log)

	// Loop through ManagedEnvironments and delete those which are missing entry in ACTDM.
	cleanOrphanedEntriesfromTable_ManagedEnvironment(ctx, dbQueries, client, listOfRowIdsInAPICRToDBMapping[dbType_ManagedEnvironment], k8sClientFactory, skipDelay, log)
}

// cleanOrphanedEntriesfromTable_RepositoryCredential loops through RepositoryCredentials in database and verifies they are still valid (Having entry in ACTDM). If not, the resources are deleted.
func cleanOrphanedEntriesfromTable_RepositoryCredential(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, listOfAppsIdsInDTAM map[string]bool, skipDelay bool, l logr.Logger) {

	log := l.WithValues(sharedutil.JobKey, sharedutil.JobKeyValue).
		WithValues(sharedutil.JobTypeKey, "DB_RepositoryCredential")

	offSet := 0

	// Continuously iterate and fetch batches until all entries of RepositoryCredentials table are processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfRepositoryCredentialsFromDB []db.RepositoryCredentials

		// Fetch RepositoryCredentials table entries in batch size as configured above.​
		if err := dbQueries.GetRepositoryCredentialsBatch(ctx, &listOfRepositoryCredentialsFromDB, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_RepositoryCredential while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
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
				err := dbQueries.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstance)
				if err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_RepositoryCredential, while fetching Engine Instance entry "+repCred.EngineClusterID+" from DB.")
					isEngineInstancePresent = false
				}

				if err := deleteDbEntry(ctx, dbQueries, repCred.RepositoryCredentialsID, dbType_RespositoryCredential, log, repCred); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_RepositoryCredential while deleting RepositoryCredential entry : "+repCred.RepositoryCredentialsID+" from DB.")
				}

				// Skip if GitOpsEngineInstance was not found, as we need namespace for creating Operation CR.
				if isEngineInstancePresent {
					// Create k8s Operation to delete related CRs using Cluster Agent.
					createOperation(ctx, repCred.EngineClusterID, repCred.RepositoryCredentialsID, gitopsEngineInstance.Namespace_name, db.OperationResourceType_Application, dbQueries, client, log)
				}
			}
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
}

// cleanOrphanedEntriesfromTable_SyncOperation loops through SyncOperations in database and verifies they are still valid (Having entry in ACTDM). If not, the resources are deleted.
func cleanOrphanedEntriesfromTable_SyncOperation(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, listOfAppsIdsInDTAM map[string]bool, skipDelay bool, l logr.Logger) {

	log := l.WithValues(sharedutil.JobKey, sharedutil.JobKeyValue).
		WithValues(sharedutil.JobTypeKey, "DB_SyncOperation")

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

				isApplicationPresent := true
				applicationDb := db.Application{Application_id: syncOperation.Application_id}
				if err := dbQueries.GetApplicationById(ctx, &applicationDb); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_SyncOperation while fetching Application by Id : "+syncOperation.Application_id+" from DB.")
					isApplicationPresent = false
				}

				// Fetch Application from DB and convert Spec into an Object as we need namespace for creating Operations CR.
				var appArgo fauxargocd.FauxApplication
				if err := yaml.Unmarshal([]byte(applicationDb.Spec_field), &appArgo); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_SyncOperation while unmarshalling application: "+applicationDb.Application_id)
					isApplicationPresent = false
				}

				if err := deleteDbEntry(ctx, dbQueries, syncOperation.SyncOperation_id, dbType_SyncOperation, log, syncOperation); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_SyncOperation while deleting SyncOperation entry : "+syncOperation.SyncOperation_id+" from DB.")
				}

				// Skip if Application was not found, as we need namespace for creating Operation CR.
				if isApplicationPresent {
					// Create k8s Operation to delete related CRs using Cluster Agent
					createOperation(ctx, applicationDb.Engine_instance_inst_id, syncOperation.SyncOperation_id, appArgo.Namespace, db.OperationResourceType_Application, dbQueries, client, log)
				}
			}
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
}

// cleanOrphanedEntriesfromTable_ManagedEnvironment loops through ManagedEnvironments in database and verifies they are still valid (Having entry in ACTDM). If not, the resources are deleted.
func cleanOrphanedEntriesfromTable_ManagedEnvironment(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, listOfRowIdsInAPICRToDBMapping map[string]bool, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, skipDelay bool, l logr.Logger) {

	log := l.WithValues(sharedutil.JobKey, sharedutil.JobKeyValue).
		WithValues(sharedutil.JobTypeKey, "DB_ManagedEnvironment")

	// Retrieve the list of K8sToDBResourceMappings, so that we don't deleted any ManagedEnvironmentes referenced in them
	allK8sToDBResourceMappings := getListOfK8sToDBResourceMapping(ctx, dbQueries, skipDelay, l)

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

	offSet := 0
	// Continuously iterate and fetch batches until all entries of the ManagedEnvironment table are processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfManagedEnvironmentFromDB []db.ManagedEnvironment

		// Fetch ManagedEnvironment table entries in batch size as configured above.​
		if err := dbQueries.GetManagedEnvironmentBatch(ctx, &listOfManagedEnvironmentFromDB, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ManagedEnvironment while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
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
					return
				}

				if err := sharedresourceloop.DeleteManagedEnvironmentResources(ctx, managedEnvironment.Managedenvironment_id, &managedEnvironment, specialClusterUser, k8sClientFactory, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ManagedEnvironment while cleaning ManagedEnvironment entry "+managedEnvironment.Managedenvironment_id+" from DB.")
					return
				}
				log.Info("Managed-gitops clean up job for DB deleted ManagedEnvironment: " + managedEnvironment.Managedenvironment_id + " entry from DB.")
			}
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
}

// cleanOrphanedEntriesfromTable_Application loops through Applications in database and verifies they are still valid (Having entry in DTAM). If not, the resources are deleted.
func cleanOrphanedEntriesfromTable_Application(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, skipDelay bool, l logr.Logger) {

	log := l.WithValues(sharedutil.JobKey, sharedutil.JobKeyValue).
		WithValues(sharedutil.JobTypeKey, "DB_Application")

	// Get list of Applications having entry in DTAM table
	listOfAppsIdsInDTAM := getListOfCRIdsFromTable(ctx, dbQueries, dbType_Application, skipDelay, log)

	offSet := 0
	// Continuously iterate and fetch batches until all entries of Application table are processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfApplicationsFromDB []db.Application

		// Fetch Application table entries in batch size as configured above.​
		if err := dbQueries.GetApplicationBatch(ctx, &listOfApplicationsFromDB, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_Application while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
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
				}

				applicationOwner := db.ApplicationOwner{
					ApplicationOwnerApplicationID: appDB.Application_id,
				}

				if err := dbQueries.GetApplicationOwnerByApplicationID(ctx, &applicationOwner); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while retrieving applicationOwner entry: "+appDB.Application_id)
				} else if err := deleteDbEntry(ctx, dbQueries, applicationOwner.ApplicationOwnerApplicationID, dbType_ApplicationOwner, log, appDB); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while deleting ApplicationOwner entry : "+appDB.Application_id+" from DB.")
				}

				applicationState := db.ApplicationState{
					Applicationstate_application_id: appDB.Application_id,
				}

				if err := dbQueries.GetApplicationStateById(ctx, &applicationState); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while retrieving applicationState from DB: "+applicationState.Applicationstate_application_id)
				} else if err := deleteDbEntry(ctx, dbQueries, applicationState.Applicationstate_application_id, dbType_ApplicationState, log, applicationState); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while deleting ApplicationState entry : "+applicationState.Applicationstate_application_id+" from DB.")
				}

				if err := deleteDbEntry(ctx, dbQueries, appDB.Application_id, dbType_Application, log, appDB); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Application while deleting Application entry : "+appDB.Application_id+" from DB.")
				}

				// Skip if Application was not found, as we need namespace for creating Operation CR.
				if isApplicationPresent {
					// Create k8s Operation to delete related CRs using Cluster Agent
					createOperation(ctx, appDB.Engine_instance_inst_id, appDB.Application_id, appArgo.Namespace, db.OperationResourceType_Application, dbQueries, client, log)
				}
			}
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
}

// cleanOrphanedEntriesfromTable_Operation loops through Operations in database and verifies they are still valid (Having CR in Cluster). If not, the resources are deleted/Creates.
func cleanOrphanedEntriesfromTable_Operation(ctx context.Context, dbQueries db.DatabaseQueries, k8sClient client.Client, skipDelay bool, l logr.Logger) {

	log := l.WithValues(sharedutil.JobKey, sharedutil.JobKeyValue).
		WithValues(sharedutil.JobTypeKey, "DB_Operation")

	offSet := 0
	// Continuously iterate and fetch batches until all entries of Operation table are processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfOperationFromDB []db.Operation

		// Fetch Operation table entries in batch size as configured above.​
		if err := dbQueries.GetOperationBatch(ctx, &listOfOperationFromDB, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_Operation while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
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

				if err := deleteDbEntry(ctx, dbQueries, opDB.Operation_id, dbType_Operation, log, opDB); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_Operation while deleting Operation entry : "+opDB.Operation_id+" from DB.")
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
							continue
						}

						log.Info("Managed-gitops clean up job for DB created K8s Operation CR. Operation CR Name: " + operationCR.Name +
							" Operation CR Namespace" + operationCR.Namespace + ", Operation CR ID" + operationCR.Spec.OperationID)
					} else {
						log.Error(err, "error occurred in cleanOrphanedEntriesfromTable_Operation while fetching Operation: "+operationCR.Name)
					}
				}
			}
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
}

// cleanOrphanedEntriesfromTable_ClusterUser loops through ClusterUsers in database and verifies they are still valid in use. If not, these users are deleted.
func cleanOrphanedEntriesfromTable_ClusterUser(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, skipDelay bool, l logr.Logger) {
	log := l.WithValues("job", "cleanOrphanedEntriesfromTable_ClusterUser")

	// Retrieve a list of user ids from the other tables that reference ClusterUser

	listOfUserIDsFromClusterAccess := getListOfUserIDsfromClusterAccessTable(ctx, dbQueries, skipDelay, log)

	listOfUserIDsFromRepoCred := getListOfUserIDsFromRespositoryCredentialsTable(ctx, dbQueries, skipDelay, log)

	listOfUserIDsFromOperation := getListOfUserIDsfromOperationTable(ctx, dbQueries, skipDelay, log)

	offSet := 0
	// Continuously iterate and fetch batches until all entries of ClusterUser table are processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfClusterUserFromDB []db.ClusterUser

		// Fetch ClusterUser table entries in batch size as configured above.​
		if err := dbQueries.GetClusterUserBatch(ctx, &listOfClusterUserFromDB, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
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
				if err := deleteDbEntry(ctx, dbQueries, userDB.Clusteruser_id, dbType_ClusterUser, log, nil); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ClusterUser while deleting ClusterUser entry : "+userDB.Clusteruser_id+" from DB.")
				}
			}
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
}

// cleanOrphanedEntriesfromTable_ClusterCredential loops through ClusterCredentials in database and verifies they are still in use. If not, the credentials are deleted.
func cleanOrphanedEntriesfromTable_ClusterCredential(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, skipDelay bool, l logr.Logger) {
	log := l.WithValues("job", "cleanOrphanedEntriesfromTable_ClusterCredential")

	// Retrieve a list of cluster credentials from the other tables that reference ClusterCredential table

	listOfClusterCredsFromManagedEnv := getListOfClusterCredentialIDsfromManagedEnvironmenTable(ctx, dbQueries, skipDelay, log)

	listOfClusterCredsFromGitOpsEngine := getListOfClusterCredentialIDsFromGitopsEngineTable(ctx, dbQueries, skipDelay, log)

	offSet := 0
	// Continuously iterate and fetch batches until all entries of ClusterCredentials table are processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfClusterCredentialsFromDB []db.ClusterCredentials

		// Fetch ClusterCredentials table entries in batch size as configured above.​
		if err := dbQueries.GetClusterCredentialsBatch(ctx, &listOfClusterCredentialsFromDB, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in cleanOrphanedEntriesfromTable_ClusterCredential while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
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
				if err := deleteDbEntry(ctx, dbQueries, clusterCred.Clustercredentials_cred_id, dbType_ClusterCredentials, log, nil); err != nil {
					log.Error(err, "Error occurred in cleanOrphanedEntriesfromTable_ClusterCredential while deleting ClusterCredentials entry : "+clusterCred.Clustercredentials_cred_id+" from DB.")
				}
			}
		}

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}
}

func getListOfK8sToDBResourceMapping(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) []db.KubernetesToDBResourceMapping {

	offSet := 0

	var res []db.KubernetesToDBResourceMapping

	// Continuously iterate and fetch batches until all entries of table processed.
	for {
		if offSet != 0 && !skipDelay {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var tempList []db.KubernetesToDBResourceMapping

		// Fetch K8sToDBResourceMapping table entries in batch size as configured above.​
		if err := dbQueries.GetKubernetesToDBResourceMappingBatch(ctx, &tempList, rowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in getListOfK8sToDBResourceMapping while fetching batch from Offset: %d to %d: ",
				offSet, offSet+rowBatchSize))
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(tempList) == 0 {
			break
		}

		res = append(res, tempList...)

		// Skip processed entries in next iteration
		offSet += rowBatchSize
	}

	return res
}

// getListOfCRIdsFromTable loops through DTAMs or APICRToDBMappigs in database and returns list of resource IDs for each CR type (i.e. RepositoryCredential, ManagedEnvironment, SyncOperation).
func getListOfCRIdsFromTable(ctx context.Context, dbQueries db.DatabaseQueries, tableType dbTableName, skipDelay bool, log logr.Logger) map[dbTableName]map[string]bool {

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
				break
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
				break
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

	return crIdMap
}

// getListOfUserIDsfromClusterAccessTable loops through ClusterAccess in database and returns list of user IDs.
func getListOfUserIDsfromClusterAccessTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) map[dbTableName][]string {

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
			break
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

	return crIdMap
}

// getListOfUserIDsFromRespositoryCredentialsTable loops through RepositoryCredentials in database and returns list of resource IDs.
func getListOfUserIDsFromRespositoryCredentialsTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) map[dbTableName][]string {

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
			break
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
	return crIdMap
}

// getListOfClusterCredentialIDsfromManagedEnvironmenTable loops through ManagedEnvironments in database and returns list of resource IDs.
func getListOfClusterCredentialIDsfromManagedEnvironmenTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) map[dbTableName][]string {

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
			break
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
	return crIdMap
}

// getListOfClusterCredentialIDsFromGitopsEngineTable loops through GitopsEngineCluster and returns list of resource IDs.
func getListOfClusterCredentialIDsFromGitopsEngineTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) map[dbTableName][]string {

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
			break
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
	return crIdMap
}

// getListOfUserIDsfromOperationTable loops through Operation in database and returns list of resource IDs.
func getListOfUserIDsfromOperationTable(ctx context.Context, dbQueries db.DatabaseQueries, skipDelay bool, log logr.Logger) map[dbTableName][]string {

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
			break
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
	return crIdMap
}

///////////////
// Utility functions
///////////////

// createOperation creates a k8s operation to inform Cluster Agent about deletion of related k8s CRs.
func createOperation(ctx context.Context, gitopsengineinstanceId, resourceId, namespace string, resourceType db.OperationResourceType, dbQueries db.DatabaseQueries, k8sClient client.Client, log logr.Logger) {

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
		log.Error(err, "unable to fetch special cluster user")
		return
	}

	// Create k8s Operation to inform Cluster Agent to delete related k8s CRs
	if _, _, err := operations.CreateOperation(ctx, false, operationDb,
		specialClusterUser.Clusteruser_id, namespace, dbQueries, k8sClient, log); err != nil {
		log.Error(err, "unable to create operation", "operation", operationDb.ShortString())
	}
}
