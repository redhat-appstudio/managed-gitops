/*
Copyright 2023.

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
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Webhook is an interface that should be implemented by operator webhooks and that allows to have a cohesive way
// of defining them and register them.
type Webhook interface {
	Register(mgr ctrl.Manager, log *logr.Logger) error
}

// SetupWebhooks invoke the Register function of every webhook passed as an argument to this function.
func SetupWebhooks(mgr manager.Manager, webhooks ...Webhook) error {
	log := ctrl.Log.WithName("webhooks")

	for _, webhook := range webhooks {
		err := webhook.Register(mgr, &log)
		if err != nil {
			return err
		}
	}

	return nil
}

// EnabledWebhooks is a slice containing references to all the webhooks that have to be registered
var EnabledWebhooks = []Webhook{
	&EnvironmentWebhook{},
	&PromotionRunWebhook{},
	&SnapshotWebhook{},
	&SnapshotEnvironmentBindingWebhook{},
}
