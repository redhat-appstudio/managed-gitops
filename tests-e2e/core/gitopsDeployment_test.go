package core

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	fixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	typed "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	name    = "my-gitops-depl"
	repoURL = "https://github.com/redhat-appstudio/gitops-repository-template"
)

var _ = Describe("GitOpsDeployment E2E tests", func() {

	Context("Create, Update and Delete a GitOpsDeployment ", func() {
		k8sClient, err := fixture.GetKubeClient()

		Expect(err).To(BeNil())
		ctx := context.Background()

		It("Should ensure succesfull creation of GitOpsDeployment", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeployment")

			gitOpsDeploymentResource := buildGitOpsDeploymentResource(name,
				repoURL, "environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err := k8s.Create(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)
			err = k8s.Delete(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())
			err = k8s.Get(&gitOpsDeploymentResource)
			Expect(err).ToNot(Succeed())

		})

		It("Should ensure synchronicity of create and update of GitOpsDeployment", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("ensurng no updates are done in existing deployment, if CR is submitted again without any changes")

			gitOpsDeploymentResource := buildGitOpsDeploymentResource(name,
				repoURL, "environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err := k8s.Create(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			gitOpsDeploymentResource.Spec.Source.RepoURL = repoURL
			gitOpsDeploymentResource.Spec.Source.Path = "environments/overlays/dev"

			err = k8s.Update(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitOpsDeploymentResource), &gitOpsDeploymentResource)
			Expect(gitOpsDeploymentResource.Spec.Source.RepoURL).To(Equal(repoURL))
			Expect(gitOpsDeploymentResource.Spec.Source.Path).To(Equal("environments/overlays/dev"))

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			err = k8s.Delete(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())
			err = k8s.Get(&gitOpsDeploymentResource)
			Expect(err).ToNot(Succeed())
		})

		It("Should ensure synchronicity of create and update of GitOpsDeployment", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())
			By("ensuring GitOpsDeployment should update successfully on changing value(s) within Spec")

			gitOpsDeploymentResource := buildGitOpsDeploymentResource(name,
				"https://github.com/redhat-appstudio/managed-gitops", "manifests/appstudio-controller-rbac",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err := k8s.Create(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			gitOpsDeploymentResource.Spec.Source.RepoURL = repoURL
			gitOpsDeploymentResource.Spec.Source.Path = "environments/overlays/dev"

			err = k8s.Update(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitOpsDeploymentResource), &gitOpsDeploymentResource)
			Expect(gitOpsDeploymentResource.Spec.Source.RepoURL).To(Equal(repoURL))
			Expect(gitOpsDeploymentResource.Spec.Source.Path).To(Equal("environments/overlays/dev"))

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			err = k8s.Delete(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())
			err = k8s.Get(&gitOpsDeploymentResource)
			Expect(err).ToNot(Succeed())

		})

		It("Checks whether a change in repo URL is reflected within the cluster", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating a new GitOpsDeployment resource")
			gitOpsDeploymentResource := buildGitOpsDeploymentResource("gitops-depl-test-status",
				"https://github.com/redhat-appstudio/managed-gitops", "environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err := k8s.Create(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("ensuring changes in repo URL deploys the resources that are a part of the latest modification")

			gitOpsDeploymentResource.Spec.Source.RepoURL = "https://github.com/redhat-appstudio/gitops-repository-template"

			//updating the CR with changes in repoURL
			err = k8s.Update(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

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

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
				),
			)
			By("delete the GitOpsDeployment resource")
			err = k8s.Delete(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentResource)
			Expect(err).ToNot(Succeed())

			for _, resourceValue := range expectedResourceStatusList {
				ns := typed.NamespacedName{
					Namespace: resourceValue.Namespace,
					Name:      resourceValue.Name,
				}

				err := k8sClient.Get(ctx, ns, &gitOpsDeploymentResource)
				Expect(err).ToNot(Succeed())
			}

		})

		It("Checks whether a change in source path is reflected within the cluster", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating a new GitOpsDeployment resource")
			gitOpsDeploymentResource := buildGitOpsDeploymentResource("gitops-depl-test-status",
				"https://github.com/redhat-appstudio/gitops-repository-template", "environments/base",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err := k8s.Create(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("ensuring changes in path deploys the resources that are a part of the latest modification")

			gitOpsDeploymentResource.Spec.Source.Path = "environments/overlays/dev"

			//updating the CR with changes in path
			err = k8s.Update(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

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

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
				),
			)
			By("delete the GitOpsDeployment resource")

			err = k8s.Delete(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentResource)
			Expect(err).ToNot(Succeed())

			for _, resourceValue := range expectedResourceStatusList {
				ns := typed.NamespacedName{
					Namespace: resourceValue.Namespace,
					Name:      resourceValue.Name,
				}

				err := k8sClient.Get(ctx, ns, &gitOpsDeploymentResource)
				Expect(err).ToNot(Succeed())
			}

		})

		It("Checks whether a change in target revision is reflected within the cluster", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating a new GitOpsDeployment resource")
			gitOpsDeploymentResource := buildTargetRevisionGitOpsDeploymentResource("gitops-depl-test-status",
				"https://github.com/redhat-appstudio/gitops-repository-template", "environments/overlays/dev", "xyz",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err := k8s.Create(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("ensuring changes in target revision deploys the resources that are a part of the latest modification")

			gitOpsDeploymentResource.Spec.Source.TargetRevision = "HEAD"

			//updating the CR with changes in target revision
			err = k8s.Update(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

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

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
				),
			)
			By("delete the GitOpsDeployment resource")

			err = k8s.Delete(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentResource)
			Expect(err).ToNot(Succeed())

			for _, resourceValue := range expectedResourceStatusList {
				ns := typed.NamespacedName{
					Namespace: resourceValue.Namespace,
					Name:      resourceValue.Name,
				}

				err := k8sClient.Get(ctx, ns, &gitOpsDeploymentResource)
				Expect(err).ToNot(Succeed())
			}

		})

		It("Checks whether deleting the GitOpsDeployment deletes all the resources deployed as part of it", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating a new GitOpsDeployment resource")
			gitOpsDeploymentResource := buildGitOpsDeploymentResource("gitops-depl-test-status",
				"https://github.com/redhat-appstudio/gitops-repository-template", "environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err = k8s.Create(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("populating the resource list")

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

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
				),
			)
			By("delete the GitOpsDeployment resource")

			err = k8s.Delete(&gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentResource)
			Expect(err).ToNot(Succeed())

			for _, resourceValue := range expectedResourceStatusList {
				ns := typed.NamespacedName{
					Namespace: resourceValue.Namespace,
					Name:      resourceValue.Name,
				}
				err := k8sClient.Get(ctx, ns, &gitOpsDeploymentResource)
				Expect(err).ToNot(Succeed())
			}

		})

		It("Checks for failure of deployment when an invalid input is provided", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("ensuring GitOpsDeployment Fails to create and deploy application when an invalid field input is passed")
			gitOpsDeploymentResource := buildGitOpsDeploymentResource("",
				"abc", "path/path/path",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err := k8s.Create(&gitOpsDeploymentResource)
			Expect(err).ToNot(Succeed())

			Eventually(gitOpsDeploymentResource, "2m", "1s").ShouldNot(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			err = k8s.Get(&gitOpsDeploymentResource)
			Expect(err).ToNot(Succeed())

		})

	})
})
