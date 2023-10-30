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
type PromotionRunWebhook struct {
	client client.Client
	log    logr.Logger
}

// change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-appstudio-redhat-com-v1alpha1-promotionrun,mutating=false,failurePolicy=fail,sideEffects=None,groups=appstudio.redhat.com,resources=promotionruns,verbs=create;update,versions=v1alpha1,name=vpromotionrun.kb.io,admissionReviewVersions=v1

func (w *PromotionRunWebhook) Register(mgr ctrl.Manager, log *logr.Logger) error {

	w.client = mgr.GetClient()
	w.log = *log

	return ctrl.NewWebhookManagedBy(mgr).
		For(&appstudiov1alpha1.PromotionRun{}).
		WithValidator(w).
		Complete()
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *PromotionRunWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) error {

	app := obj.(*appstudiov1alpha1.PromotionRun)

	log := r.log.WithName("promotionrun-webhook-create").
		WithValues("controllerKind", "PromotionRun").
		WithValues("name", app.Name).
		WithValues("namespace", app.Namespace)

	log.Info("validating the create request")

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *PromotionRunWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {

	oldApp := oldObj.(*appstudiov1alpha1.PromotionRun)
	newApp := newObj.(*appstudiov1alpha1.PromotionRun)

	log := r.log.WithName("promotionrun-webhook-update").
		WithValues("controllerKind", "PromotionRun").
		WithValues("name", newApp.Name).
		WithValues("namespace", newApp.Namespace)

	log.Info("validating the update request")

	if !reflect.DeepEqual(newApp.Spec, oldApp.Spec) {
		return fmt.Errorf("spec cannot be updated to %+v", newApp.Spec)
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *PromotionRunWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}
