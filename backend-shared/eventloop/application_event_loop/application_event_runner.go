package applicationeventloop

import (
	"context"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// cleanupOperation cleans up the database entry and (optionally) the CR, once an operation has concluded.
func CleanupOperation(ctx context.Context, dbOperation db.Operation, k8sOperation managedgitopsv1alpha1.Operation, operationNamespace string,
	dbQueries db.ApplicationScopedQueries, gitopsEngineClient client.Client, log logr.Logger) error {

	log = log.WithValues("operation", dbOperation.Operation_id, "namespace", operationNamespace)

	// // Delete the database entry
	// rowsDeleted, err := dbQueries.DeleteOperationById(ctx, dbOperation.Operation_id)
	// if err != nil {
	// 	return err
	// }
	// if rowsDeleted != 1 {
	// 	log.V(sharedutil.LogLevel_Warn).Error(err, "unexpected number of operation rows deleted", "operation-id", dbOperation.Operation_id, "rows", rowsDeleted)
	// }

	log.V(sharedutil.LogLevel_Debug).Info("Deleting operation CR: " + k8sOperation.Name)

	// Optional: Delete the Operation CR
	if err := gitopsEngineClient.Delete(ctx, &k8sOperation); err != nil {
		if !apierr.IsNotFound(err) {
			// Log the error, but don't return it: it's the responsibility of the cluster agent to delete the operation cr
			log.Error(err, "Unable to delete operation")
		}
	}

	return nil

}
