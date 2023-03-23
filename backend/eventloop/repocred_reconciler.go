package eventloop

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	sharedresourceloop "github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
)

const (
	repocredReconcilerInterval = 10 * time.Minute // Interval in Minutes to reconcile Repository Credentials.
)

// RepoCredReconciler reconciles RepositoryCredential entries
type RepoCredReconciler struct {
	client.Client
	DB               db.DatabaseQueries
	K8sClientFactory sharedresourceloop.SRLK8sClientFactory
}

// This function iterates through each entry of RepositoryCredential table in DB and updates the status of the CR.
func (r *RepoCredReconciler) StartRepoCredReconciler() {
	r.startTimerForNextCycle()
}

func (r *RepoCredReconciler) startTimerForNextCycle() {
	go func() {
		// Timer to trigger Reconciler
		timer := time.NewTimer(time.Duration(repocredReconcilerInterval))
		<-timer.C

		ctx := context.Background()
		log := log.FromContext(ctx).WithValues("component", "repocred-reconciler")

		_, _ = sharedutil.CatchPanic(func() error {

			// Reconcile RepositoryCredentials here
			reconcileRepositoryCredentials(ctx, r.DB, r.Client, r.K8sClientFactory, log)

			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'repocredReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.startTimerForNextCycle()
	}()

}

// /////////////
// Reconcile logic for API CR To Database Mapping table and utility functions.
// This will reconcile repository credential entries from ACTDM table and RepoistoryCredential table
// /////////////
func reconcileRepositoryCredentials(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, k8sClientFactory sharedresourceloop.SRLK8sClientFactory, log logr.Logger) {

	offSet := 0
	log = log.WithValues("job", "reconcileRepositoryCredentials")

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
			if db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential == apiCrToDbMappingFromDB.APIResourceType {

				// Process if CR is of GitOpsDeploymentRepositoryCredential type.
				reconcileRepositoryCredentialStatus(ctx, client, dbQueries, apiCrToDbMappingFromDB, objectMeta, log)
			}

			log.Info("RepositoryCredential ACTDM Reconcile processed APICRToDatabaseMapping entry: " + apiCrToDbMappingFromDB.APIResourceUID)
		}

		// Skip processed entries in next iteration
		offSet += appRowBatchSize
	}
}

func reconcileRepositoryCredentialStatus(ctx context.Context, client client.Client, dbQueries db.DatabaseQueries, apiCrToDbMappingFromDB db.APICRToDatabaseMapping, objectMeta metav1.ObjectMeta, log logr.Logger) {

	gitopsDeploymentRepositoryCredentialCR := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{ObjectMeta: objectMeta}

	// Check if required CR is present in cluster. If yes, skip
	if isOrphaned := isRowOrphaned(ctx, client, &apiCrToDbMappingFromDB, &gitopsDeploymentRepositoryCredentialCR, log); isOrphaned {
		return
	}

	// Sanity test for gitopsDeploymentRepositoryCredentialCR.Spec.Secret to be non-empty value
	if gitopsDeploymentRepositoryCredentialCR.Spec.Secret == "" {
		repositoryCredentialStatusConditon := &metav1.Condition{
			Type:    managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
			Reason:  "SecretNotSpecified",
			Status:  metav1.ConditionTrue,
			Message: "Secret cannot be empty",
		}
		sharedresourceloop.UpdateGitopsDeploymentRepositoryCredentialStatus(&gitopsDeploymentRepositoryCredentialCR, ctx, client, nil, repositoryCredentialStatusConditon, log)
	}

}
