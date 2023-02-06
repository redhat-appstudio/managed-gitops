/*
Copyright 2022.

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

package managedgitops

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Secret Controller Test", func() {

	Context("Secrets for GitOpsDeploymentManagedEnvironments", func() {

		var k8sClient client.Client
		var namespace *corev1.Namespace

		// var reconciler GitOpsDeploymentManagedEnvironmentReconciler
		var reconciler SecretReconciler
		var mockProcessor mockPreprocessEventLoopProcessor

		BeforeEach(func() {
			scheme, argocdNamespace, kubesystemNamespace, _, err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(argocdNamespace, kubesystemNamespace).Build()

			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-user",
					UID:  uuid.NewUUID(),
				},
				Spec: corev1.NamespaceSpec{},
			}

			err = k8sClient.Create(context.Background(), namespace)
			Expect(err).To(BeNil())

			mockProcessor = mockPreprocessEventLoopProcessor{}
			reconciler = SecretReconciler{
				Client:                       k8sClient,
				Scheme:                       scheme,
				PreprocessEventLoopProcessor: &mockProcessor,
			}

		})

		It("reconciles on a non-managed-env secret", func() {
			// secret with the wrong type
			// expect: 0
			secret := createSecretForManagedEnv("my-secret", false, *namespace, k8sClient)
			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: secret.Namespace,
					Name:      secret.Name,
				},
			})
			Expect(err).To(BeNil())
			Expect(len(mockProcessor.requestsReceived)).Should(Equal(0))

		})

		It("reconciles on a managed-env secret, but with 0 managed env CRs referring to the secret", func() {
			// secret with the right type, but no managed envs referred to it
			secret := createSecretForManagedEnv("my-secret", true, *namespace, k8sClient)
			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: secret.Namespace,
					Name:      secret.Name,
				},
			})
			Expect(err).To(BeNil())
			Expect(len(mockProcessor.requestsReceived)).Should(Equal(0))
		})

		It("reconciles on a managed-env secret, but with 1 managed env CRs referring to the secret", func() {
			// secret with the right type, and 1 managed env referring to it
			// expect: 1
			secret := createSecretForManagedEnv("my-secret", true, *namespace, k8sClient)
			createManagedEnvTargetingSecret("managed-env", secret, *namespace, k8sClient)
			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: secret.Namespace,
					Name:      secret.Name,
				},
			})
			Expect(err).To(BeNil())
			Expect(len(mockProcessor.requestsReceived)).Should(Equal(1))

		})

		It("reconciles on a managed-env secret, but with multiple managed env CRs referring to the secret", func() {
			// secret with the right type, and 2 managed envs referring to it
			// expect: 2
			secret := createSecretForManagedEnv("my-secret", true, *namespace, k8sClient)
			createManagedEnvTargetingSecret("managed-env1", secret, *namespace, k8sClient)
			createManagedEnvTargetingSecret("managed-env2", secret, *namespace, k8sClient)
			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: secret.Namespace,
					Name:      secret.Name,
				},
			})
			Expect(err).To(BeNil())
			Expect(len(mockProcessor.requestsReceived)).Should(Equal(2))

		})

	})
})

func createSecretForManagedEnv(name string, validSecret bool, namespace corev1.Namespace, k8sClient client.Client) corev1.Secret {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-name",
			Namespace: namespace.Name,
		},
		Type: sharedutil.ManagedEnvironmentSecretType,
	}

	if !validSecret {
		secret.Type = ""
	}

	err := k8sClient.Create(context.Background(), &secret)
	Expect(err).To(BeNil())
	return secret
}

func createManagedEnvTargetingSecret(name string, secret corev1.Secret, namespace corev1.Namespace, k8sClient client.Client) managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment {
	managedEnv := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace.Name,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
			ClusterCredentialsSecret: secret.Name,
		},
	}
	err := k8sClient.Create(context.Background(), &managedEnv)
	Expect(err).To(BeNil())
	return managedEnv

}
