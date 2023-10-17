package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
)

var _ = Describe("GitOpsDeployment Condition Tests", func() {

	Context("Errors are set properly in Status.Conditions field of GitOpsDeployment", func() {

		It("ensures that GitOpsDeployment .status.condition field is populated with the corrrect error when an illegal GitOpsDeployment is created", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("create an invalid GitOpsDeployment application")
			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource("managed-environment-gitops-depl",
				fixture.RepoURL, fixture.GitopsDeploymentPath,
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			gitOpsDeploymentResource.Spec.Destination = managedgitopsv1alpha1.ApplicationDestination{
				Environment: "does-not-exist", // This is the invalid value, which should return a condition error
				Namespace:   "",
			}

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			Expect(k8s.Create(&gitOpsDeploymentResource, k8sClient)).To(Succeed())

			expectedConditions := []managedgitopsv1alpha1.GitOpsDeploymentCondition{
				{
					Type:    managedgitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred,
					Message: "Unable to reconcile the ManagedEnvironment. Verify that the ManagedEnvironment and Secret are correctly defined, and have valid credentials",
					Status:  managedgitopsv1alpha1.GitOpsConditionStatusTrue,
					Reason:  managedgitopsv1alpha1.GitopsDeploymentReasonErrorOccurred,
				},
			}

			Eventually(gitOpsDeploymentResource, "5m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveConditions(expectedConditions),
				),
			)

			By("delete the GitOpsDeployment resource")
			Expect(k8s.Delete(&gitOpsDeploymentResource, k8sClient)).To(Succeed())
		})
	})
})
