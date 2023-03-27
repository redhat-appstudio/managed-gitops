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

// var _ = Describe("GitOpsDeploymentSyncRun validation webhook", func() {
// 	var gitopsDeplSyncRunCr *GitOpsDeploymentSyncRun
// 	var ctx context.Context

// 	BeforeEach(func() {

// 		ctx = context.Background()

// 		gitopsDeplSyncRunCr = &GitOpsDeploymentSyncRun{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Name:      "test-gitopsdeployment-syncrun",
// 				Namespace: Namespace,
// 				UID:       uuid.NewUUID(),
// 			},
// 			Spec: GitOpsDeploymentSyncRunSpec{
// 				GitopsDeploymentName: "test-app",
// 				RevisionID:           "HEAD",
// 			},
// 		}

// 	})

// 	Context("Create GitOpsDeploymentSyncRun CR with invalid name", func() {
// 		It("Should fail with error saying name should not be zyxwvutsrqponmlkjihgfedcba-abcdefghijklmnoqrstuvwxyz", func() {
// 			gitopsDeplSyncRunCr.Name = "zyxwvutsrqponmlkjihgfedcba-abcdefghijklmnoqrstuvwxyz"
// 			err := k8sClient.Create(ctx, gitopsDeplSyncRunCr)

// 			Expect(err).Should(Not(Succeed()))
// 			Expect(err.Error()).Should(ContainSubstring("name should not be zyxwvutsrqponmlkjihgfedcba-abcdefghijklmnoqrstuvwxyz"))

// 		})
// 	})

// 	Context("Update GitOpsDeploymentSyncRun CR with invalid name", func() {
// 		It("Should fail with error saying name should not be zyxwvutsrqponmlkjihgfedcba-abcdefghijklmnoqrstuvwxyz", func() {

// 			gitopsDeplSyncRunCr.Name = "test-gitopsdeployment-syncrun"

// 			err := k8sClient.Create(ctx, gitopsDeplSyncRunCr)
// 			Expect(err).Should(Succeed())

// 			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDeplSyncRunCr), gitopsDeplSyncRunCr)
// 			Expect(err).To(Succeed())

// 			gitopsDeplSyncRunCr.Name = "zyxwvutsrqponmlkjihgfedcba-abcdefghijklmnoqrstuvwxyz"
// 			err = k8sClient.Update(ctx, gitopsDeplSyncRunCr)
// 			Expect(err).Should(HaveOccurred())
// 			Expect(err.Error()).Should(ContainSubstring("name should not be zyxwvutsrqponmlkjihgfedcba-abcdefghijklmnoqrstuvwxyz"))

// 			err = k8sClient.Delete(context.Background(), gitopsDeplSyncRunCr)
// 			Expect(err).To(BeNil())
// 		})
// 	})

// })
