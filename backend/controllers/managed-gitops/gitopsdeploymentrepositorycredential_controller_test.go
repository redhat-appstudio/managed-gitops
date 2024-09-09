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
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("GitOpsDeploymentRepositoryCredential Controller Test", func() {

	Context("Generic tests", func() {

		var k8sClient client.Client
		var namespace corev1.Namespace

		var reconciler GitOpsDeploymentRepositoryCredentialReconciler
		// var mockProcessor mockPreprocessEventLoopProcessor

		var secretName = "test-secret"

		BeforeEach(func() {
			scheme, argocdNamespace, kubesystemNamespace, _, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(argocdNamespace, kubesystemNamespace).Build()

			namespace = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-user",
					UID:  uuid.NewUUID(),
				},
				Spec: corev1.NamespaceSpec{},
			}

			err = k8sClient.Create(context.Background(), &namespace)
			Expect(err).ToNot(HaveOccurred())

			// mockProcessor = mockPreprocessEventLoopProcessor{}
			reconciler = GitOpsDeploymentRepositoryCredentialReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}

		})

		Context("Test findSecretsForRepositoryCredential function", func() {

			When("GitOpsDeploymentRepositoryCredential references a secret in the same namespace", func() {

				It("should return a repository credential", func() {
					By("create a valid repository credential secret")
					secret := createSecretForRepositoryCredential(secretName, true, namespace, k8sClient)

					By("create a repository credential that references the secret")

					repoCred := createRepositoryCredentialTargetingSecret("testRepoCred", secret, namespace, k8sClient)

					Expect(reconciler.findSecretsForRepositoryCredential(context.Background(), &secret)).To(Equal([]reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(&repoCred)}}))
				})
			})

			When("findSecretsForRepositoryCredential is called with an invalid object", func() {

				It("should return an empty request slice", func() {

					invalidObj := corev1.Namespace{}
					Expect(reconciler.findSecretsForRepositoryCredential(context.Background(), &invalidObj)).To(BeEmpty())
				})
			})

			When("findSecretsForRepositoryCredential is called on a non-repo-cred Secret", func() {

				It("should return an empty request slice", func() {

					By("create a INVALID repository credential secret")
					secret := createSecretForRepositoryCredential(secretName, false, namespace, k8sClient)

					By("create a repository credential that references the secret")

					_ = createRepositoryCredentialTargetingSecret("testRepoCred", secret, namespace, k8sClient)

					Expect(reconciler.findSecretsForRepositoryCredential(context.Background(), &secret)).To(BeEmpty())

				})
			})

			When("findSecretsForRepositoryCredential is called on a valid repo-cred Secret, but the Secret in a different namespace than the RepositoryCredential", func() {

				It("should return an empty request slice", func() {

					By("create a valid repository credential secret, but in a different Namespace than the repo cred")
					secret := corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      secretName,
							Namespace: "a-different-namespace",
						},
						Type: sharedutil.RepositoryCredentialSecretType,
					}

					Expect(k8sClient.Create(context.Background(), &secret)).To(Succeed())

					By("create a repository credential that references the secret")

					_ = createRepositoryCredentialTargetingSecret("testRepoCred", secret, namespace, k8sClient)

					Expect(reconciler.findSecretsForRepositoryCredential(context.Background(), &secret)).To(BeEmpty())

				})
			})

		})
	})
})

func createRepositoryCredentialTargetingSecret(name string, secret corev1.Secret, namespace corev1.Namespace, k8sClient client.Client) managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential {

	repoCred := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace.Name,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
			Secret: secret.Name,
		},
	}
	Expect(k8sClient.Create(context.Background(), &repoCred)).To(Succeed())

	return repoCred

}

func createSecretForRepositoryCredential(name string, validSecret bool, namespace corev1.Namespace, k8sClient client.Client) corev1.Secret {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace.Name,
		},
		Type: sharedutil.RepositoryCredentialSecretType,
	}

	if !validSecret {
		secret.Type = ""
	}

	Expect(k8sClient.Create(context.Background(), &secret)).To(Succeed())

	return secret
}
