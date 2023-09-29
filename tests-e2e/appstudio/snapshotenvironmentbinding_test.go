package appstudio

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	dtcfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttargetclaim"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	appstudiosharedv1beta1 "github.com/redhat-appstudio/application-api/api/v1beta1"
	appstudiocontroller "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	bindingFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/binding"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"

	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SnapshotEnvironmentBinding Reconciler E2E tests", func() {

	Context("Testing SnapshotEnvironmentBinding Reconciler.", func() {

		var environment appstudiosharedv1beta1.Environment
		var k8sClient client.Client
		var err error

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the 'staging' Environment")
			environment = buildEnvironmentResource("staging", "my-environment", "", "")
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&environment, k8sClient)
			Expect(err).To(Succeed())

			if fixture.IsRunningInStonesoupEnvironment() {
				// Create an application
				application := buildApplication("new-demo-app", fixture.GitOpsServiceE2ENamespace, fixture.RepoURL)
				err = k8s.Create(&application, k8sClient)
				Expect(err).To(Succeed())

				// Create a snapshot for the application
				snapshot := buildSnapshotResource("my-snapshot", "new-demo-app", "", "", "", "")
				err = k8s.Create(&snapshot, k8sClient)
				Expect(err).To(Succeed())
			}

		})

		// This test is to verify the scenario when a user creates an SnapshotEnvironmentBinding CR in Cluster.
		// Then GitOps-Service should create GitOpsDeployment CR based on data given in Binding and update details of GitOpsDeployment in Status field of Binding.
		It("Should update Status of Binding and create new GitOpsDeployment CR.", func() {

			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")

			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a", "component-b"})

			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			// Update Status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/component-based-gitops-repository/components/componentA/overlays/dev", "resources/test-data/component-based-gitops-repository/components/componentB/overlays/dev"}, &binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			gitOpsDeploymentNameFirst := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)
			gitOpsDeploymentNameSecond := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[1].Name)

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{
				{
					ComponentName:                binding.Spec.Components[0].Name,
					GitOpsDeployment:             gitOpsDeploymentNameFirst,
					GitOpsDeploymentSyncStatus:   string(managedgitopsv1alpha1.SyncStatusCodeSynced),
					GitOpsDeploymentHealthStatus: string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
				},
				{
					ComponentName:                binding.Spec.Components[1].Name,
					GitOpsDeployment:             gitOpsDeploymentNameSecond,
					GitOpsDeploymentSyncStatus:   string(managedgitopsv1alpha1.SyncStatusCodeSynced),
					GitOpsDeploymentHealthStatus: string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
				},
			}

			Eventually(binding, "3m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeployments))

			//====================================================
			By("Verify that GitOpsDeployment CR created by GitOps-Service is having spec source as given in Binding.")

			gitOpsDeploymentFirst := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentNameFirst, binding.Namespace)
			Eventually(gitOpsDeploymentFirst, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))

			gitOpsDeploymentSecond := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentNameSecond, binding.Namespace)
			Eventually(gitOpsDeploymentSecond, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[1].GitOpsRepository.URL,
				Path:           binding.Status.Components[1].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[1].GitOpsRepository.Branch,
			}))

			//====================================================
			By("Verify that GitOpsDeployment CR created by GitOps-Service is having ownerReference according to Binding.")
			err = k8s.Get(&binding, k8sClient)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentFirst, k8sClient)
			Expect(err).To(Succeed())
			Expect(gitOpsDeploymentFirst.OwnerReferences[0].Name).To(Equal(binding.Name))
			Expect(gitOpsDeploymentFirst.OwnerReferences[0].UID).To(Equal(binding.UID))

			err = k8s.Get(&gitOpsDeploymentSecond, k8sClient)
			Expect(err).To(Succeed())
			Expect(gitOpsDeploymentSecond.OwnerReferences[0].Name).To(Equal(binding.Name))
			Expect(gitOpsDeploymentSecond.OwnerReferences[0].UID).To(Equal(binding.UID))
		})

		// Verifies a SnapshotEnvironmentBinding's status component deployment condition is set correctly when the
		// deployment of the components succeeds.
		It("updates the binding's status component deployment condition when the deployment of the components succeeds.", func() {
			if fixture.IsRunningInStonesoupEnvironment() {
				Skip("Skipping test as its running in Stonesoup environment")
			}
			By("creating binding cr and update the status field, because it is not updated when creating the object.")

			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a", "component-b"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			// Update Status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/sample-gitops-repository/components/componentA/overlays/staging", "resources/test-data/sample-gitops-repository/components/componentB/overlays/staging"}, &binding)
			Expect(err).ToNot(HaveOccurred())

			By("checking the status component deployment condition is true")
			Eventually(binding, "3m", "1s").Should(bindingFixture.HaveComponentDeploymentCondition(
				metav1.Condition{
					Type:    appstudiosharedv1.ComponentDeploymentConditionAllComponentsDeployed,
					Status:  metav1.ConditionTrue,
					Reason:  appstudiosharedv1.ComponentDeploymentConditionCommitsSynced,
					Message: "2 of 2 components deployed",
				}))

			By("updating the bindings status field to force an out-of-sync component")
			// Update Status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/sample-gitops-repository/components/componentA/overlays/staging", "resources/test-data/sample-gitops-repository/components/componentC/overlays/staging"}, &binding)
			Expect(err).ToNot(HaveOccurred())

			By("checking the status component deployment condition is false")
			Eventually(binding, "3m", "1s").Should(bindingFixture.HaveComponentDeploymentCondition(
				metav1.Condition{
					Type:    appstudiosharedv1.ComponentDeploymentConditionAllComponentsDeployed,
					Status:  metav1.ConditionFalse,
					Reason:  appstudiosharedv1.ComponentDeploymentConditionCommitsUnsynced,
					Message: "1 of 2 components deployed",
				}))
			Consistently(binding, "1m", "1s").Should(bindingFixture.HaveComponentDeploymentCondition(
				metav1.Condition{
					Type:    appstudiosharedv1.ComponentDeploymentConditionAllComponentsDeployed,
					Status:  metav1.ConditionFalse,
					Reason:  appstudiosharedv1.ComponentDeploymentConditionCommitsUnsynced,
					Message: "1 of 2 components deployed",
				}))

			By("updating the bindings status field to fix the out-of-sync component")
			// Update Status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/sample-gitops-repository/components/componentA/overlays/staging", "resources/test-data/sample-gitops-repository/components/componentB/overlays/staging"}, &binding)
			Expect(err).ToNot(HaveOccurred())

			By("checking the status component deployment condition is true")
			Eventually(binding, "3m", "1s").Should(bindingFixture.HaveComponentDeploymentCondition(
				metav1.Condition{
					Type:    appstudiosharedv1.ComponentDeploymentConditionAllComponentsDeployed,
					Status:  metav1.ConditionTrue,
					Reason:  appstudiosharedv1.ComponentDeploymentConditionCommitsSynced,
					Message: "2 of 2 components deployed",
				}))
		})

		//This test is to verify the scenario when a user creates an SnapshotEnvironmentBinding CR in Cluster and after GitOpsDeployment CR is created by GitOps-Service,
		// user does modification in Binding CR. In this case GitOps-Service should also update GitOpsDeployment CR accordingly.
		It("Should update GitOpsDeployment CR if Binding CR is updated.", func() {

			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")

			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			// Update Status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/component-based-gitops-repository/components/componentA/overlays/dev"}, &binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName:                binding.Spec.Components[0].Name,
				GitOpsDeployment:             gitOpsDeploymentName,
				GitOpsDeploymentSyncStatus:   string(managedgitopsv1alpha1.SyncStatusCodeSynced),
				GitOpsDeploymentHealthStatus: string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
			}}

			Eventually(binding, "4m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeployments))

			//====================================================
			By("Verify that GitOpsDeployment CR created, is having Spec.Source as given in Binding.")

			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)

			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))

			//====================================================
			By("Verify that GitOpsDeployment CR is updated by GitOps-Service if the Binding is updated, for example, if the Binding were to be updated by HAS")

			err = k8s.Get(&binding, k8sClient)
			Expect(err).To(Succeed())
			binding.Status.Components[0].GitOpsRepository.Path = "resources/test-data/sample-gitops-repository/components/componentA/overlays/dev"
			err = k8s.Update(&binding, k8sClient)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeployments))

			//====================================================
			By("Verify that GitOpsDeployment CR updated by GitOps-Service is having Spec.Source as given in Binding.")

			err = k8s.Get(&gitOpsDeployment, k8sClient)
			Expect(err).To(Succeed())

			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		// This test is to verify the scenario when a user creates an SnapshotEnvironmentBinding CR in Cluster and then GitOps-Service creates GitOpsDeployment CR,
		// but the user does modification directly in the GitOpsDeployment CR. In this case GitOps-Service service should revert changes done by the user in GitOpsDeployment CR.
		It("Should revert GitOpsDeployment, if modification are done directly for it, without updating Binding CR.", func() {

			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")
			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			// Update the Status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/component-based-gitops-repository/components/componentA/overlays/dev"}, &binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName:                binding.Spec.Components[0].Name,
				GitOpsDeployment:             gitOpsDeploymentName,
				GitOpsDeploymentSyncStatus:   string(managedgitopsv1alpha1.SyncStatusCodeSynced),
				GitOpsDeploymentHealthStatus: string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeployments))

			//====================================================
			By("Verify that GitOpsDeployment CR created, is having Spec.Source as given in Binding.")

			gitOpsDeploymentBefore := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)

			Eventually(gitOpsDeploymentBefore, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))

			//====================================================
			By("Update GitOpsDeployment CR, but dont change anything is in Binding CR.")

			err = gitopsDeplFixture.UpdateDeploymentWithFunction(&gitOpsDeploymentBefore, func(depl *managedgitopsv1alpha1.GitOpsDeployment) {
				depl.Spec.Source.Path = "resources/test-data/sample-gitops-repository/components/componentA/overlays/dev"
			})
			Expect(err).ToNot(HaveOccurred())

			//====================================================
			By("Verify that GitOpsDeployment CR is reverted by GitOps-Service is having same Spec.Source as given in Binding.")

			gitOpsDeploymentAfter := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)

			Eventually(gitOpsDeploymentAfter, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		// This test is to verify the scenario when a user creates an SnapshotEnvironmentBinding CR in Cluster and then GitOps-Service creates GitOpsDeployment CR.
		// Now user deletes GitOpsDeployment from Cluster but Binding is still present in Cluster. In this case GitOps-Service should recreate GitOpsDeployment CR as given in Binding.
		It("Should recreate GitOpsDeployment, if Binding still exists but GitOpsDeployment is deleted.", func() {
			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")
			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			// Update the Status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL,
				"main", "adcda66", []string{"resources/test-data/sample-gitops-repository/components/componentA/overlays/staging"}, &binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)
			// gitOpsDeploymentName := binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + binding.Spec.Components[0].Name

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName: binding.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentName,
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeployments, true))

			//====================================================
			By("Verify that GitOpsDeployment CR created, is having Spec.Source as given in Binding.")

			gitOpsDeploymentBefore := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)

			Eventually(gitOpsDeploymentBefore, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))

			//====================================================
			By("Delete GitOpsDeployment CR created by GitOps-Service, but not the Binding.")

			Expect(k8sClient.Delete(context.Background(), &gitOpsDeploymentBefore)).To(Succeed())

			//====================================================
			By("Verify that GitOpsDeployment CR is recreated by GitOps-Service.")

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeployments, true))

			gitOpsDeploymentAfter := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)

			Eventually(gitOpsDeploymentAfter, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		It("Should delete the corresponding GitOps Deployment when a component is removed", func() {
			By("Creating a Binding")
			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a", "component-b"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL,
				"main", "adcda66", []string{
					"resources/test-data/sample-gitops-repository/components/componentA/overlays/staging",
					"resources/test-data/sample-gitops-repository/components/componentB/overlays/staging",
				}, &binding)
			Expect(err).To(Succeed())

			gitOpsDeploymentName0 := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)
			gitOpsDeploymentName1 := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[1].Name)

			By("Ensuring both GitOps Deployments exist")
			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName0, binding.Namespace)
			Eventually(&gitOpsDeployment, "2m", "1s").Should(k8s.ExistByName(k8sClient))
			gitOpsDeployment = buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName1, binding.Namespace)
			Eventually(&gitOpsDeployment, "2m", "1s").Should(k8s.ExistByName(k8sClient))

			By("Removing the first component")
			err = buildAndUpdateBindingStatus(binding.Spec.Components[1:],
				fixture.RepoURL,
				"main", "adcda66", []string{"resources/test-data/sample-gitops-repository/components/componentA/overlays/staging"}, &binding)
			Expect(err).To(Succeed())

			By("Ensuring the first GitOps Deployment has been deleted")
			gitOpsDeployment = buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName0, binding.Namespace)
			Eventually(&gitOpsDeployment, "2m", "1s").ShouldNot(k8s.ExistByName(k8sClient))

			By("Ensuring the second GitOps Deployment still exists")
			gitOpsDeployment = buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName1, binding.Namespace)
			Consistently(&gitOpsDeployment, "30s", "5s").Should(k8s.ExistByName(k8sClient))

			By("Removing the remaining component")
			err = buildAndUpdateBindingStatus(nil, "", "", "", []string{""}, &binding)
			Expect(err).To(Succeed())

			By("Ensuring the first GitOps Deployment is still deleted")
			gitOpsDeployment = buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName0, binding.Namespace)
			Eventually(&gitOpsDeployment, "1m", "1s").ShouldNot(k8s.ExistByName(k8sClient))

			By("Ensuring the second GitOps Deployment has been deleted")
			gitOpsDeployment = buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName1, binding.Namespace)
			Eventually(&gitOpsDeployment, "1m", "1s").ShouldNot(k8s.ExistByName(k8sClient))
		})

		// This test is to verify the scenario when a user creates an SnapshotEnvironmentBinding CR in Cluster but GitOpsDeployment CR default name is too long.
		// By default the naming convention used for GitOpsDeployment is <Binding Name>-<Application Name>-<Environment Name>-<Components Name> and If name exceeds the max limit then GitOps-Service should follow short name <Binding Name>-<Components Name>.
		// In this test GitOps-Service should use short naming convention instead of default one.
		It("Should use short name for GitOpsDeployment, if Name field length is more than max length.", func() {
			By("Creating an Environment with a long name")
			environment := buildEnvironmentResource(strings.Repeat("e", 63), "my-environment", "", "")
			err = k8s.Create(&environment, k8sClient)
			Expect(err).To(Succeed())

			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")

			binding := buildSnapshotEnvironmentBindingResource(strings.Repeat("b", 111), "new-demo-app", environment.Name, "my-snapshot", 3, []string{"component-a"})
			binding.Spec.Application = strings.Repeat("a", 63)
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			// Update the status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/sample-gitops-repository/components/componentA/overlays/staging"}, &binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that short name is used for GitOpsDeployment CR instead.")

			gitOpsDeploymentName := binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + binding.Spec.Components[0].Name

			// Check no GitOpsDeployment CR found with default name (longer name).
			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)
			Consistently(&gitOpsDeployment, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient), "wait 30s for the object not to exist")

			// Check GitOpsDeployment is created with short name.
			gitOpsDeployment.Name = binding.Name + "-" + binding.Spec.Components[0].Name
			Eventually(&gitOpsDeployment, "2m", "1s").Should(k8s.ExistByName(k8sClient))

			// Check GitOpsDeployment is having repository data as given in Binding.
			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		// This test is to verify the scenario when a user creates an ApplicationSnapshotEnvironmentBinding CR in Cluster but GitOpsDeployment CR default name is too long.
		// By default the naming convention used for GitOpsDeployment is <Binding Name>-<Application Name>-<Environment Name>-<Components Name> and If name exceeds the max limit then GitOps-Service should follow short name <Binding Name>-<Components Name>,
		// but there is a possibility that this short name is still exceeds the max limit then we would shorten it again,
		// it will use first 210 characters of <Binding Name>-<Components Name> and then append Hash value of atual expected name i.e. <Binding Name>-<Application Name>-<Environment Name>-<Components Name>.
		// Since we used 210 characters from short name and Hash value will be of 32 characters the length will always be 243.
		// Ex: 210 (First 210 characters of combination of Binding name and Component name) + 1 ("-") + 32 (length of UUID) = 243 (Total length)
		It("Should use short name with hash value for GitOpsDeployment, if combination of Binding name and Component name is still longer than 250 characters.", func() {

			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")

			binding := buildSnapshotEnvironmentBindingResource(strings.Repeat("abcde", 50), "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			compName := strings.Repeat("wxyz", 5)
			binding.Spec.Components[0].Name = compName

			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			// Get the short name with hash value.
			hashValue := sha256.Sum256([]byte(binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + compName))
			hashString := fmt.Sprintf("%x", hashValue)
			expectedName := (binding.Name + "-" + compName)[0:180] + "-" + hashString

			// Update the status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/component-based-gitops-repository/components/componentA/overlays/staging"}, &binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that short name is used for GitOpsDeployment CR.")

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName:                binding.Spec.Components[0].Name,
				GitOpsDeployment:             expectedName,
				GitOpsDeploymentSyncStatus:   string(managedgitopsv1alpha1.SyncStatusCodeSynced),
				GitOpsDeploymentHealthStatus: string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeployments))

			// Check no GitOpsDeployment CR found with default name (longer name).
			gitOpsDeploymentName := binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + binding.Spec.Components[0].Name
			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)
			err = k8s.Get(&gitOpsDeployment, k8sClient)
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			// Check GitOpsDeployment is created with short name).
			gitOpsDeployment.Name = expectedName
			err = k8s.Get(&gitOpsDeployment, k8sClient)
			Expect(err).To(Succeed())

			// Check GitOpsDeployment is having repository data as given in Binding.
			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		It("ensures that GitOpsDeployments .spec.destination.namespace field always matching the expected Environment value", func() {
			By("creating a new Environment targetting a fake remote cluster")
			environment := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        "my-env",
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration: appstudiosharedv1beta1.EnvironmentConfiguration{
						Env: []appstudiosharedv1beta1.EnvVarPair{},
					},
					// The cluster doesn't need to be real for this test
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
							TargetNamespace:            "namespace1",
							APIURL:                     "https://fake-url-does-not-exist.redhat.com",
							ClusterCredentialsSecret:   "my-secret",
							AllowInsecureSkipTLSVerify: true,
						},
					},
				},
			}
			err = k8s.Create(&environment, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("building a SnapshotEnvironmentBinding targeting that Environment")
			seb := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", environment.Name, "my-snapshot", 3, []string{"component-a"})

			err = k8s.Create(&seb, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("mocking the ApplicationService updating the Environment status")
			// Update the status field
			err = buildAndUpdateBindingStatus(seb.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/sample-gitops-repository/components/componentA/overlays/staging"}, &seb)
			Expect(err).To(Succeed())

			gitopsDepl := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      appstudiocontroller.GenerateBindingGitOpsDeploymentName(seb, "component-a"),
					Namespace: seb.Namespace,
				},
			}
			Eventually(gitopsDepl, "60s", "1s").
				Should(gitopsDeplFixture.HaveTargetNamespace(environment.Spec.Target.TargetNamespace),
					"should match the Environment's current target ns value")

			By("updating the Environment to target another namespace")
			err = k8s.UpdateWithoutConflict(&environment, k8sClient, func(obj client.Object) {

				envObj, ok := obj.(*appstudiosharedv1beta1.Environment)

				Expect(ok).To(BeTrue())
				envObj.Spec.Target.TargetNamespace = "another-namespace"

			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(gitopsDepl, "60s", "1s").Should(gitopsDeplFixture.HaveTargetNamespace("another-namespace"), "should match the updated value")

			var clusterSecret corev1.Secret
			By("create a secret of type Opaque, based on a managed environment type Secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env-secret",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Type:       sharedutil.ManagedEnvironmentSecretType,
				StringData: map[string]string{"kubeconfig": k8s.GenerateFakeKubeConfig()},
			}

			clusterSecret = corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-opaque-secret",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Type: corev1.SecretTypeOpaque,
			}
			clusterSecret.Data = secret.Data

			err = k8s.Create(&clusterSecret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("create a new DeploymentTarget and DeploymentTargetClaim pointing to fake secret credentials")
			dt, dtc := dtcfixture.BuildDeploymentTargetAndDeploymentTargetClaim(k8s.GenerateFakeKubeConfig(), "https://fake-url.redhat.com", clusterSecret.Name, clusterSecret.Namespace, "some-other-dtc-namespace", fixture.DTName, fixture.DTCName, "test-class", true)

			err = k8s.Create(&dt, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Create(&dtc, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("updating the Environment to instead point to the DeploymentTargetClaim")
			err = k8s.UpdateWithoutConflict(&environment, k8sClient, func(obj client.Object) {

				envObj, ok := obj.(*appstudiosharedv1beta1.Environment)
				Expect(ok).To(BeTrue())
				envObj.Spec.Target = &appstudiosharedv1beta1.TargetConfiguration{
					Claim: appstudiosharedv1beta1.TargetClaim{
						DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
							ClaimName: dtc.Name,
						},
					},
				}
				envObj.Spec.Configuration = appstudiosharedv1beta1.EnvironmentConfiguration{
					Env: []appstudiosharedv1beta1.EnvVarPair{},
				}
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(gitopsDepl, "60s", "1s").
				Should(gitopsDeplFixture.HaveTargetNamespace(dt.Spec.KubernetesClusterCredentials.DefaultNamespace))

			By("update the DeploymentTarget to point to another namespace")
			err = k8s.UpdateWithoutConflict(&dt, k8sClient, func(obj client.Object) {

				dtObj, ok := obj.(*appstudiosharedv1.DeploymentTarget)
				Expect(ok).To(BeTrue())

				dtObj.Spec.KubernetesClusterCredentials.DefaultNamespace = "some-other-dtc-namespace"
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(gitopsDepl, "60s", "1s").
				Should(gitopsDeplFixture.HaveTargetNamespace("some-other-dtc-namespace"), "should match the new value set in DeploymentTarget")

			By("updating the Environment back from DTC-based credentials, back to K8s cluster credentials, and using another namespace")
			err = k8s.UpdateWithoutConflict(&environment, k8sClient, func(obj client.Object) {

				envObj, ok := obj.(*appstudiosharedv1beta1.Environment)
				Expect(ok).To(BeTrue())

				envObj.Spec.Configuration = appstudiosharedv1beta1.EnvironmentConfiguration{
					Env: []appstudiosharedv1beta1.EnvVarPair{},
				}
				envObj.Spec.Target = &appstudiosharedv1beta1.TargetConfiguration{
					KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
						TargetNamespace:            "namespace1",
						APIURL:                     "https://fake-url-does-not-exist.redhat.com",
						ClusterCredentialsSecret:   "my-secret",
						AllowInsecureSkipTLSVerify: true,
					},
				}

			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(gitopsDepl, "60s", "1s").
				Should(gitopsDeplFixture.HaveTargetNamespace("namespace1"), "should match the final NS value set above")

		})

		It("should create a GitOpsDeployment that references cluster credentials specified in Environment", func() {
			By("creating second managed environment Secret")
			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-secret",
					Namespace: environment.Namespace,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					"kubeconfig": ([]byte)("{}"),
				},
			}
			err = k8s.Create(&secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Get(&environment, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			environment.Spec.Target = &appstudiosharedv1beta1.TargetConfiguration{
				KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
					TargetNamespace:          fixture.GitOpsServiceE2ENamespace,
					APIURL:                   "https://api-url",
					ClusterCredentialsSecret: "my-secret",
				},
			}

			err = k8s.Update(&environment, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("generating the Binding, and waiting for the corresponding GitOpsDeployment to exist")

			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			// Update the status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/sample-gitops-repository/components/componentA/overlays/staging"}, &binding)
			Expect(err).To(Succeed())

			By("waiting for the the controller to Reconcile the GitOpsDeplyoment")
			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentName,
					Namespace: binding.Namespace,
				},
			}

			Eventually(&gitopsDeployment, "60s", "1s").Should(k8s.ExistByName(k8sClient))

			err = k8s.Get(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			Expect(gitopsDeployment.Spec.Destination.Environment).To(Equal("managed-environment-"+environment.Name),
				"the destination should be the environment")
			Expect(gitopsDeployment.Spec.Destination.Namespace).
				To(Equal(environment.Spec.Target.KubernetesClusterCredentials.TargetNamespace),
					"the namespace of the GitOpsDeployment should come from the Environment")
		})

		It("Should ensure the associated GitOpsDeployment has labels identifying the application, component and environment", func() {
			By("Create SnapshotEnvironmentBindingResource")
			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			By("Update Status field of SnapshotEnvironmentBindingResource")
			// Update the status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/component-based-gitops-repository/components/componentA/overlays/staging"}, &binding)
			Expect(err).To(Succeed())

			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")
			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName:                binding.Spec.Components[0].Name,
				GitOpsDeployment:             gitOpsDeploymentName,
				GitOpsDeploymentSyncStatus:   string(managedgitopsv1alpha1.SyncStatusCodeSynced),
				GitOpsDeploymentHealthStatus: string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeployments))

			By("Verify whether `gitopsDeployment.ObjectMeta.Labels` contains labels identifying the application, component and environment")
			gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentName,
					Namespace: binding.Namespace,
				},
			}

			err = k8s.Get(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io/application", binding.Spec.Application))
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io/component", binding.Spec.Components[0].Name))
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io/environment", binding.Spec.Environment))
		})

		It("Should ensure the associated GitOpsDeployment adds back the labels identifying the application, component and environment if they are removed", func() {
			By("Create SnapshotEnvironmentBindingResource")
			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			By("Update Status field of SnapshotEnvironmentBindingResource")
			// Update the status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/component-based-gitops-repository/components/componentA/overlays/staging"}, &binding)
			Expect(err).To(Succeed())

			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")
			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName:                binding.Spec.Components[0].Name,
				GitOpsDeployment:             gitOpsDeploymentName,
				GitOpsDeploymentSyncStatus:   string(managedgitopsv1alpha1.SyncStatusCodeSynced),
				GitOpsDeploymentHealthStatus: string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeployments))

			By("Verify whether `gitopsDeployment.ObjectMeta.Labels` contains labels identifying the application, component and environment")
			gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentName,
					Namespace: binding.Namespace,
				},
			}
			err = k8s.Get(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io/application", binding.Spec.Application))
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io/component", binding.Spec.Components[0].Name))
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io/environment", binding.Spec.Environment))

			By("Removing the labels")
			err = k8s.Get(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			delete(gitopsDeployment.ObjectMeta.Labels, "appstudio.openshift.io/application")
			delete(gitopsDeployment.ObjectMeta.Labels, "appstudio.openshift.io/component")
			delete(gitopsDeployment.ObjectMeta.Labels, "appstudio.openshift.io/environment")
			err = k8s.Update(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying the labels have been added back")
			gitopsDeployment = managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentName,
					Namespace: binding.Namespace,
				},
			}
			err = k8s.Get(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io/application", binding.Spec.Application))
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io/component", binding.Spec.Components[0].Name))
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io/environment", binding.Spec.Environment))
		})

		It("Should append ASEB labels with key `appstudio.openshift.io` to GitopsDeployment label", func() {
			By("Create SnapshotEnvironmentBindingResource")
			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			binding.ObjectMeta.Labels["appstudio.openshift.io"] = "testing"
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			By("Update Status field of SnapshotEnvironmentBindingResource")
			// Update the status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/component-based-gitops-repository/components/componentA/overlays/staging"}, &binding)
			Expect(err).To(Succeed())

			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")
			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName:                binding.Spec.Components[0].Name,
				GitOpsDeployment:             gitOpsDeploymentName,
				GitOpsDeploymentSyncStatus:   string(managedgitopsv1alpha1.SyncStatusCodeSynced),
				GitOpsDeploymentHealthStatus: string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeployments))

			By("Verify whether `gitopsDeployment.ObjectMeta.Labels` is updated with ASEB labels")
			gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentName,
					Namespace: binding.Namespace,
				},
			}

			err = k8s.Get(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io", "testing"))
		})

		It("Should not append ASEB label without appstudio.openshift.io label into the GitopsDeployment Label", func() {
			By("Create SnapshotEnvironmentBindingResource")
			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			By("Update Status field of SnapshotEnvironmentBindingResource")
			// Update the status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/component-based-gitops-repository/components/componentA/overlays/staging"}, &binding)
			Expect(err).To(Succeed())

			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")
			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName:                binding.Spec.Components[0].Name,
				GitOpsDeployment:             gitOpsDeploymentName,
				GitOpsDeploymentSyncStatus:   string(managedgitopsv1alpha1.SyncStatusCodeSynced),
				GitOpsDeploymentHealthStatus: string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeployments))

			By("Verify whether `gitopsDeployment.ObjectMeta.Labels` is not updated with ASEB labels")
			gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentName,
					Namespace: binding.Namespace,
				},
			}

			err = k8s.Get(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Eventually(gitopsDeployment, "2m", "10s").ShouldNot(gitopsDeplFixture.HaveLabel("appstudio.openshift.io", "testing"))
		})

		It("Should update gitopsDeployment label if ASEB label gets updated", func() {
			By("Create SnapshotEnvironmentBindingResource")
			binding := buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})

			binding.ObjectMeta.Labels["appstudio.openshift.io"] = "testing"
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			// Update the status field
			err = buildAndUpdateBindingStatus(binding.Spec.Components,
				fixture.RepoURL, "main", "adcda66",
				[]string{"resources/test-data/component-based-gitops-repository/components/componentA/overlays/staging"}, &binding)
			Expect(err).To(Succeed())

			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")
			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName:                binding.Spec.Components[0].Name,
				GitOpsDeployment:             gitOpsDeploymentName,
				GitOpsDeploymentSyncStatus:   string(managedgitopsv1alpha1.SyncStatusCodeSynced),
				GitOpsDeploymentHealthStatus: string(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeployments))

			By("Verify whether `gitopsDeployment.ObjectMeta.Labels` is not updated with ASEB labels")
			gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentName,
					Namespace: binding.Namespace,
				},
			}

			err = k8s.Get(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels["appstudio.openshift.io"]).To(Equal("testing"))

			err = k8s.Get(&binding, k8sClient)
			Expect(err).To(Succeed())

			// Update binding label
			binding.ObjectMeta.Labels["appstudio.openshift.io"] = "testing-update"
			err = k8s.Update(&binding, k8sClient)
			Expect(err).To(Succeed())

			By("Verify whether `gitopsDeployment.ObjectMeta.Labels` is updated with ASEB labels")
			err = k8s.Get(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Eventually(gitopsDeployment, "2m", "10s").Should(gitopsDeplFixture.HaveLabel("appstudio.openshift.io", "testing-update"))

			By("Remove ASEB label `appstudio.openshift.io` label and verify whether it is removed from gitopsDeployment label")
			err = k8s.UntilSuccess(k8sClient, func(k8sClient client.Client) error {
				// Retrieve the latest version of the SnapshotEnvironmentBinding resource
				err := k8s.Get(&binding, k8sClient)
				if err != nil {
					return err
				}
				delete(binding.ObjectMeta.Labels, "appstudio.openshift.io")
				return k8s.Update(&binding, k8sClient)
			})
			Expect(err).To(Succeed())

			By("Verify whether gitopsDeployment.ObjectMeta.Label `appstudio.openshift.io` is removed from gitopsDeployment")
			err = k8s.Get(&gitopsDeployment, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Eventually(gitopsDeployment, "2m", "10s").ShouldNot(gitopsDeplFixture.HaveLabel("appstudio.openshift.io", "testing-update"))
		})
	})

})

// buildAndUpdateBindingStatus builds and updates the status field of SnapshotEnvironmentBinding CR
func buildAndUpdateBindingStatus(components []appstudiosharedv1.BindingComponent, url,
	branch, commitID string, path []string, binding *appstudiosharedv1.SnapshotEnvironmentBinding) error {

	By(fmt.Sprintf("updating Status field of SnapshotEnvironmentBindingResource for '%s' of '%s' in '%v'", url, branch, path))

	return bindingFixture.UpdateStatusWithFunction(binding, func(bindingStatus *appstudiosharedv1.SnapshotEnvironmentBindingStatus) {

		// Update the binding status
		*bindingStatus = buildSnapshotEnvironmentBindingStatus(components,
			url, branch, commitID, path)

	})
}

// buildSnapshotEnvironmentBindingResource builds the SnapshotEnvironmentBinding CR
func buildSnapshotEnvironmentBindingResource(name, appName, envName, snapshotName string, replica int, componentNames []string) appstudiosharedv1.SnapshotEnvironmentBinding {
	// Create SnapshotEnvironmentBinding CR.
	binding := appstudiosharedv1.SnapshotEnvironmentBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
			Labels: map[string]string{
				"appstudio.application": appName,
				"appstudio.environment": envName,
			},
		},
		Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
			Application: appName,
			Environment: envName,
			Snapshot:    snapshotName,
		},
	}
	components := []appstudiosharedv1.BindingComponent{}
	for _, name := range componentNames {
		components = append(components, appstudiosharedv1.BindingComponent{
			Name:          name,
			Configuration: appstudiosharedv1.BindingComponentConfiguration{Replicas: &replica},
		})
	}
	binding.Spec.Components = components
	return binding
}

// buildSnapshotEnvironmentBindingStatus builds the status fields that needs to be updated
// for the SnapshotEnvironmentBinding CR
func buildSnapshotEnvironmentBindingStatus(components []appstudiosharedv1.BindingComponent, url,
	branch, commitID string, path []string) appstudiosharedv1.SnapshotEnvironmentBindingStatus {

	// Create SnapshotEnvironmentBindingStatus object.
	status := appstudiosharedv1.SnapshotEnvironmentBindingStatus{}

	var componentStatus []appstudiosharedv1.BindingComponentStatus

	for i, component := range components {
		componentStatus = append(componentStatus, appstudiosharedv1.BindingComponentStatus{
			Name: component.Name,
			GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
				URL: url, Branch: branch, Path: path[i], GeneratedResources: []string{}, CommitID: commitID,
			},
		})
	}

	status.Components = componentStatus
	return status
}

// buildApplication creates an instance of Application CR
func buildApplication(appName, appNamespace, url string) appstudiosharedv1.Application {
	// Create application.appstudio.redhat.com object.
	application := appstudiosharedv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: appNamespace,
		},
		Spec: appstudiosharedv1.ApplicationSpec{
			DisplayName: appName,
			GitOpsRepository: appstudiosharedv1.ApplicationGitRepository{
				URL: url,
			},
		},
	}
	return application
}

// buildGitOpsDeploymentObjectMeta creates ObjectMetadata for a GitopsDeployment CR
func buildGitOpsDeploymentObjectMeta(name, namespace string) managedgitopsv1alpha1.GitOpsDeployment {
	// Create GitOpsDeployment object only with ObjectMeta.
	gitOpsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return gitOpsDeployment
}
