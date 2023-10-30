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

func TestSnapshotValidatingWebhook(t *testing.T) {

	// Create initial Snapshot CR.
	originalSnapshot := appstudiov1alpha1.Snapshot{
		Spec: appstudiov1alpha1.SnapshotSpec{
			Application: "test-app-a",
			Components: []appstudiov1alpha1.SnapshotComponent{
				{
					Name:           "test-component-a",
					ContainerImage: "test-container-image-a",
				},
			},
		},
	}

	tests := []struct {
		testName      string                     // Name of test
		testData      appstudiov1alpha1.Snapshot // Test data to be passed to webhook function
		expectedError string                     // Expected error message from webhook function
	}{
		{
			testName: "No error when Spec is same.",
			testData: appstudiov1alpha1.Snapshot{
				Spec: appstudiov1alpha1.SnapshotSpec{
					Application: "test-app-a",
					Components: []appstudiov1alpha1.SnapshotComponent{
						{
							Name:           "test-component-a",
							ContainerImage: "test-container-image-a",
						},
					},
				},
			},
			expectedError: "",
		}, {
			testName: "Error occurs when Spec.Application name is changed.",
			testData: appstudiov1alpha1.Snapshot{
				Spec: appstudiov1alpha1.SnapshotSpec{
					Application: "test-app-a-changed",
					Components:  originalSnapshot.Spec.Components,
				},
			},
			expectedError: "application field cannot be updated to test-app-a-changed",
		},

		{
			testName: "Error occurs when Spec.Components.Name is changed.",
			testData: appstudiov1alpha1.Snapshot{
				Spec: appstudiov1alpha1.SnapshotSpec{
					Application: originalSnapshot.Spec.Application,
					Components: []appstudiov1alpha1.SnapshotComponent{
						{
							Name:           "test-component-a-changed",
							ContainerImage: "test-container-image-a",
						},
					},
				},
			},
			expectedError: "components cannot be updated to [{Name:test-component-a-changed ContainerImage:test-container-image-a Source:{ComponentSourceUnion:{GitSource:<nil>}}}]",
		},

		{
			testName: "Error occurs when Spec.Components.ContainerImage is changed.",
			testData: appstudiov1alpha1.Snapshot{
				Spec: appstudiov1alpha1.SnapshotSpec{
					Application: "test-app-a",
					Components: []appstudiov1alpha1.SnapshotComponent{
						{
							Name:           "test-component-a",
							ContainerImage: "test-container-image-a-changed",
						},
					},
				},
			},
			expectedError: "components cannot be updated to [{Name:test-component-a ContainerImage:test-container-image-a-changed Source:{ComponentSourceUnion:{GitSource:<nil>}}}]",
		},

		{
			testName: "Error when adding Source",
			testData: appstudiov1alpha1.Snapshot{
				Spec: appstudiov1alpha1.SnapshotSpec{
					Application: "test-app-a",
					Components: []appstudiov1alpha1.SnapshotComponent{
						{
							Name:           "test-component-a",
							ContainerImage: "test-container-image-a",
							Source: appstudiov1alpha1.ComponentSource{
								ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
									GitSource: &appstudiov1alpha1.GitSource{
										URL: "https://github.com/dummy/repo",
									},
								},
							},
						},
					},
				},
			},
			expectedError: "components cannot be updated to [{Name:test-component-a ContainerImage:test-container-image-a Source:{ComponentSourceUnion:{GitSource:",
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {

			snapshotWebhook := SnapshotWebhook{
				log: zap.New(zap.UseFlagOptions(&zap.Options{
					Development: true,
					TimeEncoder: zapcore.ISO8601TimeEncoder,
				})),
			}

			err := snapshotWebhook.ValidateUpdate(context.Background(), &originalSnapshot, &test.testData)

			if test.expectedError == "" {
				assert.Nil(t, err)
			} else {
				assert.Contains(t, err.Error(), test.expectedError)
			}
		})
	}
}
