/*
Copyright 2022.

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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	sharedoperations "github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
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

//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=operations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=operations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=operations/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *OperationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rClient := sharedutil.IfEnabledSimulateUnreliableClient(r.Client)
	r.ControllerEventLoop.EventReceived(req, rClient)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OperationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&managedgitopsv1alpha1.Operation{}).
		Complete(r)
}

type garbageCollector struct {
	db            db.DatabaseQueries
	k8sClient     client.Client
	taskRetryLoop *sharedutil.TaskRetryLoop
}

// NewGarbageCollector creates a new instance of garbageCollector for Operations
func NewGarbageCollector(dbQueries db.DatabaseQueries, client client.Client) *garbageCollector {
	return &garbageCollector{
		db:            dbQueries,
		k8sClient:     client,
		taskRetryLoop: sharedutil.NewTaskRetryLoop("garbage-collect-operations"),
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

			_, _ = sharedutil.CatchPanic(func() error {
				ctx := context.Background()
				log := log.FromContext(ctx)

				// get failed/completed operations with non-zero gc interval
				operations := []db.Operation{}
				err := g.db.ListOperationsToBeGarbageCollected(ctx, &operations)
				if err != nil {
					log.Error(err, "failed to list operations ready for garbage collection")
				}

				g.garbageCollectOperations(ctx, operations, log)
				return nil
			})
		}
	}()
}

func (g *garbageCollector) garbageCollectOperations(ctx context.Context, operations []db.Operation, log logr.Logger) {
	for _, operation := range operations {
		// last_state_update + gc_expiration_time < time.Now
		if operation.Last_state_update.Add(operation.GetGCExpirationTime()).Before(time.Now()) {
			// remove the Operation from the DB
			_, err := g.db.DeleteOperationById(ctx, operation.Operation_id)
			if err != nil {
				log.Error(err, "failed to delete operation from DB", "operation_id", operation.Operation_id)
				continue
			}

			// remove the Operation resource from the cluster
			operationCR := &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sharedoperations.GenerateOperationCRName(operation),
					Namespace: util.GetGitOpsEngineSingleInstanceNamespace(),
				},
			}

			// retry until the Operation resource is removed from the cluster
			taskName := fmt.Sprintf("garbage-collect-operation-%s", operation.Operation_id)
			gcOperationCRTask := &removeOperationCRTask{g.k8sClient, log, operationCR}
			g.taskRetryLoop.AddTaskIfNotPresent(taskName, gcOperationCRTask, sharedutil.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true})
		}
	}
}

type removeOperationCRTask struct {
	client.Client
	log       logr.Logger
	operation *managedgitopsv1alpha1.Operation
}

func (r *removeOperationCRTask) PerformTask(ctx context.Context) (bool, error) {
	if err := r.Delete(ctx, r.operation); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.log.Error(err, "failed to delete operation from the cluster", "operation_id", r.operation.Spec.OperationID)
		return true, err
	}
	sharedutil.LogAPIResourceChangeEvent(r.operation.Namespace, r.operation.Name, r.operation, sharedutil.ResourceDeleted, r.log)
	r.log.Info("successfully garbage collected operation", "operation_id", r.operation.Spec.OperationID)
	return false, nil
}
