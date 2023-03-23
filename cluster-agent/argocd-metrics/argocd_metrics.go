package argocdmetrics

import (
	"context"
	"time"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	pollInterval = 30 * time.Second
	windowSize   = 3 * time.Minute
)

var (
	ReconciledArgoAppsPercent = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:        "percent_of_argocd_apps_recently_reconciled",
			Help:        "Percent of argocd applications reconciled in the last three minutes",
			ConstLabels: map[string]string{"gitopsArgoApps": "percent-reconciled"},
		},
	)

	// Namespaces used in unit tests
	testNamespaceNames []string
)

type ReconciliationMetricsUpdater struct {
	client.Client
}

type reconciliationMetricsUpdater struct {
	client     client.Client
	ctx        context.Context
	logger     logr.Logger
	timeWindow time.Time
}

func (m *ReconciliationMetricsUpdater) Start() {
	go func() {
		m.poll(pollInterval)

		// Restart the timer once the task has run.
		// This ensures that at least *interval* time elapses between runs
		m.Start()
	}()
}

func (m *ReconciliationMetricsUpdater) poll(interval time.Duration) {
	timer := time.NewTimer(time.Duration(interval))
	<-timer.C

	ctx := context.Background()
	logger := log.FromContext(ctx).WithValues("component", "cluster-agent-argocd-metrics")

	updater := &reconciliationMetricsUpdater{
		client:     m.Client,
		ctx:        ctx,
		logger:     logger,
		timeWindow: time.Now().Add(-1 * windowSize),
	}
	_, _ = sharedutil.CatchPanic(func() error {
		updater.updateReconciliationMetrics()
		return nil
	})
}

func (m *reconciliationMetricsUpdater) updateReconciliationMetrics() {
	var appTotal, appReconciled float64
	for _, ns := range gitopsNamespaces() {
		total, reconciled := m.reconciliationMetricsForNamespace(ns)
		appTotal += total
		appReconciled += reconciled
	}
	if appTotal > 0.0 {
		ReconciledArgoAppsPercent.Set((appReconciled * 100.0) / appTotal)
	} else {
		ReconciledArgoAppsPercent.Set(0.0)
	}
}

func gitopsNamespaces() []string {
	if len(testNamespaceNames) > 0 {
		return testNamespaceNames
	}
	return []string{
		dbutil.GetGitOpsEngineSingleInstanceNamespace(),
	}
}

func (m *reconciliationMetricsUpdater) reconciliationMetricsForNamespace(namespace string) (total, reconciled float64) {
	apps := &appv1.ApplicationList{}
	if err := m.client.List(m.ctx, apps, &client.ListOptions{Namespace: namespace}); err != nil {
		m.logger.Error(err, "listing applications", "namespace", namespace)
		return
	}
	for _, app := range apps.Items {
		total += 1.0
		reconciled += m.reconciliationMetricsForApplication(&app)
	}
	return
}

func (m *reconciliationMetricsUpdater) reconciliationMetricsForApplication(application *appv1.Application) (reconciled float64) {
	reconciledAt := application.Status.ReconciledAt
	if reconciledAt != nil && reconciledAt.Time.After(m.timeWindow) {
		reconciled = 1.0
	} else {
		reconciled = 0.0
	}
	return
}

func init() {
	metrics.Registry.MustRegister(ReconciledArgoAppsPercent)
}
