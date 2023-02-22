package eventloop

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	sharedresourceloop "github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/metrics"
)

const (
	databaseMetricsReconcilerInterval = 10 * time.Minute // Interval in Minutes to reconcile metrics Database.
)

// MetricsReconciler reconciles metrics of Database entries
type MetricsReconciler struct {
	client.Client
	DB               db.DatabaseQueries
	K8sClientFactory sharedresourceloop.SRLK8sClientFactory
}

// This function counts the operation DB rows
func (r *MetricsReconciler) StartDatabaseMetricsReconciler() {
	r.startDBMetricsReconcilerForMetrics()
}

func (r *MetricsReconciler) startDBMetricsReconcilerForMetrics() {
	go func() {
		// Timer to trigger Reconciler
		timer := time.NewTimer(time.Duration(databaseMetricsReconcilerInterval))
		<-timer.C

		ctx := context.Background()
		log := log.FromContext(ctx).WithValues("component", "database-metrics-reconciler")

		_, _ = sharedutil.CatchPanic(func() error {
			operationDbReconcile(ctx, r.DB, r.Client, log)
			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'databaseMetricsReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.startDBMetricsReconcilerForMetrics()
	}()

}

// operationDbReconcile counts the total number of operation rows from database
func operationDbReconcile(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, log logr.Logger) {
	var operationDB db.Operation

	// Fetch count of total number of operation DB rows.
	countOperationDBRows, err := dbQueries.CountTotalOperationDBRows(ctx, &operationDB)
	metrics.SetTotalCountOfOperationDBRows(countOperationDBRows)
	if err != nil {
		log.Error(err, "Error occured while fetching the count of total number of operationDB rows from database")
	}

	// Fetch count of the number of operation DB rows with diffrent states(In_Progress, Waiting, Completed and Failed)
	countOperationDBRowsInNonComleteState, err := dbQueries.CountOperationDBRowsByState(ctx, &operationDB)
	if err != nil {
		log.Error(err, "Error occured while fetching the count of the number of operationDB rows based on states from database")
	}

	var inCompleteState1, inCompleteState2 int
	var completeState1, completeState2 int

	for _, op := range countOperationDBRowsInNonComleteState {

		// Update metrics with number of operation DB rows in In_Progress state
		if op.State == string(db.OperationState_In_Progress) {
			inCompleteState1 = op.RowCount
			metrics.SetCountOfOperationDBRows(op.State, op.RowCount)
		}

		// Update metrics with number of operation DB rows in Waiting state
		if op.State == string(db.OperationState_Waiting) {
			inCompleteState2 = op.RowCount
			metrics.SetCountOfOperationDBRows(op.State, op.RowCount)
		}

		// Number of operation DB rows that are not completed: Number of Waiting State + Number of In_progress and update the metrics
		totatIncompleteState := inCompleteState1 + inCompleteState2
		metrics.SetCountOfOperationDBRowsInCompleteAndNonCompleteState(false, totatIncompleteState)

		// Update metrics with number of operation DB rows in Completed state
		if op.State == string(db.OperationState_Completed) {
			completeState1 = op.RowCount
			metrics.SetCountOfOperationDBRows(op.State, op.RowCount)
		}

		// Update metrics with number of operation DB rows in Failed state
		if op.State == string(db.OperationState_Failed) {
			completeState2 = op.RowCount
			metrics.SetCountOfOperationDBRows(op.State, op.RowCount)
		}

		// Number of operation DB rows that are completed: Number in completed state + Number in error state and update the metrics
		totatCompletedState := completeState1 + completeState2
		metrics.SetCountOfOperationDBRowsInCompleteAndNonCompleteState(true, totatCompletedState)

	}

}
