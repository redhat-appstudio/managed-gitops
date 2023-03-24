package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	OperationCR = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "number_of_operationsCR",
			Help:        "number of operations CR on the cluster",
			ConstLabels: map[string]string{"name": "total_operations_CR_on_cluster"},
		},
	)
)

// SetNumberOfOperationsCR sets total number of operation CRs on cluster
func SetNumberOfOperationsCR(count int) {
	OperationCR.Set(float64(count))
}

func ClearOperationMetrics() {
	SetNumberOfOperationsCR(0)
}
