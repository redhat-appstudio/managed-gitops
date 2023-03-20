package metrics

import (
	"sync"
	"time"

	metric "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var (
	NumberOfFailedOperations_currentCount    float64
	NumberOfSucceededOperations_currentCount float64
)

var (
	NumberOfFailedOperations_previousHour    float64
	NumberOfSucceededOperations_previousHour float64
)

var (
	mutex = &sync.Mutex{}
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
			Help:        "Number of Operations DB rows currently in failed state",
			ConstLabels: map[string]string{"OperationDBState": "failed"},
		},
	)
)

func IncreaseOperationDBState(state db.OperationState) {

	if state == db.OperationState_Completed {
		mutex.Lock()
		defer mutex.Unlock()

		NumberOfSucceededOperations_currentCount++

	} else if state == db.OperationState_Failed {
		mutex.Lock()
		defer mutex.Unlock()

		NumberOfFailedOperations_currentCount++

	}

}

func StartGoRoutineRestartMetricsEveryHour() {
	go func() {
		for {

			time.Sleep(10 * time.Minute) // Sleep for 10 minutes

			mutex.Lock()

			NumberOfFailedOperations_previousHour = NumberOfFailedOperations_currentCount
			NumberOfSucceededOperations_previousHour = NumberOfSucceededOperations_currentCount

			// Scrape NumberOfFailedOperations_previousHour and NumberOfSucceededOperations_previousHour in prometheus metrics
			OperationStateFailed.Set(NumberOfFailedOperations_previousHour)
			OperationStateCompleted.Set(NumberOfSucceededOperations_previousHour)

			// Clear the current count
			NumberOfFailedOperations_currentCount = 0
			NumberOfSucceededOperations_currentCount = 0

			mutex.Unlock()

		}
	}()

}

func ClearMetrics() {
	OperationStateCompleted.Set(0)
	OperationStateFailed.Set(0)
}

func init() {
	metric.Registry.MustRegister(OperationStateCompleted, OperationStateFailed)
}
