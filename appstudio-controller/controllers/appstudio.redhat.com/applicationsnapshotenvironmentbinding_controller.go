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

package appstudioredhatcom

import (
	"context"
	"fmt"
	"reflect"

	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	apibackend "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApplicationSnapshotEnvironmentBindingReconciler reconciles a ApplicationSnapshotEnvironmentBinding object
type ApplicationSnapshotEnvironmentBindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationsnapshotenvironmentbindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationsnapshotenvironmentbindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=appstudio.redhat.com,resources=applicationsnapshotenvironmentbindings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *ApplicationSnapshotEnvironmentBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	fmt.Println("ApplicationSnapshotEventBinding event: ", req)

	return ctrl.Result{}, nil
}

// Proof-of-concept Reconcile. Comment out the above Reconcile, and rename this to Reconcile, when starting working on this.
func (r *ApplicationSnapshotEnvironmentBindingReconciler) ReconcilePOC(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	binding := &appstudioshared.ApplicationSnapshotEnvironmentBinding{}

	if err := r.Client.Get(ctx, req.NamespacedName, binding); err != nil {
		// Binding doesn't exist: it was deleted.
		// Owner refs will ensure the GitOpsDeployments are deleted, so no work to do.
		return ctrl.Result{}, nil
	}

	expectedDeployments := []apibackend.GitOpsDeployment{}
	for _, component := range binding.Spec.Components {
		expectedDeployments = append(expectedDeployments, generateExpectedGitOpsDeployment(component, *binding))
	}

	statusField := []appstudioshared.BindingStatusGitOpsDeployment{}

	var firstErr error
	// For each deployment, check if it exists, and if it has the expected content.
	// - If not, create/update it.
	for _, expectedGitOpsDeployment := range expectedDeployments {

		if err := processExpectedGitOpsDeployment(ctx, expectedGitOpsDeployment, *binding, r.Client); err != nil {

			if firstErr != nil {
				firstErr = err
				continue
			}
		} else {
			// No error: add to status
			statusField = append(statusField, appstudioshared.BindingStatusGitOpsDeployment{
				ComponentName:    "", // GITOPSRVCE-156: TODO: use the actual component name
				GitOpsDeployment: expectedGitOpsDeployment.Name,
			})
		}
	}

	// Update the status field with statusField vars (even if an error occurred)
	binding.Status.GitOpsDeployments = statusField
	if err := r.Client.Status().Update(ctx, binding); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to update gitopsdeployments status: %v", err)
	}

	if firstErr != nil {
		return ctrl.Result{}, fmt.Errorf("unable to process expected GitOpsDeployment: %v", firstErr)
	}

	return ctrl.Result{}, nil
}

// processExpectedGitOpsDeployment processed the GitOpsDeployment that is expected for a particular Component
func processExpectedGitOpsDeployment(ctx context.Context, expectedGitopsDeployment apibackend.GitOpsDeployment,
	binding appstudioshared.ApplicationSnapshotEnvironmentBinding, k8sClient client.Client) error {

	actualGitOpsDeployment := apibackend.GitOpsDeployment{}

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&expectedGitopsDeployment), &actualGitOpsDeployment); err != nil {

		// A) If the GitOpsDeployment doesn't exist, create it
		if !apierr.IsNotFound(err) {
			return fmt.Errorf("unable to retrieve gitopsdeployment '%s': %v", expectedGitopsDeployment.Name, err)
		}

		if err := k8sClient.Create(ctx, &expectedGitopsDeployment); err != nil {
			return err
		}

		return nil
	}

	// GitOpsDeployment already exists, so compare it with what we expect

	if reflect.DeepEqual(expectedGitopsDeployment.Spec, actualGitOpsDeployment) {
		// B) The GitOpsDeployment is exactly as expected, so return
		return nil
	}

	// C) The GitOpsDeployment should be updated to be consistent with what we expect
	actualGitOpsDeployment.Spec = expectedGitopsDeployment.Spec

	if err := k8sClient.Update(ctx, &actualGitOpsDeployment); err != nil {
		return fmt.Errorf("unable to update '%s', %v", actualGitOpsDeployment.Name, err)
	}

	return nil
}

func generateExpectedGitOpsDeployment(component appstudioshared.BindingComponent, binding appstudioshared.ApplicationSnapshotEnvironmentBinding) apibackend.GitOpsDeployment {

	res := apibackend.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + component.Name,
			Namespace: binding.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.GetControllerOf(&binding),
			},
		},
		Spec: apibackend.GitOpsDeploymentSpec{
			Source: apibackend.ApplicationSource{
				RepoURL:        component.GitOpsRepository.URL,
				Path:           component.GitOpsRepository.Path,
				TargetRevision: component.GitOpsRepository.Branch,
			},
			Type:        apibackend.GitOpsDeploymentSpecType_Automated, // Default to automated, for now
			Destination: apibackend.ApplicationDestination{},           // Default to same namespace, for now
		},
	}

	// If the length of the GitOpsDeployment exceeds the K8s maximum, shorten it to just binding+component
	if len(res.Name) > 250 {
		res.Name = binding.Name + "-" + component.Name
	}

	return res
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationSnapshotEnvironmentBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&appstudioshared.ApplicationSnapshotEnvironmentBinding{}).
		Owns(&apibackend.GitOpsDeployment{}).
		Complete(r)
}
