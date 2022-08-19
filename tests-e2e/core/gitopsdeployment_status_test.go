package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
)

var _ = Describe("GitOpsDeployment Status Tests", func() {
	Context("Status field of GitOpsDeployment is updated accurately", func() {
		It("GitOpsDeployment .status.resources field is populated with the right resources", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("create a new GitOpsDeployment resource")
			gitOpsDeploymentResource := buildGitOpsDeploymentResource("gitops-depl-test-status",
				"https://github.com/redhat-appstudio/gitops-repository-template", "environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err := k8s.Create(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("ensuring the GitOpsDeployment status field have health, sync and resources fields populated")
			getResourceStatusList := func(name string) []managedgitopsv1alpha1.ResourceStatus {
				return []managedgitopsv1alpha1.ResourceStatus{
					{
						Group:     "apps",
						Version:   "v1",
						Kind:      "Deployment",
						Namespace: fixture.GitOpsServiceE2ENamespace,
						Name:      name,
						Status:    managedgitopsv1alpha1.SyncStatusCodeSynced,
						Health: &managedgitopsv1alpha1.HealthStatus{
							Status: managedgitopsv1alpha1.HeathStatusCodeHealthy,
						},
					},
					{
						Group:     "route.openshift.io",
						Version:   "v1",
						Kind:      "Route",
						Namespace: fixture.GitOpsServiceE2ENamespace,
						Name:      name,
						Status:    managedgitopsv1alpha1.SyncStatusCodeSynced,
						Health: &managedgitopsv1alpha1.HealthStatus{
							Status:  managedgitopsv1alpha1.HeathStatusCodeHealthy,
							Message: "Route is healthy",
						},
					},
					{
						Group:     "",
						Version:   "v1",
						Kind:      "Service",
						Namespace: fixture.GitOpsServiceE2ENamespace,
						Name:      name,
						Status:    managedgitopsv1alpha1.SyncStatusCodeSynced,
						Health: &managedgitopsv1alpha1.HealthStatus{
							Status: managedgitopsv1alpha1.HeathStatusCodeHealthy,
						},
					},
				}
			}
			expectedResourceStatusList := []managedgitopsv1alpha1.ResourceStatus{
				{
					Group:     "",
					Version:   "v1",
					Kind:      "ConfigMap",
					Namespace: fixture.GitOpsServiceE2ENamespace,
					Name:      "environment-config-map",
					Status:    managedgitopsv1alpha1.SyncStatusCodeSynced,
				},
			}
			expectedResourceStatusList = append(expectedResourceStatusList, getResourceStatusList("component-a")...)
			expectedResourceStatusList = append(expectedResourceStatusList, getResourceStatusList("component-b")...)

			Eventually(gitOpsDeploymentResource, "5m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
				),
			)

			By("delete the GitOpsDeployment resource")
			err = k8s.Delete(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())
		})
	})
})
