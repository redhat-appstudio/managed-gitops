package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	fixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typed "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	repoURL = "https://github.com/redhat-appstudio/managed-gitops"

	// ArgoCDReconcileWaitTime is the length of time to watch for Argo CD/GitOps Service to deploy the resources
	// of an Application (e.g. to reconcile the Application resource)
	// We set this to be at least twice the default reconciliation time (3 minutes) to avoid a race condition in updating ConfigMaps
	// in Argo CD. This is an imperfect solution.
	ArgoCDReconcileWaitTime = "7m"
)

var _ = Describe("GitOpsDeployment E2E tests", func() {

	const (
		name = "my-gitops-depl"
	)

	Context("Create, Update and Delete a GitOpsDeployment ", func() {

		var config *rest.Config
		var k8sClient client.Client
		ctx := context.Background()

		// this assumes that service is running on non aware kcp client
		BeforeEach(func() {
			var err error
			config, err = fixture.GetSystemKubeConfig()
			Expect(err).To(BeNil())

			k8sClient, err = fixture.GetKubeClient(config)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			// At end of a test, output the state of Argo CD Application, for post-mortem debuggign
			err := fixture.ReportRemainingArgoCDApplications(k8sClient)
			Expect(err).To(BeNil())
		})

		// Generate expected resources for "https://github.com/redhat-appstudio/managed-gitops"
		getResourceStatusList_GitOpsRepositoryTemplateRepo := func(name string) []managedgitopsv1alpha1.ResourceStatus {
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

		// Generate  expected resource from "https://github.com/managed-gitops-test-data/deployment-permutations-*" using name
		getResourceStatusList_deploymentPermutations := func(name string) []managedgitopsv1alpha1.ResourceStatus {
			return []managedgitopsv1alpha1.ResourceStatus{
				{
					Group:     "",
					Version:   "v1",
					Kind:      "ConfigMap",
					Name:      name,
					Namespace: fixture.GitOpsServiceE2ENamespace,
					Status:    managedgitopsv1alpha1.SyncStatusCodeSynced,
				},
			}
		}

		// Generate expected identifier configmap for "https://github.com/managed-gitops-test-data/deployment-permutations-*"
		createResourceStatusListFunction_deploymentPermutations := func() []managedgitopsv1alpha1.ResourceStatus {
			return []managedgitopsv1alpha1.ResourceStatus{
				{
					Group:     "",
					Version:   "v1",
					Kind:      "ConfigMap",
					Name:      "identifier",
					Namespace: fixture.GitOpsServiceE2ENamespace,
					Status:    managedgitopsv1alpha1.SyncStatusCodeSynced,
				},
			}
		}

		It("Should ensure succesful creation of GitOpsDeployment, by creating the GitOpsDeployment", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())
			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource(name,
				repoURL, "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			gitOpsDeploymentResource.Spec.Destination.Environment = ""
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)
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
			expectedResourceStatusList = append(expectedResourceStatusList, getResourceStatusList_GitOpsRepositoryTemplateRepo("component-a")...)
			expectedResourceStatusList = append(expectedResourceStatusList, getResourceStatusList_GitOpsRepositoryTemplateRepo("component-b")...)

			expectedReconciledStateField := managedgitopsv1alpha1.ReconciledState{
				Source: managedgitopsv1alpha1.GitOpsDeploymentSource{
					Path:    gitOpsDeploymentResource.Spec.Source.Path,
					RepoURL: gitOpsDeploymentResource.Spec.Source.RepoURL,
				},
				Destination: managedgitopsv1alpha1.GitOpsDeploymentDestination{
					Name:      gitOpsDeploymentResource.Spec.Destination.Environment,
					Namespace: gitOpsDeploymentResource.Spec.Destination.Namespace,
				},
			}

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
					gitopsDeplFixture.HaveReconciledState(expectedReconciledStateField),
				),
			)

			By("deleting the GitOpsDeployment resource and waiting for the resources to be deleted")

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			expectAllResourcesToBeDeleted(expectedResourceStatusList)

		})

		It("Should ensure synchronicity of create and update of GitOpsDeployment, by ensurng no updates are done in existing deployment, if CR is submitted again without any changes", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource(name,
				repoURL, "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			gitOpsDeploymentResource.Spec.Destination.Environment = ""
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitOpsDeploymentResource), &gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			gitOpsDeploymentResource.Spec.Source.RepoURL = repoURL
			gitOpsDeploymentResource.Spec.Source.Path = "resources/test-data/sample-gitops-repository/environments/overlays/dev"

			err = k8s.Update(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitOpsDeploymentResource), &gitOpsDeploymentResource)
			Expect(err).To(BeNil())
			Expect(gitOpsDeploymentResource.Spec.Source.RepoURL).To(Equal(repoURL))
			Expect(gitOpsDeploymentResource.Spec.Source.Path).To(Equal("resources/test-data/sample-gitops-repository/environments/overlays/dev"))

			expectedReconciledStateField := managedgitopsv1alpha1.ReconciledState{
				Source: managedgitopsv1alpha1.GitOpsDeploymentSource{
					RepoURL: gitOpsDeploymentResource.Spec.Source.RepoURL,
					Path:    gitOpsDeploymentResource.Spec.Source.Path,
				},
				Destination: managedgitopsv1alpha1.GitOpsDeploymentDestination{
					Name:      gitOpsDeploymentResource.Spec.Destination.Environment,
					Namespace: gitOpsDeploymentResource.Spec.Destination.Namespace,
				},
			}

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveReconciledState(expectedReconciledStateField),
				),
			)
			Consistently(gitOpsDeploymentResource, "20s", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveReconciledState(expectedReconciledStateField),
				),
			)

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())
		})

		It("Should ensure synchronicity of create and update of GitOpsDeployment, by ensuring GitOpsDeployment should update successfully on changing value(s) within Spec", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())
			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource(name,
				repoURL, "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitOpsDeploymentResource), &gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			gitOpsDeploymentResource.Spec.Source.Path = "resources/test-data/sample-gitops-repository/environments/overlays/staging"

			err = k8s.Update(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitOpsDeploymentResource), &gitOpsDeploymentResource)
			Expect(err).To(BeNil())
			Expect(gitOpsDeploymentResource.Spec.Source.Path).To(Equal("resources/test-data/sample-gitops-repository/environments/overlays/staging"))

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())
		})

		It("Checks whether a change in repo URL is reflected within the cluster", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("creating a new GitOpsDeployment resource")
			gitOpsDeploymentResource := gitopsDeplFixture.BuildTargetRevisionGitOpsDeploymentResource("gitops-depl-test",
				"https://github.com/managed-gitops-test-data/deployment-permutations-a", "pathB", "branchA",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			createResourceStatusList_deploymentPermutations := createResourceStatusListFunction_deploymentPermutations()

			createResourceStatusList_deploymentPermutations = append(createResourceStatusList_deploymentPermutations,
				getResourceStatusList_deploymentPermutations("deployment-permutations-a-brancha-pathb")...)
			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(createResourceStatusList_deploymentPermutations),
				),
			)

			By("ensuring changes in repo URL deploys the resources that are a part of the latest modification")

			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			gitOpsDeploymentResource.Spec.Source.RepoURL = "https://github.com/managed-gitops-test-data/deployment-permutations-b"

			//updating the CR with changes in repoURL
			err = k8s.Update(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			expectedResourceStatusList := createResourceStatusListFunction_deploymentPermutations()
			expectedResourceStatusList = append(expectedResourceStatusList,
				getResourceStatusList_deploymentPermutations("deployment-permutations-b-brancha-pathb")...)

			// There is an Argo CD race condition where the ConfigMaps are updated before Argo CD can detect the event, which causes
			// the Argo CD sync to hang. Thus we need to use this large '10m' wait time here - jgwest, Jul 2022.
			Eventually(gitOpsDeploymentResource, "10m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
				),
			)
			By("delete the GitOpsDeployment resource")
			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).ToNot(Succeed())

			expectAllResourcesToBeDeleted(expectedResourceStatusList)

		})

		It("Checks whether a change in source path is reflected within the cluster", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("creating a new GitOpsDeployment resource")
			gitOpsDeploymentResource := gitopsDeplFixture.BuildTargetRevisionGitOpsDeploymentResource("gitops-depl-test",
				"https://github.com/managed-gitops-test-data/deployment-permutations-a", "pathB", "branchA",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			gitOpsDeploymentResource.Spec.Destination.Environment = ""
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			createResourceStatusList := createResourceStatusListFunction_deploymentPermutations()
			createResourceStatusList = append(createResourceStatusList,
				getResourceStatusList_deploymentPermutations("deployment-permutations-a-brancha-pathb")...)
			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			expectedReconciledStateField := managedgitopsv1alpha1.ReconciledState{
				Source: managedgitopsv1alpha1.GitOpsDeploymentSource{
					Path:    gitOpsDeploymentResource.Spec.Source.Path,
					RepoURL: gitOpsDeploymentResource.Spec.Source.RepoURL,
					Branch:  gitOpsDeploymentResource.Spec.Source.TargetRevision,
				},
				Destination: managedgitopsv1alpha1.GitOpsDeploymentDestination{
					Name:      gitOpsDeploymentResource.Spec.Destination.Environment,
					Namespace: gitOpsDeploymentResource.Spec.Destination.Namespace,
				},
			}

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(createResourceStatusList),
					gitopsDeplFixture.HaveReconciledState(expectedReconciledStateField),
				),
			)

			By("ensuring changes in repo URL deploys the resources that are a part of the latest modification")

			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			gitOpsDeploymentResource.Spec.Source.Path = "pathC"

			//updating the CR with changes in repoURL
			err = k8s.Update(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			expectedResourceStatusList := createResourceStatusListFunction_deploymentPermutations()
			expectedResourceStatusList = append(expectedResourceStatusList,
				getResourceStatusList_deploymentPermutations("deployment-permutations-a-brancha-pathc")...)

			expectedReconciledStateField = managedgitopsv1alpha1.ReconciledState{
				Source: managedgitopsv1alpha1.GitOpsDeploymentSource{
					Path:    gitOpsDeploymentResource.Spec.Source.Path,
					RepoURL: gitOpsDeploymentResource.Spec.Source.RepoURL,
					Branch:  gitOpsDeploymentResource.Spec.Source.TargetRevision,
				},
				Destination: managedgitopsv1alpha1.GitOpsDeploymentDestination{
					Name:      gitOpsDeploymentResource.Spec.Destination.Environment,
					Namespace: gitOpsDeploymentResource.Spec.Destination.Namespace,
				},
			}

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
					gitopsDeplFixture.HaveReconciledState(expectedReconciledStateField),
				),
			)
			By("delete the GitOpsDeployment resource")

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).ToNot(Succeed())

			expectAllResourcesToBeDeleted(expectedResourceStatusList)

		})

		It("Checks whether a change in target revision is reflected within the cluster", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating a new GitOpsDeployment resource")
			gitOpsDeploymentResource := gitopsDeplFixture.BuildTargetRevisionGitOpsDeploymentResource("gitops-depl-test",
				"https://github.com/managed-gitops-test-data/deployment-permutations-a", "pathB", "branchA",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			createResourceStatusList := createResourceStatusListFunction_deploymentPermutations()
			createResourceStatusList = append(createResourceStatusList, getResourceStatusList_deploymentPermutations("deployment-permutations-a-brancha-pathb")...)
			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(createResourceStatusList),
				),
			)

			By("ensuring changes in repo URL deploys the resources that are a part of the latest modification")

			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			gitOpsDeploymentResource.Spec.Source.TargetRevision = "branchB"

			//updating the CR with changes in repoURL
			err = k8s.Update(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			expectedResourceStatusList := createResourceStatusListFunction_deploymentPermutations()
			expectedResourceStatusList = append(expectedResourceStatusList,
				getResourceStatusList_deploymentPermutations("deployment-permutations-a-branchb-pathb")...)

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
				),
			)
			By("deleting the GitOpsDeployment resource")

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).ToNot(Succeed())

			expectAllResourcesToBeDeleted(expectedResourceStatusList)

		})

		It("Checks whether resources are created in target revision and reflected within the cluster", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating a new GitOpsDeployment resource")
			gitOpsDeploymentResource := gitopsDeplFixture.BuildTargetRevisionGitOpsDeploymentResource("gitops-depl-test-status",
				"https://github.com/redhat-appstudio/managed-gitops", "resources/test-data/sample-gitops-repository/environments/overlays/dev", "xyz",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			By("ensuring changes in target revision deploys the resources that are a part of the latest modification")

			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())
			gitOpsDeploymentResource.Spec.Source.TargetRevision = "HEAD"

			//updating the CR with changes in target revision
			err = k8s.Update(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			expectedResourceStatusList_GitOpsRepositoryTemplateRepo := []managedgitopsv1alpha1.ResourceStatus{
				{
					Group:     "",
					Version:   "v1",
					Kind:      "ConfigMap",
					Namespace: fixture.GitOpsServiceE2ENamespace,
					Name:      "environment-config-map",
					Status:    managedgitopsv1alpha1.SyncStatusCodeSynced,
				},
			}
			expectedResourceStatusList_GitOpsRepositoryTemplateRepo = append(expectedResourceStatusList_GitOpsRepositoryTemplateRepo, getResourceStatusList_GitOpsRepositoryTemplateRepo("component-a")...)
			expectedResourceStatusList_GitOpsRepositoryTemplateRepo = append(expectedResourceStatusList_GitOpsRepositoryTemplateRepo, getResourceStatusList_GitOpsRepositoryTemplateRepo("component-b")...)

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList_GitOpsRepositoryTemplateRepo),
				),
			)
			By("delete the GitOpsDeployment resource")

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).ToNot(Succeed())

			expectAllResourcesToBeDeleted(expectedResourceStatusList_GitOpsRepositoryTemplateRepo)

		})

		It("Checks whether deleting the GitOpsDeployment deletes all the resources deployed as part of it", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating a new GitOpsDeployment resource")
			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource("gitops-depl-test-status",
				"https://github.com/redhat-appstudio/managed-gitops", "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			By("populating the resource list")

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
			expectedResourceStatusList = append(expectedResourceStatusList, getResourceStatusList_GitOpsRepositoryTemplateRepo("component-a")...)
			expectedResourceStatusList = append(expectedResourceStatusList, getResourceStatusList_GitOpsRepositoryTemplateRepo("component-b")...)

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
				),
			)
			By("delete the GitOpsDeployment resource")

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).ToNot(Succeed())

			expectAllResourcesToBeDeleted(expectedResourceStatusList)
		})

		It("Checks whether the GitOpsDeployment can be deleted in the presence of a finalizer", func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating a new GitOpsDeployment resource with the deletion finalizer")
			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource("gitops-depl-test-status",
				"https://github.com/redhat-appstudio/managed-gitops", "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			gitOpsDeploymentResource.Finalizers = append(gitOpsDeploymentResource.Finalizers, managedgitopsv1alpha1.DeletionFinalizer)

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			By("populating the resource list")

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
			expectedResourceStatusList = append(expectedResourceStatusList, getResourceStatusList_GitOpsRepositoryTemplateRepo("component-a")...)
			expectedResourceStatusList = append(expectedResourceStatusList, getResourceStatusList_GitOpsRepositoryTemplateRepo("component-b")...)

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveResources(expectedResourceStatusList),
				),
			)

			// We add a finalizer to one of the resources to prevent a race condition where the GitOpsDeployment and
			// all the dependent resources might get deleted before the test can check if the finalizer has prevented the deletion.
			testFinalizer := "kubernetes"
			resStatus := expectedResourceStatusList[0]

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resStatus.Name,
					Namespace: resStatus.Namespace,
				},
			}

			err = k8s.UpdateWithoutConflict(cm, k8sClient, func(o client.Object) {
				o.SetFinalizers([]string{testFinalizer})
			})
			Expect(err).To(BeNil())

			By("delete the GitOpsDeployment resource")

			err = k8sClient.Delete(ctx, &gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			By("verify if the finalizer has prevented the GitOpsDeployment from deletion")
			err = k8s.Get(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(BeNil())

			Eventually(gitOpsDeploymentResource, "60s", "1s").Should(k8s.HasNonNilDeletionTimestamp(k8sClient))

			// check if the GitOps Service has not removed the finalizer before the dependent resources are deleted.
			Consistently(&gitOpsDeploymentResource, "30s", "1s").Should(k8s.ExistByName(k8sClient))

			// Remove the test finalizer and verify if all the dependent resources are deleted
			err = k8s.UpdateWithoutConflict(cm, k8sClient, func(o client.Object) {
				o.SetFinalizers([]string{})
			})
			Expect(err).To(BeNil())

			expectAllResourcesToBeDeleted(expectedResourceStatusList)

			By("verify if the GitOpsDeployment is deleted after all the dependencies are removed")
			Eventually(&gitOpsDeploymentResource, "30s", "1s").Should(k8s.NotExist(k8sClient))
		})

		It("Checks for failure of deployment when an invalid input is provided", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("ensuring GitOpsDeployment Fails to create and deploy application when an invalid field input is passed")
			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource("test-should-fail",
				"invalid-url", "path/path/path",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").ShouldNot(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)
			Consistently(gitOpsDeploymentResource, "20s", "1s").ShouldNot(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)
		})

		It("Create and delete the same GitOpsDeployment 3 times in a row, and make sure each works", func() {

			// The goal of this test is to make sure the GitOpsService properly handles the deletion and
			// recreation of GitOpsDeployments with the same name/namespace/
			// - An example of a bug that would be caught by this test: what if the application event loop
			//   runner failed to cleanup up from an old GitOpsDeployment, and thus the event loop failed
			//   to process events for new GitOpsDeployments with the same name, in that namesapace.

			for count := 0; count < 3; count++ {

				Expect(fixture.EnsureCleanSlate()).To(Succeed())
				gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource(name,
					repoURL, "resources/test-data/sample-gitops-repository/environments/overlays/dev",
					managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
				gitOpsDeploymentResource.Spec.Destination.Environment = ""
				gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

				k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
				Expect(err).To(Succeed())

				err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
				Expect(err).To(Succeed())

				Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
					SatisfyAll(
						gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
						gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					),
				)
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
				expectedResourceStatusList = append(expectedResourceStatusList, getResourceStatusList_GitOpsRepositoryTemplateRepo("component-a")...)
				expectedResourceStatusList = append(expectedResourceStatusList, getResourceStatusList_GitOpsRepositoryTemplateRepo("component-b")...)

				Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
					SatisfyAll(
						gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
						gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
						gitopsDeplFixture.HaveResources(expectedResourceStatusList),
						// gitopsDeplFixture.HaveReconciledState(expectedReconciledStateField),
					),
				)

				By("deleting the GitOpsDeployment resource and waiting for the resources to be deleted")

				err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
				Expect(err).To(Succeed())

				expectAllResourcesToBeDeleted(expectedResourceStatusList)

			}

		})

	})

	Context("Simulates simple interactions of X active users interacting with the service", func() {

		// Number of users interacting with GitOps service.
		const (
			numberToSimulate = 10
		)

		var k8sClient client.Client

		BeforeEach(func() {
			var config *rest.Config

			var err error
			config, err = fixture.GetSystemKubeConfig()
			Expect(err).To(BeNil())

			k8sClient, err = fixture.GetKubeClient(config)
			Expect(err).To(BeNil())

			// Ensure that all user-* namespaces are deleted at start of tests
			for i := 1; i <= numberToSimulate; i++ {

				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("user-%d", i),
						Labels: map[string]string{
							"argocd.argoproj.io/managed-by": "gitops-service-argocd",
						},
					},
				}

				if err = k8sClient.Delete(context.Background(), namespace); err != nil {
					Expect(apierr.IsNotFound(err)).To(BeTrue())
				}

				Eventually(namespace, "2m", "1s").ShouldNot(k8s.ExistByName(k8sClient))
			}

		})

		AfterEach(func() {
			// At end of a test, output the state of Argo CD Application, for post-mortem debuggign
			err := fixture.ReportRemainingArgoCDApplications(k8sClient)
			Expect(err).To(BeNil())
		})

		config, err := fixture.GetSystemKubeConfig()
		Expect(err).To(BeNil())

		// testPrintln outputs a log statement along with the particular test #
		testPrintln := func(str string, i int) {
			fmt.Printf("%v - %d) %s\n", time.Now(), i, str)
		}

		// runTestInner returns error if the test fail, nil otherwise.
		runTestInner := func(i int) error {

			testPrintln("Create new namespace", i)
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("user-%d", i),
					Labels: map[string]string{
						"argocd.argoproj.io/managed-by": "gitops-service-argocd",
					},
				},
			}

			if err := k8s.Create(namespace, k8sClient); err != nil {
				return err
			}

			// Generate  expected resource from "https://github.com/managed-gitops-test-data/deployment-permutations-*" using name
			getResourceStatusList_deploymentPermutations := func(name string) []managedgitopsv1alpha1.ResourceStatus {
				return []managedgitopsv1alpha1.ResourceStatus{
					{
						Group:     "",
						Version:   "v1",
						Kind:      "ConfigMap",
						Name:      name,
						Namespace: namespace.ObjectMeta.Name,
						Status:    managedgitopsv1alpha1.SyncStatusCodeSynced,
					},
				}
			}

			// Generate expected identifier configmap for "https://github.com/managed-gitops-test-data/deployment-permutations-*"
			createResourceStatusListFunction_deploymentPermutations := func() []managedgitopsv1alpha1.ResourceStatus {
				return []managedgitopsv1alpha1.ResourceStatus{
					{
						Group:     "",
						Version:   "v1",
						Kind:      "ConfigMap",
						Name:      "identifier",
						Namespace: namespace.ObjectMeta.Name,
						Status:    managedgitopsv1alpha1.SyncStatusCodeSynced,
					},
				}
			}

			testPrintln("Create GitopsDeploymentResource", i)
			gitOpsDeploymentResource := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("my-gitops-depl-%d", i),
					Namespace: namespace.ObjectMeta.Name,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/managed-gitops-test-data/deployment-permutations-a",
						Path:           "pathB",
						TargetRevision: "branchA",
					},
					Destination: managedgitopsv1alpha1.ApplicationDestination{},
					Type:        managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			if err := k8s.Create(gitOpsDeploymentResource, k8sClient); err != nil {
				return err
			}

			testPrintln("Make sure it deploys the target resources", i)
			createResourceStatusList_deploymentPermutations := createResourceStatusListFunction_deploymentPermutations()

			createResourceStatusList_deploymentPermutations = append(createResourceStatusList_deploymentPermutations,
				getResourceStatusList_deploymentPermutations("deployment-permutations-a-brancha-pathb")...)
			if err := k8s.Get(gitOpsDeploymentResource, k8sClient); err != nil {
				return err
			}

			if err := wait.PollImmediate(time.Second*1, time.Minute*10, func() (done bool, err error) {

				if !gitopsDeplFixture.HaveSyncStatusCodeFunc(managedgitopsv1alpha1.SyncStatusCodeSynced, *gitOpsDeploymentResource) {
					return false, nil
				}

				if !gitopsDeplFixture.HaveHealthStatusCodeFunc(managedgitopsv1alpha1.HeathStatusCodeHealthy, *gitOpsDeploymentResource) {
					return false, nil
				}

				if !gitopsDeplFixture.HaveResourcesFunc(createResourceStatusList_deploymentPermutations, *gitOpsDeploymentResource) {
					return false, nil
				}

				return true, nil

			}); err != nil {
				return err
			}

			testPrintln("Modify the gitops deployment to a new repoURL", i)
			if err := k8s.Get(gitOpsDeploymentResource, k8sClient); err != nil {
				return err
			}

			gitOpsDeploymentResource.Spec.Source.RepoURL = "https://github.com/managed-gitops-test-data/deployment-permutations-b"

			testPrintln("updating the CR with changes in repoURLL", i)
			if err := k8s.Update(gitOpsDeploymentResource, k8sClient); err != nil {
				return err
			}

			expectedResourceStatusList := createResourceStatusListFunction_deploymentPermutations()
			expectedResourceStatusList = append(expectedResourceStatusList,
				getResourceStatusList_deploymentPermutations("deployment-permutations-b-brancha-pathb")...)

			if err := wait.PollImmediate(time.Second*1, time.Minute*10, func() (done bool, err error) {

				if !gitopsDeplFixture.HaveSyncStatusCodeFunc(managedgitopsv1alpha1.SyncStatusCodeSynced, *gitOpsDeploymentResource) {
					return false, nil
				}

				if !gitopsDeplFixture.HaveHealthStatusCodeFunc(managedgitopsv1alpha1.HeathStatusCodeHealthy, *gitOpsDeploymentResource) {
					return false, nil
				}

				if !gitopsDeplFixture.HaveResourcesFunc(expectedResourceStatusList, *gitOpsDeploymentResource) {
					return false, nil
				}

				return true, nil

			}); err != nil {
				return err
			}

			testPrintln("delete the GitOpsDeployment resource", i)
			if err := k8s.Delete(gitOpsDeploymentResource, k8sClient); err != nil {
				return err
			}

			testPrintln("delete namespace", i)
			if err := fixture.DeleteNamespace(namespace.ObjectMeta.Name, config); err != nil {
				return err
			}

			return nil
		}

		// runTests calls the inner test function, and returns the result on the channel
		runTest := func(i int, resChannel chan<- error, wg *sync.WaitGroup) {

			result := runTestInner(i)
			wg.Done()
			resChannel <- result
		}

		It("Simulates simple interactions of X active users interacting with the service ", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			resultChan := make(chan error)
			var wg sync.WaitGroup

			wg.Add(numberToSimulate)

			for i := 1; i <= numberToSimulate; i++ {
				go runTest(i, resultChan, &wg)
			}

			fmt.Println("Waiting for goroutines to finish...")
			wg.Wait()

			for j := 1; j <= numberToSimulate; j++ {
				res := <-resultChan
				fmt.Println("read from channel ", res)
				Expect(res).To(BeNil())
			}

			fmt.Println("Done!")
		})

	})

})

// Returns true if the all param resources no longer exist (have been deleted), and false otherwise.
func expectAllResourcesToBeDeleted(expectedResourceStatusList []managedgitopsv1alpha1.ResourceStatus) {

	Eventually(func() bool {

		k8sclient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
		Expect(err).To(Succeed())

		for _, resourceValue := range expectedResourceStatusList {
			resourceName := typed.NamespacedName{
				Namespace: resourceValue.Namespace,
				Name:      resourceValue.Name,
			}

			var obj client.Object

			if resourceValue.Kind == "ConfigMap" {
				obj = &corev1.ConfigMap{}
			} else if resourceValue.Kind == "Deployment" {
				obj = &appsv1.Deployment{}
			} else if resourceValue.Kind == "Route" {
				obj = &routev1.Route{}
			} else if resourceValue.Kind == "Service" {
				obj = &corev1.Service{}
			} else {
				GinkgoWriter.Println("unrecognize kind:", resourceValue.Kind)
				return false
			}

			// The object should not exist: a NotFound error should be returned, otherwise return false.
			if err := k8sclient.Get(context.Background(), resourceName, obj); err == nil || !apierr.IsNotFound(err) {
				fmt.Println("Waiting for resource to be deleted:", resourceName)
				return false
			}
		}

		// If all the resources were found, return true
		return true
	}, ArgoCDReconcileWaitTime, "1s").Should(BeTrue(), "all resources should be deleted")

}
