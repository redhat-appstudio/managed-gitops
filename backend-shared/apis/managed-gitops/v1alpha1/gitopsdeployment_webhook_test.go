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

var _ = Describe("GitOpsDeployment validation webhook", func() {
	var namespace *corev1.Namespace
	var gitopsDepl *GitOpsDeployment
	ctx = context.Background()

	BeforeEach(func() {

		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-user",
				UID:  uuid.NewUUID(),
			},
			Spec: corev1.NamespaceSpec{},
		}

		gitopsDepl = &GitOpsDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-gitops-depl",
				Namespace: namespace.Name,
			},
			Spec: GitOpsDeploymentSpec{
				Source: ApplicationSource{},
			},
		}
	})

	Context("Create GitOpsDeployment CR with invalid spec.Type field", func() {
		It("Should fail with error saying spec type must be manual or automated", func() {

			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(BeNil())

			gitopsDepl.Spec.Type = "invalid-type"

			err = k8sClient.Create(ctx, gitopsDepl)
			Expect(err).Should(Not(Succeed()))
			Expect(err.Error()).Should(ContainSubstring(error_invalid_spec_type))

		})

	})
	Context("Create GitOpsDeployment CR with invalid .spec.syncPolicy.syncOptions field", func() {
		It("Should fail with error saying the specified sync option in .spec.syncPolicy.syncOptions is either mispelled or is not supported by GitOpsDeployment", func() {
			gitopsDepl.Spec.Type = GitOpsDeploymentSpecType_Automated
			gitopsDepl.Spec.SyncPolicy = &SyncPolicy{
				SyncOptions: SyncOptions{
					"CreateNamespace=foo",
				},
			}

			err := k8sClient.Create(ctx, gitopsDepl)
			Expect(err).Should(Not(Succeed()))
			Expect(err.Error()).Should(ContainSubstring(error_invalid_sync_option))

		})

	})

	Context("Update GitOpsDeployment CR with invalid .spec.Type field", func() {
		It("Should fail with error saying spec type must be manual or automated", func() {
			gitopsDepl.Spec.Type = GitOpsDeploymentSpecType_Automated
			err := k8sClient.Create(ctx, gitopsDepl)
			Expect(err).Should(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl)
			Expect(err).To(BeNil())

			gitopsDepl.Spec.Type = "invalid-type"
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(error_invalid_spec_type))

			err = k8sClient.Delete(context.Background(), gitopsDepl)
			Expect(err).To(BeNil())
		})
	})

	Context("Update GitOpsDeployment CR with invalid .spec.syncPolicy.syncOptions field", func() {
		It("Should fail with error saying the specified sync option in .spec.syncPolicy.syncOptions is either mispelled or is not supported by GitOpsDeployment", func() {
			gitopsDepl.Spec.Type = GitOpsDeploymentSpecType_Automated
			gitopsDepl.Spec.SyncPolicy = &SyncPolicy{
				SyncOptions: SyncOptions{
					SyncOptions_CreateNamespace_true,
				},
			}
			err := k8sClient.Create(ctx, gitopsDepl)
			Expect(err).Should(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl)
			Expect(err).To(BeNil())

			gitopsDepl.Spec.SyncPolicy.SyncOptions = SyncOptions{
				"CreateNamespace=foo",
			}
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(error_invalid_sync_option))

			err = k8sClient.Delete(context.Background(), gitopsDepl)
			Expect(err).To(BeNil())

		})
	})

	Context("Create GitOpsDeployment CR with empty Environment field and non-empty namespace", func() {
		It("Should fail with error saying the environment field should not be empty when the namespace is non-empty", func() {
			gitopsDepl.Spec.Type = GitOpsDeploymentSpecType_Automated
			gitopsDepl.Spec.Destination.Environment = ""
			gitopsDepl.Spec.Destination.Namespace = "test-namespace"

			err := k8sClient.Create(ctx, gitopsDepl)
			Expect(err).Should(Not(Succeed()))
			Expect(err.Error()).Should(ContainSubstring(error_nonempty_namespace_empty_environment))

		})

	})

	Context("Update GitOpsDeployment CR with empty Environment field and non-empty namespace", func() {
		It("Should fail with error saying the environment field should not be empty when the namespace is non-empty", func() {
			gitopsDepl.Spec.Type = GitOpsDeploymentSpecType_Automated
			err := k8sClient.Create(ctx, gitopsDepl)
			Expect(err).Should(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl)
			Expect(err).To(BeNil())

			gitopsDepl.Spec.Destination.Environment = ""
			gitopsDepl.Spec.Destination.Namespace = "test-namespace"
			err = k8sClient.Update(ctx, gitopsDepl)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring(error_nonempty_namespace_empty_environment))

			err = k8sClient.Delete(context.Background(), gitopsDepl)
			Expect(err).To(BeNil())
		})
	})
})
