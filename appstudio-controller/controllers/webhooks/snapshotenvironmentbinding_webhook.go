/*
Copyright 2021-2022 Red Hat, Inc.

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
	"reflect"

	"github.com/go-logr/logr"
	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Webhook describes the data structure for the release webhook
type SnapshotEnvironmentBindingWebhook struct {
	client client.Client
	log    logr.Logger
}

var snapshotEnvironmentBindingClientFromManager client.Client

// change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1alpha1-snapshotenvironmentbinding,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=snapshotenvironmentbindings,verbs=create;update,versions=v1alpha1,name=vsnapshotenvironmentbinding.kb.io,admissionReviewVersions=v1

func (w *SnapshotEnvironmentBindingWebhook) Register(mgr ctrl.Manager, log *logr.Logger) error {

	w.client = mgr.GetClient()
	w.log = *log

	snapshotEnvironmentBindingClientFromManager = w.client

	return ctrl.NewWebhookManagedBy(mgr).
		For(&appstudiov1alpha1.SnapshotEnvironmentBinding{}).
		WithValidator(w).
		Complete()
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *SnapshotEnvironmentBindingWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) error {

	app := obj.(*appstudiov1alpha1.SnapshotEnvironmentBinding)

	log := r.log.WithName("snapshotenvironmentbinding-webhook-create").
		WithValues("controllerKind", "SnapshotEnvironmentBinding").
		WithValues("name", app.Name).
		WithValues("namespace", app.Namespace)

	log.Info("validating the create request")

	if err := validateSEB(app); err != nil {
		return fmt.Errorf("invalid SnapshotEnvironmentBinding: %v", err)
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *SnapshotEnvironmentBindingWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {

	oldApp := oldObj.(*appstudiov1alpha1.SnapshotEnvironmentBinding)
	newApp := newObj.(*appstudiov1alpha1.SnapshotEnvironmentBinding)

	log := r.log.WithName("snapshotenvironmentbinding-webhook-update").
		WithValues("controllerKind", "SnapshotEnvironmentBinding").
		WithValues("name", newApp.Name).
		WithValues("namespace", newApp.Namespace)

	log.Info("validating the update request")

	if !reflect.DeepEqual(oldApp.Spec.Application, newApp.Spec.Application) {
		return fmt.Errorf("application field cannot be updated to %+v", newApp.Spec.Application)
	}

	if !reflect.DeepEqual(oldApp.Spec.Environment, newApp.Spec.Environment) {
		return fmt.Errorf("environment field cannot be updated to %+v", newApp.Spec.Environment)
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *SnapshotEnvironmentBindingWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

func validateSEB(newBinding *appstudiov1alpha1.SnapshotEnvironmentBinding) error {

	if snapshotEnvironmentBindingClientFromManager == nil {
		return fmt.Errorf("webhook not initialized")
	}

	// Retrieve the list of existing SnapshotEnvironmentBindings from the namespace
	existingSEBs := appstudiov1alpha1.SnapshotEnvironmentBindingList{}

	if err := snapshotEnvironmentBindingClientFromManager.List(context.Background(), &existingSEBs, &client.ListOptions{Namespace: newBinding.Namespace}); err != nil {
		return fmt.Errorf("failed to list existing SnapshotEnvironmentBindings: %v", err)
	}

	// Check if any existing SEB has the same Application/Environment combination
	for _, existingSEB := range existingSEBs.Items {

		// Don't match the SEB on itself
		if existingSEB.Name == newBinding.Name && existingSEB.Namespace == newBinding.Namespace {
			continue
		}

		if existingSEB.Spec.Application == newBinding.Spec.Application && existingSEB.Spec.Environment == newBinding.Spec.Environment {
			return fmt.Errorf("duplicate combination of Application (%s) and Environment (%s). Duplicated by: %s", newBinding.Spec.Application, newBinding.Spec.Environment, existingSEB.Name)
		}
	}

	return nil
}
