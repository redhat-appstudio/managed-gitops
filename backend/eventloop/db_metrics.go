package eventloop

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"github.com/redhat-appstudio/managed-gitops/backend/metrics"
)

const (
	databaseMetricsReconcilerInterval = 10 * time.Minute // Interval in Minutes to reconcile metrics Database.
)

// MetricsReconciler reconciles metrics of Database entries
type MetricsReconciler struct {
	client.Client
	DB db.DatabaseQueries
}

func (r *MetricsReconciler) StartDBMetricsReconcilerForMetrics() {
	go func() {
		// Timer to trigger Reconciler
		timer := time.NewTimer(time.Duration(databaseMetricsReconcilerInterval))
		<-timer.C

		ctx := context.Background()
		log := log.FromContext(ctx).
			WithName(logutil.LogLogger_managed_gitops).
			WithValues(logutil.Log_Component, logutil.Log_Component_Backend_DatabaseMetricsReconciler)

		_, _ = sharedutil.CatchPanic(func() error {
			operationDbReconcile(ctx, r.DB, r.Client, log)
			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'databaseMetricsReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.StartDBMetricsReconcilerForMetrics()
	}()

}

// operationDbReconcile counts the total number of operation rows from database
func operationDbReconcile(ctx context.Context, dbQueries db.DatabaseQueries, client client.Client, log logr.Logger) {
	var operationDB db.Operation

	// Fetch count of the number of operation DB rows with different states: In_Progress, Waiting, Completed and Failed
	operationStateCounts, err := dbQueries.CountOperationDBRowsByState(ctx, &operationDB)
	if err != nil {
		log.Error(err, "Error occured while fetching the count of the number of operationDB rows based on states from database")
	}

	var inProgressCount, waitingCount int
	var completedCount, failedCount int
	totalCountOfOperationDBRows := 0

	for i, op := range operationStateCounts {

		totalCountOfOperationDBRows += operationStateCounts[i].RowCount

		// Update metrics with number of operation DB rows in In_Progress state
		if op.State == string(db.OperationState_In_Progress) {
			inProgressCount = op.RowCount
			metrics.SetCountOfOperationDBRows(op.State, op.RowCount)
		}

		// Update metrics with number of operation DB rows in Waiting state
		if op.State == string(db.OperationState_Waiting) {
			waitingCount = op.RowCount
			metrics.SetCountOfOperationDBRows(op.State, op.RowCount)
		}

		// Number of operation DB rows that are not completed: Number of Waiting State + Number of In_progress and update the metrics
		totalIncompleteState := inProgressCount + waitingCount
		metrics.SetCountOfOperationDBRowsInNonCompleteState(totalIncompleteState)

		// Update metrics with number of operation DB rows in Completed state
		if op.State == string(db.OperationState_Completed) {
			completedCount = op.RowCount
			metrics.SetCountOfOperationDBRows(op.State, op.RowCount)
		}

		// Update metrics with number of operation DB rows in Failed state
		if op.State == string(db.OperationState_Failed) {
			failedCount = op.RowCount
			metrics.SetCountOfOperationDBRows(op.State, op.RowCount)
		}

		// Number of operation DB rows that are completed: Number in completed state + Number in error state and update the metrics
		totalCompletedState := completedCount + failedCount
		metrics.SetCountOfOperationDBRowsInCompleteState(totalCompletedState)

	}

	// Update metrics with total number of operation DB rows
	metrics.SetTotalCountOfOperationDBRows(totalCountOfOperationDBRows)

}
