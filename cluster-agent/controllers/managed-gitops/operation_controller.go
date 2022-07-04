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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/managed-gitops/eventloop"
)

// OperationReconciler reconciles a Operation object
type OperationReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	ControllerEventLoop *eventloop.OperationEventLoop
	DB                  db.DatabaseQueries
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
