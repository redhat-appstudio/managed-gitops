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

	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const error_invalid_cluster_api_url = "cluster api url must start with https://"

// log is for logging in this package.
var gitopsdeploymentmanagedenvironmentlog = logf.Log.WithName(logutil.LogLogger_managed_gitops)

func (r *GitOpsDeploymentManagedEnvironment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-managed-gitops-redhat-com-v1alpha1-gitopsdeploymentmanagedenvironment,mutating=true,failurePolicy=fail,sideEffects=None,groups=managed-gitops.redhat.com,resources=gitopsdeploymentmanagedenvironments,verbs=create;update,versions=v1alpha1,name=mgitopsdeploymentmanagedenvironment.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &GitOpsDeploymentManagedEnvironment{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *GitOpsDeploymentManagedEnvironment) Default() {
	gitopsdeploymentmanagedenvironmentlog.Info("default", "name", r.Name)

}

//+kubebuilder:webhook:path=/validate-managed-gitops-redhat-com-v1alpha1-gitopsdeploymentmanagedenvironment,mutating=false,failurePolicy=fail,sideEffects=None,groups=managed-gitops.redhat.com,resources=gitopsdeploymentmanagedenvironments,verbs=create;update,versions=v1alpha1,name=vgitopsdeploymentmanagedenvironment.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &GitOpsDeploymentManagedEnvironment{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeploymentManagedEnvironment) ValidateCreate() error {
	gitopsdeploymentmanagedenvironmentlog.Info("validate create", "name", r.Name)

	if err := r.ValidateGitOpsDeploymentManagedEnv(); err != nil {
		return err
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeploymentManagedEnvironment) ValidateUpdate(old runtime.Object) error {
	gitopsdeploymentmanagedenvironmentlog.Info("validate update", "name", r.Name)

	if err := r.ValidateGitOpsDeploymentManagedEnv(); err != nil {
		return err
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeploymentManagedEnvironment) ValidateDelete() error {
	gitopsdeploymentmanagedenvironmentlog.Info("validate delete", "name", r.Name)

	return nil
}

func (r *GitOpsDeploymentManagedEnvironment) ValidateGitOpsDeploymentManagedEnv() error {
	if r.Spec.APIURL != "" {
		apiURL, err := url.ParseRequestURI(r.Spec.APIURL)
		if err != nil {
			return fmt.Errorf(err.Error())
		}

		if apiURL.Scheme != "https" {
			return fmt.Errorf(error_invalid_cluster_api_url)
		}
	}

	return nil
}
