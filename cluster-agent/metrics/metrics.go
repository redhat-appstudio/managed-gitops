package metrics

import (
	"fmt"
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

	ticker := time.NewTicker(60 * time.Minute)

	// Creating channel using make
	tickerChan := make(chan bool)

	go func() {
		for {
			select {
			case <-tickerChan:
				return
			case <-ticker.C:
				// Reset counter to zero every 1 hour
				ClearMetrics()
			}
		}
	}()

	// calling Sleep() method
	time.Sleep(120 * time.Minute)

	// Calling Stop() method to stop the ticker
	ticker.Stop()

	// Setting the value of channel
	tickerChan <- true

	// Printed when the ticker is turned off
	fmt.Println("Ticker is turned off!")

}

func ClearMetrics() {
	OperationStateCompleted.Set(0)
	OperationStateFailed.Set(0)
}

func init() {
	metric.Registry.MustRegister(OperationStateCompleted, OperationStateFailed)
}
