package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("GitOpsDeploymentRepositoryCredential validation webhook", func() {
	var namespace *corev1.Namespace
	var repoCredentialCr *GitOpsDeploymentRepositoryCredential
	var ctx context.Context

	BeforeEach(func() {

		ctx = context.Background()

		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-2",
				UID:  uuid.NewUUID(),
			},
			Spec: corev1.NamespaceSpec{},
		}

		repoCredentialCr = &GitOpsDeploymentRepositoryCredential{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-repo",
				Namespace: namespace.Name,
				UID:       uuid.NewUUID(),
			},
			Spec: GitOpsDeploymentRepositoryCredentialSpec{
				Secret: "test-secret",
			},
		}

	})

	Context("Create GitOpsDeploymentRepositoryCredential CR with invalid Repository URL", func() {
		It("Should fail with error saying repository must begin with ssh:// or https://", func() {

			// TODO: Re-enable webhook tests
			Skip("webhook ports are conflicting")

			err := k8sClient.Create(ctx, namespace)
			Expect(err).ToNot(HaveOccurred())

			repoCredentialCr.Spec.Repository = "smtp://repo-url"
			err = k8sClient.Create(ctx, repoCredentialCr)

			Expect(err).Should(Not(Succeed()))
			Expect(err.Error()).Should(ContainSubstring(error_invalid_repository))

		})
	})

	Context("Update GitOpsDeploymentRepositoryCredential CR with invalid Repository API URL", func() {
		It("Should fail with error saying repository must begin with ssh:// or https://", func() {

			Skip("webhook ports are conflicting")

			repoCredentialCr.Spec.Repository = "https://test-private-url"

			err := k8sClient.Create(ctx, repoCredentialCr)
			Expect(err).Should(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(repoCredentialCr), repoCredentialCr)
			Expect(err).To(Succeed())

			repoCredentialCr.Spec.Repository = "smtp://repo-url"
			err = k8sClient.Update(ctx, repoCredentialCr)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(error_invalid_repository))

			err = k8sClient.Delete(context.Background(), repoCredentialCr)
			Expect(err).ToNot(HaveOccurred())

		})
	})

})
