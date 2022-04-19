package core

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"

	// matcher "github.com/onsi/gomega/types"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	// corev1 "k8s.io/api/core/v1"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("GitOpsDeployment E2E tests", func() {

	Context("Create a new GitOpDeployment", func() {

		It("should be healthy and have synced status, and resources should be deployed", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("Create the GitOpsDeployment")

			gitOpsDeploymentResource := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						RepoURL: "https://github.com/redhat-appstudio/gitops-repository-template",
						Path:    "environments/overlays/dev",
					},
					Destination: managedgitopsv1alpha1.ApplicationDestination{},
					Type:        managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			err := k8s.Create(gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("GitOpsDeployment should have expected health and status")

			Eventually(*gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode("Healthy"))) // TODO: Switch this to the constant once PR #101 merges.

			By("The resources of the GitOps repo should be successfully deployed")

			componentADepl := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "component-a", Namespace: fixture.GitOpsServiceE2ENamespace},
			}
			componentBDepl := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "component-b", Namespace: fixture.GitOpsServiceE2ENamespace},
			}
			Eventually(componentADepl, "60s", "1s").Should(k8s.ExistByName())
			Eventually(componentBDepl, "60s", "1s").Should(k8s.ExistByName())

			By("Delete the GitOpsDeployment")

			err = k8s.Delete(gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("The resources of the GitOps repo should be successfully deleted")

			Eventually(componentADepl, "60s", "1s").ShouldNot(k8s.ExistByName())
			Eventually(componentBDepl, "60s", "1s").ShouldNot(k8s.ExistByName())

		})
	})
})
