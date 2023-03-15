package metrics

import (
	"time"

	metric "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var (
	OperationStateCompleted = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "operationDB_completedState",
			Help:        "Number of Operation DB rows that were set to completed state in the last hour",
			ConstLabels: map[string]string{"OperationDBState": "completed"},
		},
	)

	OperationStateFailed = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "operationDB_failedState",
			Help:        "Number of Operations DB rows currently in non-complete state",
			ConstLabels: map[string]string{"OperationDBState": "failed"},
		},
	)
)

func SetOperationDBState(state db.OperationState) {

	if state == db.OperationState_Completed {
		OperationStateCompleted.Inc()
	} else if state == db.OperationState_Failed {
		OperationStateFailed.Inc()
	}

}

// Reset metrics to zero every hour
func SetMetricsCountToZero() {

	ticker := time.NewTicker(time.Minute)
	go func() {
		for {

			<-ticker.C

			ClearMetrics()

		}
	}()

	// To keep the program runnning indefinitely, we can use an empty select statement
	select {}

}

func ClearMetrics() {
	OperationStateCompleted.Set(0)
	OperationStateFailed.Set(0)
}

func init() {
	metric.Registry.MustRegister(OperationStateCompleted, OperationStateFailed)
}
