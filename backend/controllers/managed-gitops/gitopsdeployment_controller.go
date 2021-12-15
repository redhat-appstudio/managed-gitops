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

package managedgitops

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop"
)

// GitOpsDeploymentReconciler reconciles a GitOpsDeployment object
type GitOpsDeploymentReconciler struct {
	client.Client
	WorkspaceName string // not used
	Scheme        *runtime.Scheme
	EventLoop     *eventloop.EventLoop
}

//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeployments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GitOpsDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	namespace := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Namespace,
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&namespace), &namespace); err != nil {
		return ctrl.Result{}, err
	}

	r.EventLoop.EventReceived(req, managedgitopsv1alpha1.GitOpsDeploymentTypeName, r.Client, eventloop.DeploymentModified, string(namespace.UID))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitOpsDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&managedgitopsv1alpha1.GitOpsDeployment{}).
		Complete(r)
}
