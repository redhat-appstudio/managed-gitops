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

// var _ = Describe("GitOpsDeploymentManagedEnvironment validation webhook", func() {
// 	var managedEnv *GitOpsDeploymentManagedEnvironment
// 	var ctx context.Context

// 	BeforeEach(func() {

// 		ctx = context.Background()

// 		managedEnv = &GitOpsDeploymentManagedEnvironment{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Name:      "my-managed-env",
// 				Namespace: Namespace,
// 				UID:       uuid.NewUUID(),
// 			},
// 			Spec: GitOpsDeploymentManagedEnvironmentSpec{
// 				ClusterCredentialsSecret: "fake-secret-name",
// 			},
// 		}

// 	})

// 	Context("Create GitOpsDeploymentManagedEnvironment CR with invalid API URL", func() {
// 		It("Should fail with error saying cluster api url must start with https://", func() {
// 			managedEnv.Spec.APIURL = "smtp://api-url"
// 			err := k8sClient.Create(ctx, managedEnv)

// 			Expect(err).Should(Not(Succeed()))
// 			Expect(err.Error()).Should(ContainSubstring("cluster api url must start with https://"))
// 		})
// 	})

// 	Context("Update GitOpsDeploymentManagedEnvironment CR with invalid invalid API URL", func() {
// 		It("Should fail with error saying cluster api url must start with https://", func() {

// 			managedEnv.Spec.APIURL = "https://api.fake-unit-test-data.origin-ci-int-gce.dev.rhcloud.com:6443"

// 			err := k8sClient.Create(ctx, managedEnv)
// 			Expect(err).Should(Succeed())

// 			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(managedEnv), managedEnv)
// 			Expect(err).To(Succeed())

// 			managedEnv.Spec.APIURL = "smtp://api-url"
// 			err = k8sClient.Update(ctx, managedEnv)
// 			Expect(err).Should(HaveOccurred())
// 			Expect(err.Error()).Should(ContainSubstring("cluster api url must start with https://"))

// 			err = k8sClient.Delete(context.Background(), managedEnv)
// 			Expect(err).To(BeNil())

// 		})
// 	})

// })
