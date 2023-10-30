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
type SnapshotWebhook struct {
	client client.Client
	log    logr.Logger
}

// change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1alpha1-snapshot,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=snapshots,verbs=create;update,versions=v1alpha1,name=vsnapshot.kb.io,admissionReviewVersions=v1

func (w *SnapshotWebhook) Register(mgr ctrl.Manager, log *logr.Logger) error {

	w.client = mgr.GetClient()
	w.log = *log

	return ctrl.NewWebhookManagedBy(mgr).
		For(&appstudiov1alpha1.Snapshot{}).
		WithValidator(w).
		Complete()
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *SnapshotWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) error {

	app := obj.(*appstudiov1alpha1.Snapshot)

	log := r.log.WithName("snapshot-webhook-create").
		WithValues("controllerKind", "Snapshot").
		WithValues("name", app.Name).
		WithValues("namespace", app.Namespace)

	log.Info("validating the create request")

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *SnapshotWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {

	oldApp := oldObj.(*appstudiov1alpha1.Snapshot)
	newApp := newObj.(*appstudiov1alpha1.Snapshot)

	log := r.log.WithName("snapshot-webhook-update").
		WithValues("controllerKind", "Snapshot").
		WithValues("name", newApp.Name).
		WithValues("namespace", newApp.Namespace)

	log.Info("validating the update request")

	if !reflect.DeepEqual(newApp.Spec.Application, oldApp.Spec.Application) {
		return fmt.Errorf("application field cannot be updated to %+v", newApp.Spec.Application)
	}

	if !reflect.DeepEqual(newApp.Spec.Components, oldApp.Spec.Components) {
		return fmt.Errorf("components cannot be updated to %+v", newApp.Spec.Components)
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *SnapshotWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}
