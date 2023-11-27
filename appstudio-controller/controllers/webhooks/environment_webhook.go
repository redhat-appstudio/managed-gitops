/*
Copyright 2023 Red Hat, Inc.

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

package webhooks

import (
	"context"
	"fmt"
	"net/url"
	"reflect"

	"github.com/go-logr/logr"
	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Webhook describes the data structure for the release webhook
type EnvironmentWebhook struct {
	client client.Client
	log    logr.Logger
}

// change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1alpha1-environment,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=environments,verbs=create;update,versions=v1alpha1,name=venvironment.kb.io,admissionReviewVersions={v1,v1beta1}

func (w *EnvironmentWebhook) Register(mgr ctrl.Manager, log *logr.Logger) error {

	w.client = mgr.GetClient()
	w.log = *log

	return ctrl.NewWebhookManagedBy(mgr).
		For(&appstudiov1alpha1.Environment{}).
		WithValidator(w).
		Complete()
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *EnvironmentWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) error {

	app := obj.(*appstudiov1alpha1.Environment)

	log := r.log.WithName("environment-webhook-create").
		WithValues("controllerKind", "Environment").
		WithValues("name", app.Name).
		WithValues("namespace", app.Namespace)

	log.Info("validating the create request")

	// We use the DNS-1123 format for environment names, so ensure it conforms to that specification
	if len(validation.IsDNS1123Label(app.Name)) != 0 {
		return fmt.Errorf("invalid environment name: %s, an environment resource name must start with a lower case alphabetical character, be under 63 characters, and can only consist of lower case alphanumeric characters or ‘-’", app.Name)
	}

	return validateEnvironment(app)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *EnvironmentWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {

	newApp := newObj.(*appstudiov1alpha1.Environment)

	log := r.log.WithName("environment-webhook-update").
		WithValues("controllerKind", "Environment").
		WithValues("name", newApp.Name).
		WithValues("namespace", newApp.Namespace)

	log.Info("validating the update request")

	return validateEnvironment(newApp)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *EnvironmentWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

// validateEnvironment validates the ingress domain and API URL
func validateEnvironment(r *appstudiov1alpha1.Environment) error {

	unstableConfig := r.Spec.Target
	if unstableConfig != nil {
		// if cluster type is Kubernetes, then Ingress Domain should be set
		if unstableConfig.ClusterType == appstudiov1alpha1.ConfigurationClusterType_Kubernetes && unstableConfig.IngressDomain == "" {
			return fmt.Errorf(appstudiov1alpha1.MissingIngressDomain)
		}

		// if Ingress Domain is provided, we use the DNS-1123 format for ingress domain, so ensure it conforms to that specification
		if unstableConfig.IngressDomain != "" && len(validation.IsDNS1123Subdomain(unstableConfig.IngressDomain)) != 0 {
			return fmt.Errorf(appstudiov1alpha1.InvalidDNS1123Subdomain, unstableConfig.IngressDomain)
		}

	}

	if r.Spec.Target != nil && !reflect.ValueOf(r.Spec.Target.KubernetesClusterCredentials).IsZero() && r.Spec.Target.KubernetesClusterCredentials.APIURL != "" {
		if _, err := url.ParseRequestURI(r.Spec.Target.KubernetesClusterCredentials.APIURL); err != nil {
			return fmt.Errorf(err.Error() + appstudiov1alpha1.InvalidAPIURL)
		}
	}

	return nil
}
