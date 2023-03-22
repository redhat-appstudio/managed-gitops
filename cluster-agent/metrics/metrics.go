package metrics

import (
	"sync"
	"time"

	metric "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
)

var (
	// Acquire this operationsMetricsMutex before updating these values
	operationsMetricsMutex = &sync.Mutex{}

	numberOfFailedOperations_currentCount    float64
	numberOfSucceededOperations_currentCount float64
)

const (
	resetOperationMetricsEveryX = 10 * time.Minute
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
	operationsMetricsMutex.Lock()
	defer operationsMetricsMutex.Unlock()

	if state == db.OperationState_Completed {
		numberOfSucceededOperations_currentCount++

	} else if state == db.OperationState_Failed {
		numberOfFailedOperations_currentCount++
	}

}

func StartGoRoutineCollectOperationMetrics() {
	go func() {
		_, _ = sharedutil.CatchPanic(func() error {
			for {
				time.Sleep(resetOperationMetricsEveryX)
				runCollectOperationMetrics()
			}
		})
	}()
}

func runCollectOperationMetrics() {

	operationsMetricsMutex.Lock()
	defer operationsMetricsMutex.Unlock()

	// Scrape NumberOfFailedOperations_previousHour and NumberOfSucceededOperations_previousHour in prometheus metrics
	OperationStateFailed.Set(numberOfFailedOperations_currentCount)
	OperationStateCompleted.Set(numberOfSucceededOperations_currentCount)

	// Clear the current count
	clearOperationMetricsCount()

}

func clearOperationMetrics() {
	OperationStateCompleted.Set(0)
	OperationStateFailed.Set(0)
}

// Ensure the mutex is owned before calling this.
func clearOperationMetricsCount() {
	numberOfFailedOperations_currentCount = 0
	numberOfSucceededOperations_currentCount = 0
}

func init() {
	metric.Registry.MustRegister(OperationStateCompleted, OperationStateFailed, OperationCR)
}

// TestOnly_runCollectOperationMetrics should only be called from unit tests
func TestOnly_runCollectOperationMetrics() {

	runCollectOperationMetrics()

}

// TestOnly_resetAllMetricsCount should only be called from unit tests.
func TestOnly_resetAllMetricsCount() {

	clearOperationMetrics()

	operationsMetricsMutex.Lock()
	defer operationsMetricsMutex.Unlock()

	clearOperationMetricsCount()
}
