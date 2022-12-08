package eventloop

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	appRowBatchSize            = 50               // Number of rows needs to be fetched in each batch.
	databaseReconcilerInterval = 30 * time.Minute // Interval in Minutes to reconcile Database.
	sleepIntervalsOfBatches    = 1 * time.Second  // Interval in Millisecond between each batch.
)

// DatabaseReconciler reconciles Database entries
type DatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	DB     db.DatabaseQueries
}

// This function iterates through each entry present of DeploymentToApplicationMapping table in DB and ensures that the required GitOpsDeployment CR is present in cluster.
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
			databaseReconcile(ctx, r.DB, r.Client, log)
			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'databaseReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.startTimerForNextCycle()
	}()

}

func databaseReconcile(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, log logr.Logger) {
	offSet := 0

	// Continuously iterate and fetch batches until all entries of DeploymentToApplicationMapping table are processed.
	for {
		if offSet != 0 {
			time.Sleep(sleepIntervalsOfBatches)
		}

		var listOfdeplToAppMapping []db.DeploymentToApplicationMapping

		// Fetch DeploymentToApplicationMapping table entries in batch size as configured above.​
		if err := dbQueries.GetDeploymentToApplicationMappingBatch(ctx, &listOfdeplToAppMapping, appRowBatchSize, offSet); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred in Database Reconcile while fetching batch from Offset: %d to %d: ",
				offSet, offSet+appRowBatchSize))
			break
		}

		// Break the loop if no entries are left in table to be processed.
		if len(listOfdeplToAppMapping) == 0 {
			log.Info("All DeploymentToApplicationMapping entries are processed by Database Reconciler.")
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
					log.Info("GitOpsDeployment " + gitOpsDeployment.Name + " not found in Cluster, probably user deleted it, " +
						"but it still exists in DB, hence deleting related database entries.")

					// If GitOpsDeployment CR is not found in cluster, delete related entries from table
					err = cleanEntriesFromDb(ctx, &deplToAppMappingFromDB, dbQueries, log)
					if err != nil {
						log.Error(err, "Error occurred in Database Reconciler while cleaning GitOpsDeployment entries from DB: "+gitOpsDeployment.Name)
					}
				} else {
					log.Error(err, "Error occurred in Database Reconciler while fetching GitOpsDeployment from cluster: "+gitOpsDeployment.Name)
				}
			}

			log.Info("Database Reconcile processed deploymentToApplicationMapping entry: " + deplToAppMappingFromDB.Deploymenttoapplicationmapping_uid_id)
		}

		// Skip processed entries in next iteration
		offSet += appRowBatchSize
	}
}

// Delete database entries related to a GitOpsDeployment
func cleanEntriesFromDb(ctx context.Context, deplToAppMapping *db.DeploymentToApplicationMapping,
	dbQueries db.ApplicationScopedQueries, logger logr.Logger) error {
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
	rowsDeleted, err := dbQueries.DeleteApplicationStateById(ctx, deplToAppMapping.Application_id)
	if err != nil {
		log.V(sharedutil.LogLevel_Warn).Error(err, "unable to delete application state by id")
		return err
	} else if rowsDeleted == 0 {
		// Log the warning, but continue
		log.Info("No ApplicationState rows were found, while cleaning up after deleted GitOpsDeployment", "rowsDeleted", rowsDeleted)
	} else {
		log.Info("ApplicationState rows were successfully deleted, while cleaning up after deleted GitOpsDeployment", "rowsDeleted", rowsDeleted)
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
	rowsDeleted, err = dbQueries.DeleteDeploymentToApplicationMappingByDeplId(ctx, deplToAppMapping.Deploymenttoapplicationmapping_uid_id)
	if err != nil {
		log.Error(err, "unable to delete deplToAppMapping by id", "deplToAppMapUid", deplToAppMapping.Deploymenttoapplicationmapping_uid_id)
		return err
	} else if rowsDeleted == 0 {
		// Log the warning, but continue
		log.V(sharedutil.LogLevel_Warn).Error(nil, "unexpected number of rows deleted for deplToAppMapping", "rowsDeleted", rowsDeleted)
	} else {
		log.Info("While cleaning up after deleted GitOpsDeployment, deleted deplToAppMapping", "deplToAppMapUid", deplToAppMapping.Deploymenttoapplicationmapping_uid_id)
	}

	if !dbApplicationFound {
		log.Info("While cleaning up old gitopsdepl entries, the Application row wasn't found. No more work to do.")
		// If the Application CR no longer exists, then our work is done.
		return nil
	}

	// If the Application table entry still exists, finish the cleanup...

	// 4) Remove the Application from the database
	log.Info("GitOpsDeployment was deleted, so deleting Application row from database")
	rowsDeleted, err = dbQueries.DeleteApplicationById(ctx, deplToAppMapping.Application_id)
	if err != nil {
		// Log the error, but continue
		log.Error(err, "unable to delete application by id")
	} else if rowsDeleted == 0 {
		// Log the error, but continue
		log.V(sharedutil.LogLevel_Warn).Error(nil, "unexpected number of rows deleted for application", "rowsDeleted", rowsDeleted)
	}
	return nil
}
