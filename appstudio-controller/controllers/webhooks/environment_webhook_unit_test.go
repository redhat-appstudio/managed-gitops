//
// Copyright 2023 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package webhooks

import (
	"context"
	"fmt"
	"strings"
	"testing"

	appstudiov1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestEnvironmentCreateValidatingWebhook(t *testing.T) {

	badIngressDomain := "AbADIngr3ssDomaiN.CoM"

	orgEnv := appstudiov1alpha1.Environment{
		ObjectMeta: v1.ObjectMeta{
			Name: "kubernetes-environment",
		},
		Spec: appstudiov1alpha1.EnvironmentSpec{
			UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
				ClusterType: appstudiov1alpha1.ConfigurationClusterType_Kubernetes,
				KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
					IngressDomain: "domain",
				},
			},
		},
	}

	tests := []struct {
		name   string
		newEnv appstudiov1alpha1.Environment
		err    string
	}{
		{
			name: "environment ingress domain is empty when its Kubernetes",
			err:  appstudiov1alpha1.MissingIngressDomain,
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType:                  appstudiov1alpha1.ConfigurationClusterType_Kubernetes,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{},
					},
				},
			},
		},
		{
			name: "environment ingress domain not DNS 1123 compliant",
			err:  fmt.Sprintf(appstudiov1alpha1.InvalidDNS1123Subdomain, badIngressDomain),
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_Kubernetes,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							IngressDomain: badIngressDomain,
						},
					},
				},
			},
		},
		{
			name:   "environment ingress domain is good",
			newEnv: orgEnv,
		},
		{
			name: "environment unstable config is empty",
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: nil,
				},
			},
		},
		{
			name: "environment's ingress domain is empty when its OpenShift",
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							TargetNamespace: "mynamespace",
						},
					},
				},
			},
		},
		{
			name: "environment's ingress domain is provided when its OpenShift",
			err:  fmt.Sprintf(appstudiov1alpha1.InvalidDNS1123Subdomain, badIngressDomain),
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							IngressDomain: badIngressDomain,
						},
					},
				},
			},
		}, {
			name: "environment name must have DNS-1123 format  (test 1)",
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment-1",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							IngressDomain: "domain",
						},
					},
				},
			},
		}, {
			name: "environment name must have DNS-1123 format (test 2)",
			err:  "invalid environment name: Kubernetes-environment, an environment resource name must start with a lower case alphabetical character, be under 63 characters, and can only consist of lower case alphanumeric characters or ‘-’",
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "Kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							IngressDomain: "domain",
						},
					},
				},
			},
		}, {
			name: "environment name must have DNS-1123 format  (test 3)",
			err:  "invalid environment name: kubernetesEnvironment, an environment resource name must start with a lower case alphabetical character, be under 63 characters, and can only consist of lower case alphanumeric characters or ‘-’",
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetesEnvironment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							IngressDomain: "domain",
						},
					},
				},
			},
		}, {
			name: "environment name must have DNS-1123 format  (test 4)",
			err:  "invalid environment name: abcdeabcdeabcdeabcdeabcdeabcdeabcdeabcdeabcdeabcdeabcdeabcdeabcde, an environment resource name must start with a lower case alphabetical character, be under 63 characters, and can only consist of lower case alphanumeric characters or ‘-’",
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: strings.Repeat("abcde", 13),
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							IngressDomain: badIngressDomain,
						},
					},
				},
			},
		},
		{
			name: "environment api url must start with https",
			err:  "invalid URI for request" + appstudiov1alpha1.InvalidAPIURL,
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment-1",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							IngressDomain: "domain",
							APIURL:        "api.test.com",
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			envWebhook := EnvironmentWebhook{
				log: zap.New(zap.UseFlagOptions(&zap.Options{
					Development: true,
					TimeEncoder: zapcore.ISO8601TimeEncoder,
				})),
			}

			err := envWebhook.ValidateCreate(context.Background(), &test.newEnv)

			if test.err == "" {
				assert.Nil(t, err)
			} else {
				assert.Contains(t, err.Error(), test.err)
			}
		})
	}
}

func TestEnvironmentUpdateValidatingWebhook(t *testing.T) {

	badIngressDomain := "AbADIngr3ssDomaiN.CoM"

	orgEnv := appstudiov1alpha1.Environment{
		ObjectMeta: v1.ObjectMeta{
			Name: "kubernetes-environment",
		},
		Spec: appstudiov1alpha1.EnvironmentSpec{
			UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
				ClusterType: appstudiov1alpha1.ConfigurationClusterType_Kubernetes,
				KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
					IngressDomain: "domain",
				},
			},
		},
	}

	tests := []struct {
		name   string
		newEnv appstudiov1alpha1.Environment
		err    string
	}{
		{
			name: "environment ingress domain is empty when its Kubernetes",
			err:  appstudiov1alpha1.MissingIngressDomain,
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType:                  appstudiov1alpha1.ConfigurationClusterType_Kubernetes,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{},
					},
				},
			},
		},
		{
			name: "environment ingress domain not DNS 1123 compliant",
			err:  fmt.Sprintf(appstudiov1alpha1.InvalidDNS1123Subdomain, badIngressDomain),
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_Kubernetes,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							IngressDomain: badIngressDomain,
						},
					},
				},
			},
		},
		{
			name:   "environment ingress domain is good",
			newEnv: orgEnv,
		},
		{
			name: "environment unstable config is empty",
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: nil,
				},
			},
		},
		{
			name: "environment's ingress domain is empty when its OpenShift",
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType:                  appstudiov1alpha1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{},
					},
				},
			},
		},
		{
			name: "environment's ingress domain is provided when its OpenShift",
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							IngressDomain: "domain",
						},
					},
				},
			},
		},
		{
			name: "environment api url must start with https",
			err:  "invalid URI for request" + appstudiov1alpha1.InvalidAPIURL,
			newEnv: appstudiov1alpha1.Environment{
				ObjectMeta: v1.ObjectMeta{
					Name: "kubernetes-environment-1",
				},
				Spec: appstudiov1alpha1.EnvironmentSpec{
					UnstableConfigurationFields: &appstudiov1alpha1.UnstableEnvironmentConfiguration{
						ClusterType: appstudiov1alpha1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiov1alpha1.KubernetesClusterCredentials{
							IngressDomain: "domain",
							APIURL:        "api.test.com",
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			envWebhook := EnvironmentWebhook{
				log: zap.New(zap.UseFlagOptions(&zap.Options{
					Development: true,
					TimeEncoder: zapcore.ISO8601TimeEncoder,
				})),
			}

			err := envWebhook.ValidateUpdate(context.Background(), &appstudiov1alpha1.Environment{}, &test.newEnv)

			if test.err == "" {
				assert.Nil(t, err)
			} else {
				assert.Contains(t, err.Error(), test.err)
			}
		})
	}
}
