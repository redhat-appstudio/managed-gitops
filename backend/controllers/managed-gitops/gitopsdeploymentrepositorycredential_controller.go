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

package managedgitops

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/preprocess_event_loop"
)

// GitOpsDeploymentRepositoryCredentialReconciler reconciles a GitOpsDeploymentRepositoryCredential object
type GitOpsDeploymentRepositoryCredentialReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	PreprocessEventLoop *preprocess_event_loop.PreprocessEventLoop
}

//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeploymentrepositorycredentials,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeploymentrepositorycredentials/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeploymentrepositorycredentials/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *GitOpsDeploymentRepositoryCredentialReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	_ = log.FromContext(ctx).
		WithName(logutil.LogLogger_managed_gitops)

	rClient := sharedutil.IfEnabledSimulateUnreliableClient(r.Client)

	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Namespace,
		},
	}
	if err := rClient.Get(ctx, client.ObjectKeyFromObject(&namespace), &namespace); err != nil {
		return ctrl.Result{}, err
	}

	r.PreprocessEventLoop.EventReceived(req, eventlooptypes.GitOpsDeploymentRepositoryCredentialTypeName, rClient,
		eventlooptypes.RepositoryCredentialModified, string(namespace.UID))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitOpsDeploymentRepositoryCredentialReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.findSecretsForRepositoryCredential),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Complete(r)
}

func (r *GitOpsDeploymentRepositoryCredentialReconciler) findSecretsForRepositoryCredential(secret client.Object) []reconcile.Request {
	ctx := context.Background()
	handlerLog := log.FromContext(ctx).
		WithName(logutil.LogLogger_managed_gitops)

	secretObj, ok := secret.(*corev1.Secret)

	if !ok {
		handlerLog.Error(nil, "incompatible object in the Environment mapping function, expected a Secret")
		return []reconcile.Request{}
	}

	if secretObj.Type != sharedutil.RepositoryCredentialSecretType {
		return []reconcile.Request{}
	}

	repoCredList := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialList{}

	if err := r.List(ctx, &repoCredList, &client.ListOptions{Namespace: secretObj.Namespace}); err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}

	for idx := range repoCredList.Items {
		repoCredCR := repoCredList.Items[idx]

		if repoCredCR.Spec.Secret == secretObj.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&repoCredCR),
			})
		}
	}

	return requests
}
