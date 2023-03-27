package v1alpha1

// import (
// 	"context"

// 	. "github.com/onsi/ginkgo/v2"
// 	. "github.com/onsi/gomega"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/apimachinery/pkg/util/uuid"
// 	"sigs.k8s.io/controller-runtime/pkg/client"
// 	//+kubebuilder:scaffold:imports
// )

// var _ = Describe("GitOpsDeploymentRepositoryCredential validation webhook", func() {
// 	var repoCredentialCr *GitOpsDeploymentRepositoryCredential
// 	var ctx context.Context

// 	BeforeEach(func() {

// 		ctx = context.Background()

// 		repoCredentialCr = &GitOpsDeploymentRepositoryCredential{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Name:      "test-repo",
// 				Namespace: Namespace,
// 				UID:       uuid.NewUUID(),
// 			},
// 			Spec: GitOpsDeploymentRepositoryCredentialSpec{
// 				Repository: "https://test-private-url",
// 				Secret:     "test-secret",
// 			},
// 		}

// 	})

// 	Context("Create GitOpsDeploymentRepositoryCredential CR with invalid Repository URL", func() {
// 		It("Should fail with error saying repository must begin with ssh:// or https://", func() {
// 			repoCredentialCr.Spec.Repository = "smtp://repo-url"
// 			err := k8sClient.Create(ctx, repoCredentialCr)

// 			Expect(err).Should(Not(Succeed()))
// 			Expect(err.Error()).Should(ContainSubstring("repository must begin with ssh:// or https://"))

// 		})
// 	})

// 	Context("Update GitOpsDeploymentManagedEnvironment CR with invalid Repository API URL", func() {
// 		It("Should fail with error saying repository must begin with ssh:// or https://", func() {

// 			repoCredentialCr.Spec.Repository = "https://test-private-url"

// 			err := k8sClient.Create(ctx, repoCredentialCr)
// 			Expect(err).Should(Succeed())

// 			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(repoCredentialCr), repoCredentialCr)
// 			Expect(err).To(Succeed())

// 			repoCredentialCr.Spec.Repository = "smtp://repo-url"
// 			err = k8sClient.Update(ctx, repoCredentialCr)
// 			Expect(err).Should(HaveOccurred())
// 			Expect(err.Error()).Should(ContainSubstring("repository must begin with ssh:// or https://"))

// 			err = k8sClient.Delete(context.Background(), repoCredentialCr)
// 			Expect(err).To(BeNil())

// 		})
// 	})

// })
