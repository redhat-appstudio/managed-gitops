package eventloop

import (
	"context"

	"time"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/metrics"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	operationReconcilerInterval = 10 * time.Minute // Interval in Minutes to reconcile.
)

// OperationReconciler reconciles operation CR
type OperationReconciler struct {
	client.Client
}

func (r *OperationReconciler) StartOperationReconciler() {
	go func() {

		// Timer to trigger Reconciler
		timer := time.NewTimer(time.Duration(operationReconcilerInterval))
		<-timer.C

		ctx := context.Background()
		log := log.FromContext(ctx).WithValues("component", "operation-reconciler")
		_, _ = sharedutil.CatchPanic(func() error {
			listOperationCRs(ctx, r.Client, log)
			return nil
		})

		// Kick off the timer again, once the old task runs.
		// This ensures that at least 'operationReconcilerInterval' time elapses from the end of one run to the beginning of another.
		r.StartOperationReconciler()
	}()

}

func listOperationCRs(ctx context.Context, client client.Client, log logr.Logger) {
	// Get list of Operations from cluster.
	listOfK8sOperation := managedgitopsv1alpha1.OperationList{}
	err := client.List(ctx, &listOfK8sOperation)
	if err != nil {
		log.Error(err, "unable to fetch list of Operation from cluster.")
	}

	metrics.SetNumberOfOperationsCR(len(listOfK8sOperation.Items))
}
