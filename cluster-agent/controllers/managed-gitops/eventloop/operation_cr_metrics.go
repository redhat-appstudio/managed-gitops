package eventloop

import (
	"context"

	"time"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/metrics"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	operationCRMetricInterval = 10 * time.Minute // Interval in Minutes to reconcile.
)

// OperationCRMetricUpdater reconciles operation CR
type OperationCRMetricUpdater struct {
	client.Client
}

func (r *OperationCRMetricUpdater) StartOperationCRMetricUpdater() {
	go func() {

		// Timer to trigger update
		timer := time.NewTimer(time.Duration(operationCRMetricInterval))
		<-timer.C

		ctx := context.Background()
		log := log.FromContext(ctx).
			WithName(logutil.LogLogger_managed_gitops).
			WithValues(logutil.Log_Component, logutil.Log_Component_Appstudio_Controller)

		_, _ = sharedutil.CatchPanic(func() error {
			updateOperationCRMetrics(ctx, r.Client, log)
			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'operationCRMetricInterval' time elapses from the end of one run to the beginning of another.
		r.StartOperationCRMetricUpdater()
	}()

}

func updateOperationCRMetrics(ctx context.Context, client client.Client, log logr.Logger) {
	// Get list of Operations from cluster.
	listOfK8sOperation := managedgitopsv1alpha1.OperationList{}
	if err := client.List(ctx, &listOfK8sOperation); err != nil {
		log.Error(err, "unable to fetch list of Operation from cluster.")
		return
	}

	metrics.SetNumberOfOperationsCR(len(listOfK8sOperation.Items))
}
