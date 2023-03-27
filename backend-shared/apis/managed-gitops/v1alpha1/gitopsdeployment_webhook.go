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

package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var gitopsdeploymentlog = logf.Log.WithName("gitopsdeployment-resource")

func (r *GitOpsDeployment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-managed-gitops-redhat-com-v1alpha1-gitopsdeployment,mutating=true,failurePolicy=fail,sideEffects=None,groups=managed-gitops.redhat.com,resources=gitopsdeployments,verbs=create;update,versions=v1alpha1,name=mgitopsdeployment.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &GitOpsDeployment{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *GitOpsDeployment) Default() {
	gitopsdeploymentlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-managed-gitops-redhat-com-v1alpha1-gitopsdeployment,mutating=false,failurePolicy=fail,sideEffects=None,groups=managed-gitops.redhat.com,resources=gitopsdeployments,verbs=create;update,versions=v1alpha1,name=vgitopsdeployment.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &GitOpsDeployment{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeployment) ValidateCreate() error {
	gitopsdeploymentlog.Info("validate create", "name", r.Name)

	// Check whether Type is manual or automated
	if !(r.Spec.Type == GitOpsDeploymentSpecType_Automated || r.Spec.Type == GitOpsDeploymentSpecType_Manual) {
		return fmt.Errorf("spec type must be manual or automated")
	}

	// Check whether sync options are valid
	if r.Spec.SyncPolicy != nil {
		for _, syncOptionString := range r.Spec.SyncPolicy.SyncOptions {

			if !(syncOptionString == SyncOptions_CreateNamespace_true ||
				syncOptionString == SyncOptions_CreateNamespace_false) {
				return fmt.Errorf("the specified sync option in .spec.syncPolicy.syncOptions is either mispelled or is not supported by GitOpsDeployment")
			}

		}
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeployment) ValidateUpdate(old runtime.Object) error {
	gitopsdeploymentlog.Info("validate update", "name", r.Name)

	// Check whether Type is manual or automated
	if !(r.Spec.Type == GitOpsDeploymentSpecType_Automated || r.Spec.Type == GitOpsDeploymentSpecType_Manual) {
		return fmt.Errorf("spec type must be manual or automated")
	}

	// Check whether sync options are valid
	if r.Spec.SyncPolicy != nil {
		for _, syncOptionString := range r.Spec.SyncPolicy.SyncOptions {

			if !(syncOptionString == SyncOptions_CreateNamespace_true ||
				syncOptionString == SyncOptions_CreateNamespace_false) {
				return fmt.Errorf("the specified sync option in .spec.syncPolicy.syncOptions is either mispelled or is not supported by GitOpsDeployment")
			}

		}
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeployment) ValidateDelete() error {
	gitopsdeploymentlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
