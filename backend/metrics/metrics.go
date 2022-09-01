package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	Gitopsdepl = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "active_gitopsDeployments",
			Help:        "Total Number of gitopsDeployments active",
			ConstLabels: map[string]string{"gitopsDeployment": "success"},
		},
	)
	GitopsdeplFailures = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "gitopsDeployments_failures",
			Help:        "Total Number of failed gitopsDeployments",
			ConstLabels: map[string]string{"gitopsDeployment": "fail"},
		},
	)
)

// func init() {
// 	r := prometheus.NewRegistry()
// 	r.MustRegister(Gitopsdepl)
// 	r.MustRegister(GitopsdeplFailures)
// }
