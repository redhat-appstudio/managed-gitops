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
	"fmt"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("GitOpsDeploymentManagedEnvironment Controller Test", func() {

	Context("Generic tests", func() {

		var k8sClient client.Client
		var namespace *corev1.Namespace

		var reconciler GitOpsDeploymentManagedEnvironmentReconciler
		var mockProcessor mockPreprocessEventLoopProcessor

		var secretName = "test-secret"

		BeforeEach(func() {
			scheme, argocdNamespace, kubesystemNamespace, _, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(argocdNamespace, kubesystemNamespace).Build()

			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-user",
					UID:  uuid.NewUUID(),
				},
				Spec: corev1.NamespaceSpec{},
			}

			err = k8sClient.Create(context.Background(), namespace)
			Expect(err).ToNot(HaveOccurred())

			mockProcessor = mockPreprocessEventLoopProcessor{}
			reconciler = GitOpsDeploymentManagedEnvironmentReconciler{
				Client:                       k8sClient,
				Scheme:                       scheme,
				PreprocessEventLoopProcessor: &mockProcessor,
			}

		})

		Context("Basic reconcile managed environment", func() {

			It("reconciles on a managed-env", func() {
				// secret with the right type, and 2 managed envs referring to it
				// expect: 2
				secret := createSecretForManagedEnv("my-secret", true, *namespace, k8sClient)
				managedEnv := createManagedEnvTargetingSecret("managed-env1", secret, *namespace, k8sClient)
				_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: managedEnv.Namespace,
						Name:      managedEnv.Name,
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(mockProcessor.requestsReceived).Should(HaveLen(1))

			})
		})

		Context("Test findSecretsForManagedEnvironment function", func() {

			When("GitOpsDeploymentManagedEnvironment references a secret in the same namespace", func() {

				It("should return a managed environment", func() {
					By("create a managed environment secret type")
					secret := createSecretForManagedEnv(secretName, true, *namespace, k8sClient)

					By("create a managed environment that references the secret")

					managedEnv := createManagedEnvTargetingSecret("testManagedEnv", secret, *namespace, k8sClient)

					Expect(reconciler.findSecretsForManagedEnvironment(&secret)).To(Equal([]reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(&managedEnv)}}))
				})
			})

			When("GitOpsDeploymentManagedEnvironment references a secret of a different type. Expect no Managed Environment to be reconciled", func() {

				It("should return a managed environment", func() {
					By("create a NON-managed environment secret type")
					secret := createSecretForManagedEnv(secretName, true, *namespace, k8sClient)

					By("create a managed environment that references the secret")

					managedEnv := createManagedEnvTargetingSecret("testManagedEnv", secret, *namespace, k8sClient)

					// Now pass the managedEnv (anything other than a secret) and there should be no managed environments returned
					By("check that a non-secret is passed and the code handles it properly")
					Expect(reconciler.findSecretsForManagedEnvironment(&managedEnv)).To(BeEmpty())
				})
			})

			When("GitOpsDeploymentManagedEnvironment references a secret of a different type. Expect no Managed Environment to be reconciled", func() {

				It("should return a managed environment", func() {
					By("create a NON-managed environment secret type")
					secret := createSecretForManagedEnv(secretName, false, *namespace, k8sClient)

					By("create a managed environment that references the secret")

					createManagedEnvTargetingSecret("testManagedEnv", secret, *namespace, k8sClient)

					Expect(reconciler.findSecretsForManagedEnvironment(&secret)).To(BeEmpty())
				})
			})

			When("Five GitOpsDeploymentManagedEnvironment reference a secret in the same namespace", func() {

				It("should return five managed environments", func() {
					By("create a managed environment secret type")
					secret := createSecretForManagedEnv(secretName, true, *namespace, k8sClient)

					By("create a managed environments that references the secret")
					for i := 1; i <= 5; i++ {
						managedEnvCR := createManagedEnvTargetingSecret("my-managed-env-"+fmt.Sprint(i), secret, *namespace, k8sClient)
						Expect(managedEnvCR).ToNot(BeNil())
					}

					Expect(reconciler.findSecretsForManagedEnvironment(&secret)).To(HaveLen(5))

					expectedArray := []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{
								Namespace: "my-user",
								Name:      "my-managed-env-1",
							},
						},
						{
							NamespacedName: types.NamespacedName{
								Namespace: "my-user",
								Name:      "my-managed-env-2",
							},
						},
						{
							NamespacedName: types.NamespacedName{
								Namespace: "my-user",
								Name:      "my-managed-env-3",
							},
						},
						{
							NamespacedName: types.NamespacedName{
								Namespace: "my-user",
								Name:      "my-managed-env-4",
							},
						},
						{
							NamespacedName: types.NamespacedName{
								Namespace: "my-user",
								Name:      "my-managed-env-5",
							},
						},
					}
					Expect(reconciler.findSecretsForManagedEnvironment(&secret)).To(ContainElements(expectedArray))
				})
			})
		})
	})
})

func createSecretForManagedEnv(name string, validSecret bool, namespace corev1.Namespace, k8sClient client.Client) corev1.Secret {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace.Name,
		},
		Type: sharedutil.ManagedEnvironmentSecretType,
	}

	if !validSecret {
		secret.Type = ""
	}

	err := k8sClient.Create(context.Background(), &secret)
	Expect(err).ToNot(HaveOccurred())
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
	Expect(err).ToNot(HaveOccurred())
	return managedEnv

}

// mockPreprocessEventLoopProcessor keeps track of ctrl.Requests that are sent to the preprocess event loop listener, so
// that we can verify that the correct ones were sent.
type mockPreprocessEventLoopProcessor struct {
	requestsReceived []ctrl.Request
}

func (mockProcessor *mockPreprocessEventLoopProcessor) callPreprocessEventLoopForManagedEnvironment(requestToProcess ctrl.Request,
	k8sClient client.Client, namespace corev1.Namespace) {

	mockProcessor.requestsReceived = append(mockProcessor.requestsReceived, requestToProcess)

}
