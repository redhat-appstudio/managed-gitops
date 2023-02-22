package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var (
	OperationDBRows = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "total_number_of_operatioDB_rows",
			Help:        "Total number of operation DB rows",
			ConstLabels: map[string]string{"name": "total_operation_DB_rows"},
		},
	)

	OperationDBRowsInWaitingState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "operatioDB_rows_in_waiting_state",
			Help:        "Number of operation DB rows in waiting state",
			ConstLabels: map[string]string{"operationDBState": "Waiting"},
		},
	)

	OperationDBRowsIn_InProgressState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "operatioDB_rows_in_In_progress_state",
			Help:        "Number of operation DB rows in in_progress state",
			ConstLabels: map[string]string{"operationDBState": "In_Progress"},
		},
	)

	OperationDBRowsInCompletedState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "operatioDB_rows_in_completed_state",
			Help:        "Number of operation DB rows in completed state",
			ConstLabels: map[string]string{"operationDBState": "Completed"},
		},
	)

	OperationDBRowsInErrorState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "operatioDB_rows_in_error_state",
			Help:        "Number of operation DB rows in error state",
			ConstLabels: map[string]string{"operationDBState": "Failed"},
		},
	)

	TotalOperationDBRowsInCompletedState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "total_operationDB_rows_in_complete_state",
			Help:        "Number of Operation DB rows in complete state",
			ConstLabels: map[string]string{"operationDBRow": "CompleteState"},
		},
	)

	TotalOperationDBRowsInNonCompleteState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "operationDB_rows_in_non_complete_state",
			Help:        "Number of Operation DB rows in non complete state",
			ConstLabels: map[string]string{"operationDBRow": "NonCompleteState"},
		},
	)
)

func SetTotalCountOfOperationDBRows(count int) {
	OperationDBRows.Set((float64)(count))
}

// SetCountOfOperationDBRows counts the operation DB rows in In_Progress, Waiting, Completed and Failed state
func SetCountOfOperationDBRows(state string, count int) {
	if state == string(db.OperationState_In_Progress) {
		OperationDBRowsIn_InProgressState.Set((float64)(count))
	}
	if state == string(db.OperationState_Waiting) {
		OperationDBRowsInWaitingState.Set((float64)(count))
	}
	if state == string(db.OperationState_Completed) {
		OperationDBRowsInCompletedState.Set((float64)(count))
	}
	if state == string(db.OperationState_Failed) {
		OperationDBRowsInErrorState.Set((float64)(count))
	}
}

// SetCountOfOperationDBRowsInCompleteAndNonCompleteState counts the operation DB rows in complete(Complete and Error state) and non-complete(In_Progress and Waiting) state
func SetCountOfOperationDBRowsInCompleteAndNonCompleteState(complete bool, count int) {
	if complete {
		TotalOperationDBRowsInCompletedState.Set((float64)(count))
	}
	if !complete {
		TotalOperationDBRowsInNonCompleteState.Set((float64)(count))
	}
}

func ClearDBMetrics() {
	OperationDBRows.Set(0)
	OperationDBRowsInWaitingState.Set(0)
	OperationDBRowsIn_InProgressState.Set(0)
	OperationDBRowsInCompletedState.Set(0)
	OperationDBRowsInErrorState.Set(0)
	TotalOperationDBRowsInCompletedState.Set(0)
	TotalOperationDBRowsInNonCompleteState.Set(0)
}
