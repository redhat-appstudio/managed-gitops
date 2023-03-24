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
	"net/url"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var gitopsdeploymentrepositorycredentiallog = logf.Log.WithName("gitopsdeploymentrepositorycredential-resource")

func (r *GitOpsDeploymentRepositoryCredential) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-managed-gitops-redhat-com-v1alpha1-gitopsdeploymentrepositorycredential,mutating=true,failurePolicy=fail,sideEffects=None,groups=managed-gitops.redhat.com,resources=gitopsdeploymentrepositorycredentials,verbs=create;update,versions=v1alpha1,name=mgitopsdeploymentrepositorycredential.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &GitOpsDeploymentRepositoryCredential{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *GitOpsDeploymentRepositoryCredential) Default() {
	gitopsdeploymentrepositorycredentiallog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-managed-gitops-redhat-com-v1alpha1-gitopsdeploymentrepositorycredential,mutating=false,failurePolicy=fail,sideEffects=None,groups=managed-gitops.redhat.com,resources=gitopsdeploymentrepositorycredentials,verbs=create;update,versions=v1alpha1,name=vgitopsdeploymentrepositorycredential.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &GitOpsDeploymentRepositoryCredential{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeploymentRepositoryCredential) ValidateCreate() error {
	gitopsdeploymentrepositorycredentiallog.Info("validate create", "name", r.Name)

	if r.Spec.Repository != "" {
		apiURL, err := url.ParseRequestURI(r.Spec.Repository)
		if err != nil {
			return fmt.Errorf(err.Error())
		}

		if !(apiURL.Scheme == "https://" || apiURL.Scheme == "ssh://") {
			return fmt.Errorf("repository must begin with ssh:// or https://")
		}
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeploymentRepositoryCredential) ValidateUpdate(old runtime.Object) error {
	gitopsdeploymentrepositorycredentiallog.Info("validate update", "name", r.Name)

	if r.Spec.Repository != "" {
		apiURL, err := url.ParseRequestURI(r.Spec.Repository)
		if err != nil {
			return fmt.Errorf(err.Error())
		}

		if !(apiURL.Scheme == "https://" || apiURL.Scheme == "ssh://") {
			return fmt.Errorf("repository must begin with ssh:// or https://")
		}
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeploymentRepositoryCredential) ValidateDelete() error {
	gitopsdeploymentrepositorycredentiallog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
