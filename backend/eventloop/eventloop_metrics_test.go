package eventloop

// import (
// 	. "github.com/onsi/ginkgo/v2"
// 	. "github.com/onsi/gomega"
// 	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
// 	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// )

// var _ = Describe("Gitopsdeployment metrics counter Test", func() {

// 	Context("Prometheus metrics responds to count of active/failed GitopsDeployments", func() {

// 		It("Should alter metrics counter whenever GitopsDeployment is created/deleted/failed", func() {
// 			Expect(fixture.EnsureCleanSlate()).To(Succeed())
// 			k8sClient, err := fixture.GetKubeClient()
// 			if err != nil {
// 				return err
// 			}
// 			metrics_counter_active := Gitopsdepl
// 			metrics_counter_fail := GitopsdeplFailures

// 			gitOpsDeploymentResource := managedgitopsv1alpha1.GitOpsDeployment{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "deployment-name",
// 					Namespace: fixture.GitOpsServiceE2ENamespace,
// 				},
// 				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
// 					Source: managedgitopsv1alpha1.ApplicationSource{
// 						RepoURL: "https://github.com/redhat-appstudio/gitops-repository-template",
// 						Path:    "environments/overlays/dev",
// 					},
// 					Destination: managedgitopsv1alpha1.ApplicationDestination{},
// 					Type:        managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
// 				},
// 			}
// 			err = fixture.Create(&gitOpsDeploymentResource)
// 			Expect(err).To(Succeed())

// 			Expect(Gitopsdepl).To(Equal(metrics_counter_active + 1))

// 			metrics_counter_active = Gitopsdepl

// 			err = fixture.Delete(&gitOpsDeploymentResource)
// 			Expect(err).To(Succeed())
// 			Expect(Gitopsdepl).To(Equal(metrics_counter_active - 1))

// 			gitOpsDeploymentResource = managedgitopsv1alpha1.GitOpsDeployment{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "deployment-name",
// 					Namespace: fixture.GitOpsServiceE2ENamespace,
// 				},
// 				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
// 					Source: managedgitopsv1alpha1.ApplicationSource{
// 						RepoURL: "https://github.com/redhat-appstudio/gitops-repository-template",
// 						Path:    "invalid/path",
// 					},
// 					Destination: managedgitopsv1alpha1.ApplicationDestination{},
// 					Type:        managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
// 				},
// 			}
// 			err = fixture.Create(&gitOpsDeploymentResource)
// 			Expect(err).ToNot(Succeed())

// 			Expect(GitopsdeplFailures).To(Equal(metrics_counter_fail + 1))

// 		})

// 	})
// })
