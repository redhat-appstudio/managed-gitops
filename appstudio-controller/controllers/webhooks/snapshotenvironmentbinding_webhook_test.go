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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestSnapshotEnvironmentBindingValidateCreateWebhook(t *testing.T) {
	tests := []struct {
		testName      string                                         // Name of test
		testData      appstudiov1alpha1.SnapshotEnvironmentBinding   // Test data to be passed to ValidateCreate function
		existingSEBs  []appstudiov1alpha1.SnapshotEnvironmentBinding // Existing SnapshotEnvironmentBindings for the namespace
		expectedError string                                         // Expected error message from ValidateCreate function
		warnings      []string
	}{
		{
			testName: "No error when no existing SnapshotEnvironmentBindings with the same combination",
			testData: appstudiov1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"test-key-b": "test-value-b"},
					Name:   "seb1",
				},
				Spec: appstudiov1alpha1.SnapshotEnvironmentBindingSpec{
					Application: "test-app-b",
					Environment: "test-env-b",
				},
			},
			existingSEBs:  []appstudiov1alpha1.SnapshotEnvironmentBinding{},
			expectedError: "",
		},

		{
			testName: "Error occurs when an existing SnapshotEnvironmentBinding has the same combination",
			testData: appstudiov1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"test-key-c": "test-value-c"},
					Name:   "seb2",
				},
				Spec: appstudiov1alpha1.SnapshotEnvironmentBindingSpec{
					Application: "test-app-c",
					Environment: "test-env-c",
				},
			},
			existingSEBs: []appstudiov1alpha1.SnapshotEnvironmentBinding{
				{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"test-key-d": "test-value-d"},
						Name:   "seb1",
					},
					Spec: appstudiov1alpha1.SnapshotEnvironmentBindingSpec{
						Application: "test-app-c",
						Environment: "test-env-c",
					},
				},
			},
			expectedError: "duplicate combination of Application (test-app-c) and Environment (test-env-c)",
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {

			objects := make([]runtime.Object, len(test.existingSEBs))
			for i, seb := range test.existingSEBs {
				objects[i] = &seb
			}

			scheme := runtime.NewScheme()

			err := appstudiov1alpha1.AddToScheme(scheme)
			if err != nil {
				t.Fatalf("failed to setup scheme: %v", err)
			}

			snapshotEnvironmentBindingClientFromManager = fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				Build()

			sebWebhook := SnapshotEnvironmentBindingWebhook{
				log: zap.New(zap.UseFlagOptions(&zap.Options{
					Development: true,
					TimeEncoder: zapcore.ISO8601TimeEncoder,
				})),
			}

			err = sebWebhook.ValidateCreate(context.Background(), &test.testData)

			if test.expectedError == "" {
				assert.Nil(t, err)
			} else {
				if assert.NotNil(t, err) {
					assert.Contains(t, err.Error(), test.expectedError)
				}
			}
		})
	}
}

func TestSnapshotEnvironmentBindingValidateUpdateWebhook(t *testing.T) {

	originalBinding := appstudiov1alpha1.SnapshotEnvironmentBinding{
		ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"test-key-a": "test-value-a"},
		},
		Spec: appstudiov1alpha1.SnapshotEnvironmentBindingSpec{
			Application: "test-app-a",
			Environment: "test-env-a",
		},
	}

	tests := []struct {
		testName      string                                         // Name of test
		testData      appstudiov1alpha1.SnapshotEnvironmentBinding   // Test data to be passed to webhook function
		existingSEBs  []appstudiov1alpha1.SnapshotEnvironmentBinding // Existing SnapshotEnvironmentBindings for the namespace
		expectedError string                                         // Expected error message from webhook function
		warnings      []string
	}{
		{
			testName: "No error when Spec is same.",
			testData: appstudiov1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"test-key-a": "test-value-a"},
				},
				Spec: appstudiov1alpha1.SnapshotEnvironmentBindingSpec{
					Application: "test-app-a",
					Environment: "test-env-a",
				},
			},
			expectedError: "",
		},

		{
			testName: "Error occurs when Spec.Application name is changed.",
			testData: appstudiov1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"test-key-a": "test-value-a"},
				},
				Spec: appstudiov1alpha1.SnapshotEnvironmentBindingSpec{
					Application: "test-app-a-changed",
					Environment: "test-env-a",
				},
			},
			expectedError: "application field cannot be updated to test-app-a-changed",
		},

		{
			testName: "Error occurs when Spec.Environment name is changed.",
			testData: appstudiov1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"test-key-a": "test-value-a"},
				},
				Spec: appstudiov1alpha1.SnapshotEnvironmentBindingSpec{
					Application: "test-app-a",
					Environment: "test-env-a-changed",
				},
			},
			expectedError: "environment field cannot be updated to test-env-a-changed",
		},
		{
			testName: "Error does not occur when an existing SnapshotEnvironmentBinding is updated",
			testData: appstudiov1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"test-key-a": "test-value-a"},
					Name:   "seb1",
				},
				Spec: appstudiov1alpha1.SnapshotEnvironmentBindingSpec{
					Application: "test-app-a",
					Environment: "test-env-a",
				},
			},
			existingSEBs: []appstudiov1alpha1.SnapshotEnvironmentBinding{
				{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"test-key-a": "test-value-a"},
						Name:   "seb1",
					},
					Spec: appstudiov1alpha1.SnapshotEnvironmentBindingSpec{
						Application: "test-app-a",
						Environment: "test-env-a",
					},
				},
			},
			expectedError: "",
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {

			sebWebhook := SnapshotEnvironmentBindingWebhook{
				log: zap.New(zap.UseFlagOptions(&zap.Options{
					Development: true,
					TimeEncoder: zapcore.ISO8601TimeEncoder,
				})),
			}

			err := sebWebhook.ValidateUpdate(context.Background(), &originalBinding, &test.testData)

			if test.expectedError == "" {
				assert.Nil(t, err)
			} else {
				assert.Contains(t, err.Error(), test.expectedError)
			}
		})
	}
}
