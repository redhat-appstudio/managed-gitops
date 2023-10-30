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
	"testing"

	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestPromotionRunValidatingWebhook(t *testing.T) {

	originalPromotionRun := appstudiov1alpha1.PromotionRun{
		Spec: appstudiov1alpha1.PromotionRunSpec{
			Snapshot:    "test-snapshot-a",
			Application: "test-app-a",
			ManualPromotion: appstudiov1alpha1.ManualPromotionConfiguration{
				TargetEnvironment: "test-env-a",
			},
		},
	}

	tests := []struct {
		testName      string                         // Name of test
		testData      appstudiov1alpha1.PromotionRun // Test data to be passed to webhook function
		expectedError string                         // Expected error message from webhook function
	}{
		{
			testName: "No error when Spec is same.",
			testData: appstudiov1alpha1.PromotionRun{
				Spec: appstudiov1alpha1.PromotionRunSpec{
					Snapshot:    "test-snapshot-a",
					Application: "test-app-a",
					ManualPromotion: appstudiov1alpha1.ManualPromotionConfiguration{
						TargetEnvironment: "test-env-a",
					},
				},
			},
			expectedError: "",
		},

		{
			testName: "Error occurs when Spec.Snapshot is changed.",
			testData: appstudiov1alpha1.PromotionRun{
				Spec: appstudiov1alpha1.PromotionRunSpec{
					Snapshot:    "test-snapshot-a-changed",
					Application: "test-app-a",
					ManualPromotion: appstudiov1alpha1.ManualPromotionConfiguration{
						TargetEnvironment: "test-env-a",
					},
				},
			},
			expectedError: "spec cannot be updated to {Snapshot:test-snapshot-a-changed Application:test-app-a ManualPromotion:{TargetEnvironment:test-env-a} AutomatedPromotion:{InitialEnvironment:}}",
		},

		{
			testName: "Error occurs when Spec.Application is changed.",
			testData: appstudiov1alpha1.PromotionRun{
				Spec: appstudiov1alpha1.PromotionRunSpec{
					Snapshot:    "test-snapshot-a",
					Application: "test-app-a-changed",
					ManualPromotion: appstudiov1alpha1.ManualPromotionConfiguration{
						TargetEnvironment: "test-env-a",
					},
				},
			},
			expectedError: "spec cannot be updated to {Snapshot:test-snapshot-a Application:test-app-a-changed ManualPromotion:{TargetEnvironment:test-env-a} AutomatedPromotion:{InitialEnvironment:}}",
		},

		{
			testName: "Error occurs when Spec.Application is changed.",
			testData: appstudiov1alpha1.PromotionRun{
				Spec: appstudiov1alpha1.PromotionRunSpec{
					Snapshot:    "test-snapshot-a",
					Application: "test-app-a-changed",
					ManualPromotion: appstudiov1alpha1.ManualPromotionConfiguration{
						TargetEnvironment: "test-env-a-changed",
					},
				},
			},
			expectedError: "spec cannot be updated to {Snapshot:test-snapshot-a Application:test-app-a-changed ManualPromotion:{TargetEnvironment:test-env-a-changed} AutomatedPromotion:{InitialEnvironment:}}",
		},

		{
			testName: "Error occurs when Spec.AutomatedPromotion is added.",
			testData: appstudiov1alpha1.PromotionRun{
				Spec: appstudiov1alpha1.PromotionRunSpec{
					Snapshot:    "test-snapshot-a",
					Application: "test-app-a-changed",
					AutomatedPromotion: appstudiov1alpha1.AutomatedPromotionConfiguration{
						InitialEnvironment: "test-env-a",
					},
				},
			},
			expectedError: "spec cannot be updated to {Snapshot:test-snapshot-a Application:test-app-a-changed ManualPromotion:{TargetEnvironment:} AutomatedPromotion:{InitialEnvironment:test-env-a}}",
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			promotionRunWebhook := PromotionRunWebhook{
				log: zap.New(zap.UseFlagOptions(&zap.Options{
					Development: true,
					TimeEncoder: zapcore.ISO8601TimeEncoder,
				})),
			}

			err := promotionRunWebhook.ValidateUpdate(context.Background(), &originalPromotionRun, &test.testData)

			if test.expectedError == "" {
				assert.Nil(t, err)
			} else {
				assert.Contains(t, err.Error(), test.expectedError)
			}
		})
	}
}
