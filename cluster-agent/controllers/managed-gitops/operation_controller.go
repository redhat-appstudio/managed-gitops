/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/managed-gitops/eventloop"
)

const (
	garbageCollectionInterval = 10 * time.Minute
)

// OperationReconciler reconciles a Operation object
type OperationReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	ControllerEventLoop *eventloop.OperationEventLoop
}

//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=operations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=operations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=operations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *OperationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	contextLog := log.FromContext(ctx)

	contextLog.Info("Operation event seen in reconciler: " + req.NamespacedName.String())

	r.ControllerEventLoop.EventReceived(req, r.Client)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OperationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&managedgitopsv1alpha1.Operation{}).
		Complete(r)
}

type garbageCollector struct {
	db db.DatabaseQueries
}

// NewGarbageCollector creates a new instance of garbageCollector for Operations
func NewGarbageCollector(dbQueries db.DatabaseQueries) *garbageCollector {
	return &garbageCollector{
		db: dbQueries,
	}
}

// StartGarbageCollector starts a goroutine that removes the expired operations after a specified interval
func (g *garbageCollector) StartGarbageCollector() {
	g.startGarbageCollectionCycle()
}

func (g *garbageCollector) startGarbageCollectionCycle() {
	go func() {
		for {
			// garbage collect the operations after a specified interval
			<-time.After(garbageCollectionInterval)

			ctx := context.Background()
			log := log.FromContext(ctx)

			// get failed/completed operations with non-zero gc interval
			operations := []db.Operation{}
			err := g.db.ListOperationsToBeGarbageCollected(ctx, &operations)
			if err != nil {
				log.Error(err, "failed to list operations ready for garbage collection")
			}

			g.garbageCollectOperations(ctx, operations, log)
		}
	}()
}

func (g *garbageCollector) garbageCollectOperations(ctx context.Context, operations []db.Operation, log logr.Logger) {
	for _, operation := range operations {
		// last_state_update + gc_expiration_time < time.Now
		if operation.Last_state_update.Add(operation.GetGCExpirationTime()).Before(time.Now()) {
			_, err := g.db.DeleteOperationById(ctx, operation.Operation_id)
			if err != nil {
				log.Error(err, "failed to delete operation", operation.Operation_id)
				continue
			}
			log.Info("successfully garbage collected operation", "operation_id", operation.Operation_id)
		}
	}
}
