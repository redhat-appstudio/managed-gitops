package eventloop

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	sharedresourceloop "github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	appRowBatchSize            = 50               // Number of rows needs to be fetched in each batch.
	databaseReconcilerInterval = 30 * time.Minute // Interval in Minutes to reconcile Database.
	sleepIntervalsOfBatches    = 1 * time.Second  // Interval in Millisecond between each batch.
)

// A 'dangling' DB entry (for lack of a better term) is a DeploymentToApplicationMapping (DTAM) or APICRToDatabaseMapping (ACTDM)
// row in the database that points to a K8s resource (GitOpsDeployment/GitOpsDeploymentManagedEnvironment/GitOpsDeploymentSyncRun/GitOpsDeploymentRepositoryCredential CR) that no longer exists.
//
// This usually shouldn't occur: usually we should get informed by K8 when a CR is deleted, but this
// is not guaranteed in all cases (for example, if CRs are deleted while the GitOps service is
// down/or otherwise not running).
//
// Thus, it is useful to have some code that will periodically run to clean up old DTAMs and ACTDMs, as as a method of
// background self-healing.
//
// This periodic, background self-healing of DTAMs and ACTDMs is the responsibility of this file.

// DatabaseReconciler reconciles Database entries
type DatabaseReconciler struct {
	client.Client
	DB               db.DatabaseQueries
	K8sClientFactory sharedresourceloop.SRLK8sClientFactory
}

// This function iterates through each entry of DTAM and ACTDM tables in DB and ensures that the required CRs is present in cluster.
func (r *DatabaseReconciler) StartDatabaseReconciler() {
	r.startTimerForNextCycle()
}

func (r *DatabaseReconciler) startTimerForNextCycle() {
	go func() {
		// Timer to trigger Reconciler
		timer := time.NewTimer(time.Duration(databaseReconcilerInterval))
		<-timer.C

		ctx := context.Background()
		log := log.FromContext(ctx).WithValues("component", "database-reconciler")

		_, _ = sharedutil.CatchPanic(func() error {
			deplToAppMappingDbReconcile(ctx, r.DB, r.Client, log)
			apiCRToDbMappingDbReconcile(ctx, r.DB, r.Client, r.K8sClientFactory, log)
			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'databaseReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.startTimerForNextCycle()
	}()

}

// deplToAppMappingDbReconcile loops through the DTAMs in a database, and verifies they are still valid. If not, the resources are deleted.
func deplToAppMappingDbReconcile(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, log logr.Logger) {
	offSet := 0

	log = log.WithValues("job", "deplToAppMappingDbReconcile")

	// Continuously iterate and fetch batches until all entries of DeploymentToApplicationMapping table are processed.
	for {
		if offSet != 0 {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfdeplToAppMapping []db.DeploymentToApplicationMapping

		// Fetch DeploymentToApplicationMapping table entries in batch size as configured above.​
		if err := dbQueries.GetDeploymentToApplicationMappingBatch(ctx, &listOfdeplToAppMapping, appRowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in DTAM Reconcile while fetching batch from Offset: %d to %d: ",
				offSet, offSet+appRowBatchSize))
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

					if err := cleanDeplToAppMappingFromDb(ctx, &deplToAppMappingFromDB, dbQueries, log); err != nil {
						log.Error(err, "Error occurred in DTAM Reconciler while cleaning gitOpsDeployment entries from DB: "+gitOpsDeployment.Name)
					}
				} else {
					// B) Some other unexpected error occurred, so we just skip it until next time
					log.Error(err, "Error occurred in DTAM Reconciler while fetching GitOpsDeployment from cluster: "+gitOpsDeployment.Name)
				}

				// C) If the GitOpsDeployment does exist, but the UID doesn't match, then we can delete DTAM and Application
			} else if string(gitOpsDeployment.UID) != deplToAppMappingFromDB.Deploymenttoapplicationmapping_uid_id {

				// This means that another GitOpsDeployment exists in the namespace with this name.

				if err := cleanDeplToAppMappingFromDb(ctx, &deplToAppMappingFromDB, dbQueries, log); err != nil {
					log.Error(err, "Error occurred in DTAM Reconciler while cleaning gitOpsDeployment entries from DB: "+gitOpsDeployment.Name)
				}

			}

			log.Info("DTAM Reconcile processed deploymentToApplicationMapping entry: " + deplToAppMappingFromDB.Deploymenttoapplicationmapping_uid_id)
		}

		// Skip processed entries in next iteration
		offSet += appRowBatchSize
	}
}

// apiCRToDbMappingDbReconcile loops through the ACTDM in a database, and verifies they are still valid. If not, the resources are deleted.
func apiCRToDbMappingDbReconcile(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, log logr.Logger) {
	offSet := 0
	log = log.WithValues("job", "apiCRToDbMappingDbReconcile")

	// Continuously iterate and fetch batches until all entries of ACTDM table are processed.
	for {
		if offSet != 0 {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfApiCrToDbMapping []db.APICRToDatabaseMapping

		// Fetch ACTDMs table entries in batch size as configured above.​
		if err := dbQueries.GetAPICRToDatabaseMappingBatch(ctx, &listOfApiCrToDbMapping, appRowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in ACTDM Reconcile while fetching batch from Offset: %d to %d: ",
				offSet, offSet+appRowBatchSize))
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
				apiCRToDbMappingDbReconcile_ManagedEnvironment(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, k8sClientFactory, log)
			} else if db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential == apiCrToDbMappingFromDB.APIResourceType {

				// Process if CR is of GitOpsDeploymentRepositoryCredential type.
				apiCRToDBMappingDbReconcile_RepositoryCredential(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, log)
			} else if db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun == apiCrToDbMappingFromDB.APIResourceType {

				// Process if CR is of GitOpsDeploymentSyncRun type.
				apiCRToDBMappingDBReconcile_GitOpsDeploymentSyncRun(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, log)
			}

			log.Info("ACTDM Reconcile processed APICRToDatabaseMapping entry: " + apiCrToDbMappingFromDB.APIResourceUID)
		}

		// Skip processed entries in next iteration
		offSet += appRowBatchSize
	}
}

// cleanDeplToAppMappingFromDb deletes database entries related to a given GitOpsDeployment
func cleanDeplToAppMappingFromDb(ctx context.Context, deplToAppMapping *db.DeploymentToApplicationMapping,
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
	if err := cleanRowFromDB(ctx, dbQueries, deplToAppMapping.Application_id, dbType_ApplicationState, log, deplToAppMapping); err != nil {
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
	if err := cleanRowFromDB(ctx, dbQueries, deplToAppMapping.Deploymenttoapplicationmapping_uid_id, dbType_DeploymentToApplicationMapping, log, deplToAppMapping); err != nil {
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
	if err := cleanRowFromDB(ctx, dbQueries, deplToAppMapping.Application_id, dbType_Application, log, deplToAppMapping); err != nil {
		return err
	}
	return nil
}

func apiCRToDbMappingDbReconcile_ManagedEnvironment(ctx context.Context, client client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, log logr.Logger) {
	// Process if CR is of GitOpsDeploymentManagedEnvironment type.
	managedEnvK8s := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{ObjectMeta: objectMeta}

	// Check if required CR is present in cluster
	if isOrphaned := isRowOrphaned(ctx, client, &apiCrToDbMappingFromDB, &managedEnvK8s, log); !isOrphaned {
		return
	}

	// If CR is not present in cluster clean ACTDM entry
	if err := cleanRowFromDB(ctx, dbQueries, apiCrToDbMappingFromDB.DBRelationKey, dbType_APICRToDatabaseMapping, log, apiCrToDbMappingFromDB); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_ManagedEnvironment while deleting APICRToDatabaseMapping entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	managedEnvDb := db.ManagedEnvironment{
		Managedenvironment_id: apiCrToDbMappingFromDB.DBRelationKey,
	}
	if err := dbQueries.GetManagedEnvironmentById(ctx, &managedEnvDb); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_ManagedEnvironment while fetching ManagedEnvironment by Id : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	var specialClusterUser db.ClusterUser
	if err := dbQueries.GetOrCreateSpecialClusterUser(context.Background(), &specialClusterUser); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_ManagedEnvironment while fetching SpecialClusterUser from DB.")
		return
	}

	if err := sharedresourceloop.DeleteManagedEnvironmentResources(ctx, apiCrToDbMappingFromDB.DBRelationKey, &managedEnvDb, specialClusterUser, k8sClientFactory, dbQueries, log); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_ManagedEnvironment while cleaning ManagedEnvironment entry "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}
}

func apiCRToDBMappingDbReconcile_RepositoryCredential(ctx context.Context, client client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, log logr.Logger) {

	// Process if CR is of GitOpsDeploymentRepositoryCredential type.
	repoCredentialK8s := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{ObjectMeta: objectMeta}

	// Check if required CR is present in cluster
	if isOrphaned := isRowOrphaned(ctx, client, &apiCrToDbMappingFromDB, &repoCredentialK8s, log); !isOrphaned {
		return
	}

	// If CR is not present in cluster clean ACTDM entry
	if err := cleanRowFromDB(ctx, dbQueries, apiCrToDbMappingFromDB.DBRelationKey, dbType_APICRToDatabaseMapping, log, apiCrToDbMappingFromDB); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_RepositoryCredential while deleting APICRToDatabaseMapping entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	var err error
	var repoCredentialDb db.RepositoryCredentials

	if repoCredentialDb, err = dbQueries.GetRepositoryCredentialsByID(ctx, apiCrToDbMappingFromDB.DBRelationKey); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_RepositoryCredential while fetching RepositoryCredentials by ID : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	// Clean RepositoryCredential table entry
	if err := cleanRowFromDB(ctx, dbQueries, apiCrToDbMappingFromDB.DBRelationKey, dbType_RespositoryCredential, log, repoCredentialK8s); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_RepositoryCredential while deleting RepositoryCredential entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	// Create k8s Operation to delete related CRs using Cluster Agent
	createOperation(ctx, repoCredentialDb.EngineClusterID, repoCredentialDb.RepositoryCredentialsID, repoCredentialK8s.Namespace, db.OperationResourceType_RepositoryCredentials, dbQueries, client, log)
}

func apiCRToDBMappingDBReconcile_GitOpsDeploymentSyncRun(ctx context.Context, client client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, log logr.Logger) {
	// Process if CR is of GitOpsDeploymentSyncRun type.
	syncRunK8s := managedgitopsv1alpha1.GitOpsDeploymentSyncRun{ObjectMeta: objectMeta}

	// Check if required CR is present in cluster
	if isOrphaned := isRowOrphaned(ctx, client, &apiCrToDbMappingFromDB, &syncRunK8s, log); !isOrphaned {
		return
	}

	// If CR is not present in cluster clean ACTDM entry
	if err := cleanRowFromDB(ctx, dbQueries, apiCrToDbMappingFromDB.DBRelationKey, dbType_APICRToDatabaseMapping, log, apiCrToDbMappingFromDB); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_GitOpsDeploymentSyncRun while deleting APICRToDatabaseMapping entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	// Clean GitOpsDeploymentSyncRun table entry
	var applicationDb db.Application

	syncOperationDb := db.SyncOperation{SyncOperation_id: apiCrToDbMappingFromDB.DBRelationKey}
	if err := dbQueries.GetSyncOperationById(ctx, &syncOperationDb); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_GitOpsDeploymentSyncRun while fetching SyncOperation by Id : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	applicationDb = db.Application{Application_id: syncOperationDb.Application_id}
	if err := dbQueries.GetApplicationById(ctx, &applicationDb); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_GitOpsDeploymentSyncRun while fetching Application by Id : "+syncOperationDb.Application_id+" from DB.")
		return
	}

	if err := cleanRowFromDB(ctx, dbQueries, apiCrToDbMappingFromDB.DBRelationKey, dbType_SyncOperation, log, syncRunK8s); err != nil {
		log.Error(err, "Error occurred in apiCrToDbMappingDbReconcile_GitOpsDeploymentSyncRun while deleting GitOpsDeploymentSyncRun entry : "+apiCrToDbMappingFromDB.DBRelationKey+" from DB.")
		return
	}

	// Create k8s Operation to delete related CRs using Cluster Agent
	createOperation(ctx, applicationDb.Engine_instance_inst_id, applicationDb.Application_id, syncRunK8s.Namespace, db.OperationResourceType_SyncOperation, dbQueries, client, log)

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
			logger.Error(err, "Error occurred in Database Reconciler while fetching resource from cluster: ")
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
)

// cleanRowFromDB deletes database entry of a given CR
func cleanRowFromDB(ctx context.Context, dbQueries db.DatabaseQueries, id string, dbRow dbTableName, logger logr.Logger, t interface{}) error {
	var rowsDeleted int
	var err error

	// Delete row according to type
	switch dbRow {

	case dbType_RespositoryCredential:
		rowsDeleted, err = dbQueries.DeleteRepositoryCredentialsByID(ctx, id)
	case dbType_ApplicationState:
		rowsDeleted, err = dbQueries.DeleteApplicationStateById(ctx, id)
	case dbType_DeploymentToApplicationMapping:
		rowsDeleted, err = dbQueries.DeleteDeploymentToApplicationMappingByDeplId(ctx, id)
	case dbType_Application:
		rowsDeleted, err = dbQueries.DeleteApplicationById(ctx, id)
	case dbType_SyncOperation:
		rowsDeleted, err = dbQueries.DeleteSyncOperationById(ctx, id)
	case dbType_APICRToDatabaseMapping:
		apiCrToDbMapping, ok := t.(db.APICRToDatabaseMapping)
		if ok {
			rowsDeleted, err = dbQueries.DeleteAPICRToDatabaseMapping(ctx, &apiCrToDbMapping)
		} else {
			return fmt.Errorf("SEVERE: invalid APICRToDatabaseMapping type provided in 'cleanRowFromDB'")
		}
	}

	if err != nil {
		logger.Error(err, "Error occurred in DB Reconciler while cleaning "+string(dbRow)+" entry "+id+" from DB.")
		return err
	} else if rowsDeleted == 0 {
		// Log the warning, but continue
		logger.Info("No rows were found in table, while cleaning up after deleted "+string(dbRow), "rowsDeleted", rowsDeleted)
	} else {
		logger.Info(string(dbRow)+" rows were successfully deleted, while cleaning up after deleted "+string(dbRow), "rowsDeleted", rowsDeleted)
	}
	return nil
}

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
