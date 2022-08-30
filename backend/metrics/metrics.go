package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	Gitopsdepl = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "active_gitopsDeployments",
			Help:        "Total Number of gitopsDeployments active",
			ConstLabels: map[string]string{"gitopsDeployment": "success"},
		},
	)
	GitopsdeplFailures = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "gitopsDeployments_failures",
			Help:        "Total Number of failed gitopsDeployments",
			ConstLabels: map[string]string{"gitopsDeployment": "fail"},
		},
	)
)
