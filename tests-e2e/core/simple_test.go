package core

import (
	"context"

	argocdoperator "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	argocdv1 "github.com/redhat-appstudio/managed-gitops/cluster-agent/utils"
	mocks "github.com/redhat-appstudio/managed-gitops/cluster-agent/utils/mocks"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("GitOpsDeployment E2E tests", func() {

	Context("Create a new GitOpsDeployment", func() {

		It("should be healthy and have synced status, and resources should be deployed", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeployment")

			gitOpsDeploymentResource := buildGitOpsDeploymentResource("my-gitops-depl",
				"https://github.com/redhat-appstudio/gitops-repository-template", "environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err := k8s.Create(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("ensuring GitOpsDeployment should have expected health and status")

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)))

			By("ensuring the resources of the GitOps repo are successfully deployed")

			componentADepl := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "component-a", Namespace: fixture.GitOpsServiceE2ENamespace},
			}
			componentBDepl := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "component-b", Namespace: fixture.GitOpsServiceE2ENamespace},
			}
			Eventually(componentADepl, "60s", "1s").Should(k8s.ExistByName())
			Eventually(componentBDepl, "60s", "1s").Should(k8s.ExistByName())

			By("deleting the GitOpsDeployment")

			err = k8s.Delete(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("ensuring the resources of the GitOps repo are successfully deleted")

			Eventually(componentADepl, "60s", "1s").ShouldNot(k8s.ExistByName())
			Eventually(componentBDepl, "60s", "1s").ShouldNot(k8s.ExistByName())

		})
	})
})

// buildGitOpsDeploymentResource builds a GitOpsDeployment with 'opinionated' default values, which is self-contained to
// the GitGitOpsServiceE2ENamespace. This makes it easy to clean up after tests using EnsureCleanSlate.
// - Defaults to creation in GitOpsServiceE2ENamespace
// - Defaults to deployment of K8s resources to GitOpsServiceE2ENamespace
func buildGitOpsDeploymentResource(name, repoURL, path, deploymentSpecType string) managedgitopsv1alpha1.GitOpsDeployment {

	gitOpsDeploymentResource := managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
			Source: managedgitopsv1alpha1.ApplicationSource{
				RepoURL: repoURL,
				Path:    path,
			},
			Destination: managedgitopsv1alpha1.ApplicationDestination{},
			Type:        deploymentSpecType,
		},
	}

	return gitOpsDeploymentResource
}

var _ = Describe("Standalone ArgoCD instance E2E tests", func() {

	Context("Create a Standalone ArgoCD instance", func() {

		It("should create ArgoCD resource and application, wait for it to be installed and synced", func() {
			ctx := context.Background()
			var k8sClient client.Client

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating ArgoCD resource")

			argoCDResource := argocdoperator.ArgoCD{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd",
					Namespace: "my-argo-cd",
				},
			}
			err := argocdv1.CreateNamespaceScopedArgoCD(ctx, argoCDResource.Name, argoCDResource.Namespace, k8sClient)
			Expect(err).To(Succeed())

			By("ensuring ArgoCD resource exists")
			argoCDInstance := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: argoCDResource.Name, Namespace: argoCDResource.Namespace},
			}
			Eventually(argoCDInstance, "60s", "1s").Should(k8s.ExistByName())

			err = argocdv1.SetupArgoCD(k8sClient)
			Expect(err).To(Succeed())

			By("creating ArgoCD application")
			app := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "",
					Namespace: "gitops-service-argocd",
				},
				Spec: appv1.ApplicationSpec{
					Source: appv1.ApplicationSource{
						RepoURL: "https://github.com/redhat-appstudio/gitops-repository-template",
						Path:    "environments/overlays/dev",
						// TargetRevision: "",
					},
					Destination: appv1.ApplicationDestination{
						Name:      "in-cluster",
						Namespace: argoCDResource.Namespace,
					},
				},
			}
			err = k8s.Create(&app)
			Expect(err).To(Succeed())

			//debug appsync

			mockAppClient := &mocks.Client{}
			clientGenerator := argocdv1.MockClientGenerator{
				mockClient: mockAppClient,
			}

			cs := argocdv1.NewCredentialService(&clientGenerator, true)
			err = argocdv1.AppSync(context.Background(), app.Name, "master", app.Namespace, k8sClient, cs, true)

			Eventually(app, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)))

		})
	})
})
