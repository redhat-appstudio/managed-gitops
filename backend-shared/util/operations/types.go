package operations

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
)

const KubeSystemNamespace = "kube-system"

// Global variables to identify resources created by Namespace Reconciler.
const (
	IdentifierKey   = "source"
	IdentifierValue = "periodic-cleanup"
)

// Pattern to use for generating unique names for the Operation CR.
const operationCRNamePattern = "operation-%s"

// CreateOperation will create an Operation CR on the target GitOpsEngine cluster, and a corresponding entry in the
// database. It will then wait for that operation to complete (if waitForOperation is true)
// - In order to avoid intermittent issues, the Operation could will keep trying for 60 seconds.
func CreateOperation(ctx context.Context, waitForOperation bool, dbOperationParam db.Operation, clusterUserID string,
	operationNamespace string, dbQueries db.ApplicationScopedQueries, gitopsEngineClient client.Client,
	l logr.Logger) (*managedgitopsv1alpha1.Operation, *db.Operation, error) {

	backoff := sharedutil.ExponentialBackoff{Factor: 1.5, Min: time.Millisecond * 500, Max: time.Second * 5, Jitter: true}

	var (
		opCR *managedgitopsv1alpha1.Operation
		opDB *db.Operation
		err  error
	)

	// Try for up 1 minute
	expireTime := time.Now().Add(1 * time.Minute)
outer_for:
	for {

		if time.Now().After(expireTime) {
			// Expired: break out and return error
			break outer_for
		}

		opCR, opDB, err = createOperationInternal(ctx, waitForOperation, dbOperationParam, clusterUserID, operationNamespace, dbQueries, gitopsEngineClient, l)

		if err != nil {
			// Failure: an error occurred, try again in a moment.
			backoff.DelayOnFail(ctx)
		} else {
			// Success! Break out.
			break outer_for
		}
	}

	return opCR, opDB, err

}

// createOperationInternal is called by CreateOperation, and is the function that does the actual work of creating the
// Operation CR/DB row.
func createOperationInternal(ctx context.Context, waitForOperation bool, dbOperationParam db.Operation, clusterUserID string,
	operationNamespace string, dbQueries db.ApplicationScopedQueries, gitopsEngineClient client.Client,
	l logr.Logger) (*managedgitopsv1alpha1.Operation, *db.Operation, error) {

	var err error
	l = l.WithValues("Operation GitOpsEngineInstanceID", dbOperationParam.Instance_id,
		"Operation ResourceID", dbOperationParam.Resource_id,
		"Operation ResourceType", dbOperationParam.Resource_type,
		"Operation OwnerUserID", clusterUserID,
	)
	var dbOperationList []db.Operation
	if err = dbQueries.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, dbOperationParam.Resource_id, dbOperationParam.Resource_type, &dbOperationList, clusterUserID); err != nil {
		l.Error(err, "unable to fetch list of Operations")
		// We intentionally don't return here: if we were unable to fetch the list,
		// then we will create an Operation as usual (and it might be duplicate, but that's fine)
	}

	// Iterate through existing DB entries for a given resource: look to see if there is already an Operation
	// in the waiting state.
	for idx := range dbOperationList {

		dbOperation := dbOperationList[idx]

		if dbOperation.State != db.OperationState_Waiting {
			continue
		}

		k8sOperation := managedgitopsv1alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateOperationCRName(dbOperation),
				Namespace: operationNamespace,
			},
		}

		if err = gitopsEngineClient.Get(ctx, client.ObjectKeyFromObject(&k8sOperation), &k8sOperation); err != nil {
			l.Error(err, "unable to fetch existing Operation from cluster, skipping.", "Operation k8s Name", k8sOperation.Name)
			// We intentionally don't return here: we keep going through the list, even if an error occurs.
			// Only one needs to match.
		} else {
			// An operation already exists in waiting state, and the Operation CR for it still exists, so we don't need to create
			// a new operation.
			l.Info("Skipping Operation creation, as it already exists for resource.", "existingOperationState", string(dbOperation.State))
			return &k8sOperation, &dbOperation, nil
		}
	}

	dbOperation := db.Operation{
		Instance_id:             dbOperationParam.Instance_id,
		Resource_id:             dbOperationParam.Resource_id,
		Resource_type:           dbOperationParam.Resource_type,
		Operation_owner_user_id: clusterUserID,
		Created_on:              time.Now(),
		Last_state_update:       time.Now(),
		State:                   db.OperationState_Waiting,
		Human_readable_state:    "",
	}

	if err := dbQueries.CreateOperation(ctx, &dbOperation, clusterUserID); err != nil {
		l.Error(err, "Unable to create Operation database row")
		return nil, nil, err
	}
	l.Info("Created Operation database row", "Operation DB ID", dbOperation.Operation_id)

	// Create K8s operation
	operation := managedgitopsv1alpha1.Operation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateOperationCRName(dbOperation),
			Namespace: operationNamespace,
		},
		Spec: managedgitopsv1alpha1.OperationSpec{
			OperationID: dbOperation.Operation_id,
		},
	}

	// Set annotation as an identifier for Operations created by Namespace Reconciler.
	if clusterUserID == db.SpecialClusterUserName {
		operation.Annotations = map[string]string{IdentifierKey: IdentifierValue}
	}

	if err := gitopsEngineClient.Create(ctx, &operation, &client.CreateOptions{}); err != nil {
		l.Error(err, "Unable to create K8s Operation")
		return nil, nil, err
	}
	l.Info("Created K8s Operation CR", "Operation CR Name", operation.Name,
		"Operation CR Namespace", operation.Namespace, "Operation CR ID", operation.Spec.OperationID,
		"Operation CR State", operation.Status)

	// Wait for operation to complete.
	if waitForOperation {
		l.V(sharedutil.LogLevel_Debug).Info("Waiting for Operation to complete")

		if err = waitForOperationToComplete(ctx, &dbOperation, dbQueries, l); err != nil {
			l.Error(err, "operation did not complete", "operation", dbOperation.Operation_id, "namespace", operation.Namespace)
			return nil, nil, err
		}

		l.Info("Operation completed", "operation", fmt.Sprintf("%v", operation.Spec.OperationID))
	}

	return &operation, &dbOperation, nil

}

// cleanupOperation cleans up the operation CR and (optionally) the database entry, once an operation has concluded.
func CleanupOperation(ctx context.Context, dbOperation db.Operation, k8sOperation managedgitopsv1alpha1.Operation, operationNamespace string,
	dbQueries db.ApplicationScopedQueries, gitopsEngineClient client.Client, deleteDBOperation bool, log logr.Logger) error {

	log = log.WithValues("operation", dbOperation.Operation_id, "namespace", operationNamespace)

	if deleteDBOperation {
		// Delete the database entry
		rowsDeleted, err := dbQueries.DeleteOperationById(ctx, dbOperation.Operation_id)
		if err != nil {
			return err
		}
		if rowsDeleted != 1 {
			log.V(sharedutil.LogLevel_Warn).Info("unexpected number of operation rows deleted", "operation-id", dbOperation.Operation_id, "rows", rowsDeleted)
		}
	}

	// Delete the Operation CR
	if err := gitopsEngineClient.Delete(ctx, &k8sOperation); err != nil {
		if !apierr.IsNotFound(err) {
			log.Error(err, "Unable to delete Operation CR")
			return err
		}
	}
	log.V(sharedutil.LogLevel_Debug).Info("Deleted operation CR: " + k8sOperation.Name)

	return nil

}

// GetOperationCRName returns an unique name to be used for the creation of the Operation CR.
func GenerateOperationCRName(dbOperation db.Operation) string {
	return generateUniqueOperationCRName(dbOperation, func(db.Operation) string { return dbOperation.Operation_id })
}

// generateUniqueOperationCRName generates an unique name to be used for the creation of the Operation CR.
// It takes db.Operation as the input and applies the value generated by the uniqueIdFn to the OperationCRNamePattern.
func generateUniqueOperationCRName(dbOperation db.Operation, uniqueIdFn func(db.Operation) string) string {
	return fmt.Sprintf(operationCRNamePattern, uniqueIdFn(dbOperation))
}

// waitForOperationToComplete waits for an Operation database entry to have 'Completed' or 'Failed' status.
func waitForOperationToComplete(ctx context.Context, dbOperation *db.Operation, dbQueries db.ApplicationScopedQueries, log logr.Logger) error {

	backoff := sharedutil.ExponentialBackoff{Factor: 2, Min: time.Duration(100 * time.Millisecond), Max: time.Duration(10 * time.Second), Jitter: true}

	for {

		isComplete, err := IsOperationComplete(ctx, dbOperation, dbQueries)

		if err != nil {
			return fmt.Errorf("an error occurred on waiting for operation to complete: %v", err)
		}

		if isComplete {
			break
		}

		backoff.DelayOnFail(ctx)

		// Break if the request is cancelled, or the timeout expires
		select {
		case <-ctx.Done():
			return fmt.Errorf("operation context is Done() in waitForOperationToComplete")
		default:
		}

	}

	return nil
}

func IsOperationComplete(ctx context.Context, dbOperation *db.Operation, dbQueries db.ApplicationScopedQueries) (bool, error) {

	err := dbQueries.GetOperationById(ctx, dbOperation)
	if err != nil {
		// Either the operation couldn't be found (which shouldn't happen here), or some other issue, so return it
		return false, err
	}

	// Operation is complete if it exists in the DB, and it is completed/failed
	return err == nil && (dbOperation.State == db.OperationState_Completed || dbOperation.State == db.OperationState_Failed), nil
}
