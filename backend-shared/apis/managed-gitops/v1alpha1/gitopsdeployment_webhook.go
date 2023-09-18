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

	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	error_nonempty_namespace_empty_environment = "the environment field should not be empty when the namespace is non-empty"
	error_invalid_sync_option                  = "the specified sync option in .spec.syncPolicy.syncOptions is either mispelled or is not supported by GitOpsDeployment"
	error_invalid_spec_type                    = "spec type must be manual or automated"
)

// log is for logging in this package.
var gitopsdeploymentlog = logf.Log.WithName(logutil.LogLogger_managed_gitops)

func (r *GitOpsDeployment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-managed-gitops-redhat-com-v1alpha1-gitopsdeployment,mutating=true,failurePolicy=fail,sideEffects=None,groups=managed-gitops.redhat.com,resources=gitopsdeployments,verbs=create;update,versions=v1alpha1,name=mgitopsdeployment.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &GitOpsDeployment{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *GitOpsDeployment) Default() {
	log := gitopsdeploymentlog.WithValues(logutil.Log_K8s_Request_Name, r.Name, logutil.Log_K8s_Request_Namespace, r.Namespace, "kind", "GitOpsDeployment")

	log.V(logutil.LogLevel_Debug).Info("default")
}

//+kubebuilder:webhook:path=/validate-managed-gitops-redhat-com-v1alpha1-gitopsdeployment,mutating=false,failurePolicy=fail,sideEffects=None,groups=managed-gitops.redhat.com,resources=gitopsdeployments,verbs=create;update,versions=v1alpha1,name=vgitopsdeployment.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &GitOpsDeployment{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeployment) ValidateCreate() error {

	log := gitopsdeploymentlog.WithValues(logutil.Log_K8s_Request_Name, r.Name, logutil.Log_K8s_Request_Namespace, r.Namespace, "kind", "GitOpsDeployment")

	log.V(logutil.LogLevel_Debug).Info("validate create")

	if err := r.validateGitOpsDeployment(); err != nil {
		log.Info("webhook rejected invalid create", "error", fmt.Sprintf("%v", err))
		return err
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeployment) ValidateUpdate(old runtime.Object) error {

	log := gitopsdeploymentlog.WithValues(logutil.Log_K8s_Request_Name, r.Name, logutil.Log_K8s_Request_Namespace, r.Namespace, "kind", "GitOpsDeployment")

	log.V(logutil.LogLevel_Debug).Info("validate update")

	if err := r.validateGitOpsDeployment(); err != nil {
		log.Info("webhook rejected invalid update", "error", fmt.Sprintf("%v", err))
		return err
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *GitOpsDeployment) ValidateDelete() error {

	log := gitopsdeploymentlog.WithValues(logutil.Log_K8s_Request_Name, r.Name, logutil.Log_K8s_Request_Namespace, r.Namespace, "kind", "GitOpsDeployment")

	log.V(logutil.LogLevel_Debug).Info("validate delete")

	return nil
}

func (r *GitOpsDeployment) validateGitOpsDeployment() error {

	// Check whether Type is manual or automated
	if !(r.Spec.Type == GitOpsDeploymentSpecType_Automated || r.Spec.Type == GitOpsDeploymentSpecType_Manual) {
		return fmt.Errorf(error_invalid_spec_type)
	}

	// Check whether sync options are valid
	if r.Spec.SyncPolicy != nil {
		for _, syncOptionString := range r.Spec.SyncPolicy.SyncOptions {

			if !(syncOptionString == SyncOptions_CreateNamespace_true ||
				syncOptionString == SyncOptions_CreateNamespace_false) {
				return fmt.Errorf(error_invalid_sync_option)
			}

		}
	}

	if r.Spec.Destination.Environment == "" && r.Spec.Destination.Namespace != "" {
		return fmt.Errorf(error_nonempty_namespace_empty_environment)
	}

	return nil
}
