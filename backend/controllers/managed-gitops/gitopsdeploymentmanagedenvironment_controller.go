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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/preprocess_event_loop"
)

// GitOpsDeploymentManagedEnvironmentReconciler reconciles a GitOpsDeploymentManagedEnvironment object
type GitOpsDeploymentManagedEnvironmentReconciler struct {
	client.Client
	Scheme                       *runtime.Scheme
	PreprocessEventLoopProcessor PreprocessEventLoopProcessor
}

//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeploymentmanagedenvironments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeploymentmanagedenvironments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=managed-gitops.redhat.com,resources=gitopsdeploymentmanagedenvironments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.2/pkg/reconcile
func (r *GitOpsDeploymentManagedEnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	ctx = sharedutil.AddKCPClusterToContext(ctx, req.ClusterName)
	_ = log.FromContext(ctx)

	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Namespace,
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(&namespace), &namespace); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to retrieve namespace: %v", err)
	}

	requestsToProcess := []ctrl.Request{}

	// Attempt to retrieve the request as a Secret; if it's not a secret, just pass the event as is.
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, req.NamespacedName, secret); err == nil && secret != nil {
		if secret.Type == sharedutil.ManagedEnvironmentSecretType {

			// Locate any managed environments that reference this Secret, in the same Namespace
			managedEnvList, err := processSecret(ctx, *secret, r.Client)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to process secret resource: %v", err)
			}

			// For each GitOpsDeploymentManagedEnvironment that was found that references the Secret,
			// add the ManagedEnv to the list of requests to process.
			for _, managedEnv := range managedEnvList {
				newReq := ctrl.Request{
					ClusterName: req.ClusterName,
					NamespacedName: types.NamespacedName{
						Namespace: managedEnv.Namespace,
						Name:      managedEnv.Name,
					},
				}
				requestsToProcess = append(requestsToProcess, newReq)
			}

		}
	} else {
		// If it's not a Secret, it's a GitOpsDeploymentManagedEnvironment, so just add it to the request list
		requestsToProcess = append(requestsToProcess, req)
	}

	for idx := range requestsToProcess {
		requestToProcess := requestsToProcess[idx]
		r.PreprocessEventLoopProcessor.callPreprocessEventLoopForManagedEnvironment(requestToProcess, r.Client, namespace)

	}

	return ctrl.Result{}, nil
}

type PreprocessEventLoopProcessor interface {
	callPreprocessEventLoopForManagedEnvironment(requestToProcess ctrl.Request, k8sClient client.Client, namespace corev1.Namespace)
}

func NewDefaultPreProcessEventLoopProcessor(preprocessEventLoop *preprocess_event_loop.PreprocessEventLoop) PreprocessEventLoopProcessor {
	return &DefaultPreProcessEventLoopProcessor{
		PreprocessEventLoop: preprocessEventLoop,
	}
}

var _ PreprocessEventLoopProcessor = &DefaultPreProcessEventLoopProcessor{}

type DefaultPreProcessEventLoopProcessor struct {
	PreprocessEventLoop *preprocess_event_loop.PreprocessEventLoop
}

func (dppelp *DefaultPreProcessEventLoopProcessor) callPreprocessEventLoopForManagedEnvironment(requestToProcess ctrl.Request, k8sClient client.Client, namespace corev1.Namespace) {
	dppelp.PreprocessEventLoop.EventReceived(requestToProcess, eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
		k8sClient,
		eventlooptypes.ManagedEnvironmentModified, string(namespace.UID))
}

func processSecret(ctx context.Context, secret corev1.Secret, k8sClient client.Client) ([]managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, error) {
	managedEnvList := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentList{}

	if err := k8sClient.List(ctx, &managedEnvList, &client.ListOptions{Namespace: secret.Namespace}); err != nil {
		return nil, fmt.Errorf("unable to list Managed Environment resources in namespace '%s': %v", secret.Namespace, err)
	}

	listOfManagedEnvCRsThatReferenceSecret := []managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{}

	for idx := range managedEnvList.Items {
		managedEnvCR := managedEnvList.Items[idx]

		if managedEnvCR.Namespace != secret.Namespace {
			// Sanity check that the managed environment resource is in the same namespace as the Secret
			continue
		}

		if managedEnvCR.Spec.ClusterCredentialsSecret == secret.Name {
			listOfManagedEnvCRsThatReferenceSecret = append(listOfManagedEnvCRsThatReferenceSecret, managedEnvCR)
		}
	}

	return listOfManagedEnvCRsThatReferenceSecret, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitOpsDeploymentManagedEnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{}).
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}
