package core

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"

	appstudiocontroller "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	bindingFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/binding"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"

	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ApplicationSnapshotEnvironmentBinding Reconciler E2E tests", func() {

	Context("Testing ApplicationSnapshotEnvironmentBinding Reconciler.", func() {

		var environment appstudiosharedv1.Environment
		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the 'staging' Environment")
			environment = appstudiosharedv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "staging",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudiosharedv1.EnvironmentSpec{
					Type:               appstudiosharedv1.EnvironmentType_POC,
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration: appstudiosharedv1.EnvironmentConfiguration{
						Env: []appstudiosharedv1.EnvVarPair{},
					},
				},
			}
			err := k8s.Create(&environment)
			Expect(err).To(Succeed())

		})

		// This test is to verify the scenario when a user creates an ApplicationSnapshotEnvironmentBinding CR in Cluster.
		// Then GitOps-Service should create GitOpsDeployment CR based on data given in Binding and update details of GitOpsDeployment in Status field of Binding.
		It("Should update Status of Binding and create new GitOpsDeployment CR.", func() {

			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")

			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a", "component-b"})
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			// Update Status field
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status = buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template", "main", "fdhyqtw", []string{"components/componentA/overlays/staging", "components/componentB/overlays/staging"})
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			gitOpsDeploymentNameFirst := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)
			gitOpsDeploymentNameSecond := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[1].Name)

			// gitOpsDeploymentNameFirst := binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + binding.Spec.Components[0].Name
			// gitOpsDeploymentNameSecond := binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + binding.Spec.Components[1].Name

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{
				{ComponentName: binding.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentNameFirst},
				{ComponentName: binding.Spec.Components[1].Name, GitOpsDeployment: gitOpsDeploymentNameSecond},
			}

			Eventually(binding, "3m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeployments))

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
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentFirst)
			Expect(err).To(Succeed())
			Expect(gitOpsDeploymentFirst.OwnerReferences[0].Name).To(Equal(binding.Name))
			Expect(gitOpsDeploymentFirst.OwnerReferences[0].UID).To(Equal(binding.UID))

			err = k8s.Get(&gitOpsDeploymentSecond)
			Expect(err).To(Succeed())
			Expect(gitOpsDeploymentSecond.OwnerReferences[0].Name).To(Equal(binding.Name))
			Expect(gitOpsDeploymentSecond.OwnerReferences[0].UID).To(Equal(binding.UID))
		})

		//This test is to verify the scenario when a user creates an ApplicationSnapshotEnvironmentBinding CR in Cluster and after GitOpsDeployment CR is created by GitOps-Service,
		// user does modification in Binding CR. In this case GitOps-Service should also update GitOpsDeployment CR accordingly.
		It("Should update GitOpsDeployment CR if Binding CR is updated.", func() {

			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")

			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			// Update Status field
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status = buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template", "main", "fdhyqtw", []string{"components/componentA/overlays/staging"})
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)
			// gitOpsDeploymentName := binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + binding.Spec.Components[0].Name

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName: binding.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentName,
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeployments))

			//====================================================
			By("Verify that GitOpsDeployment CR created, is having Spec.Source as given in Binding.")

			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)

			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))

			//====================================================
			By("Verify that GitOpsDeployment CR is updated by GitOps-Service as Binding is updated.")

			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status.Components[0].GitOpsRepository.Path = "components/componentA/overlays/dev"
			err = k8s.Update(&binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeployments))

			//====================================================
			By("Verify that GitOpsDeployment CR updated by GitOps-Service is having Spec.Source as given in Binding.")

			err = k8s.Get(&gitOpsDeployment)
			Expect(err).To(Succeed())

			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		// This test is to verify the scenario when a user creates an ApplicationSnapshotEnvironmentBinding CR in Cluster and then GitOps-Service creates GitOpsDeployment CR,
		// but the user does modification directly in the GitOpsDeployment CR. In this case GitOps-Service service should revert changes done by the user in GitOpsDeployment CR.
		It("Should revert GitOpsDeployment, if modification are done directly for it, without updating Binding CR.", func() {

			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")

			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			// Update the Status field
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status = buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template", "main", "fdhyqtw", []string{"components/componentA/overlays/staging"})
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)
			// gitOpsDeploymentName := binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + binding.Spec.Components[0].Name

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName: binding.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentName,
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeployments))

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

			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status.Components[0].GitOpsRepository.Path = "components/componentA/overlays/dev"
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that GitOpsDeployment CR is reverted by GitOps-Service is having same Spec.Source as given in Binding.")

			gitOpsDeploymentAfter := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)

			Eventually(gitOpsDeploymentAfter, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		// This test is to verify the scenario when a user creates an ApplicationSnapshotEnvironmentBinding CR in Cluster and then GitOps-Service creates GitOpsDeployment CR.
		// Now user deletes GitOpsDeployment from Cluster but Binding is still present in Cluster. In this case GitOps-Service should recreate GitOpsDeployment CR as given in Binding.
		It("Should recreate GitOpsDeployment, if Binding still exists but GitOpsDeployment is deleted.", func() {

			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")

			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app",
				"staging", "my-snapshot", 3, []string{"component-a"})
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			// Update the Status field
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status = buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template",
				"main", "fdhyqtw", []string{"components/componentA/overlays/staging"})
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")

			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)
			// gitOpsDeploymentName := binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + binding.Spec.Components[0].Name

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName: binding.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentName,
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeployments))

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

			err = k8s.Delete(&gitOpsDeploymentBefore)
			Expect(err).To(Succeed())

			err = k8s.Get(&gitOpsDeploymentBefore)
			Expect(err).NotTo(Succeed())
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			//====================================================
			By("Verify that GitOpsDeployment CR is recreated by GitOps-Service.")

			// Update any value in Binding just to trigger Reconciler.
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Spec.Components[0].Configuration.Replicas = 2
			err = k8s.Update(&binding)
			Expect(err).To(Succeed())

			gitOpsDeploymentAfter := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)
			err = k8s.Get(&gitOpsDeploymentAfter)
			Expect(err).To(Succeed())

			Eventually(gitOpsDeploymentAfter, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		// This test is to verify the scenario when a user creates an ApplicationSnapshotEnvironmentBinding CR in Cluster but GitOpsDeployment CR default name is too long.
		// By default the naming convention used for GitOpsDeployment is <Binding Name>-<Application Name>-<Environment Name>-<Components Name> and If name exceeds the max limit then GitOps-Service should follow short name <Binding Name>-<Components Name>.
		// In this test GitOps-Service should use short naming convention instead of default one.
		It("Should use short name for GitOpsDeployment, if Name field length is more than max length.", func() {

			By("Create Binding CR in Cluster and it requires to update the Status field of Binding, because it is not updated while creating object.")

			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			binding.Spec.Application = strings.Repeat("abcde", 45)
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			// Update the status field
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status = buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template", "main", "fdhyqtw", []string{"components/componentA/overlays/staging"})
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			//====================================================
			By("Verify that short name is used for GitOpsDeployment CR instead.")

			gitOpsDeploymentName := binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + binding.Spec.Components[0].Name

			// Check no GitOpsDeployment CR found with default name (longer name).
			gitOpsDeployment := buildGitOpsDeploymentObjectMeta(gitOpsDeploymentName, binding.Namespace)
			err = k8s.Get(&gitOpsDeployment)
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			// Check GitOpsDeployment is created with short name).
			gitOpsDeployment.Name = binding.Name + "-" + binding.Spec.Components[0].Name
			err = k8s.Get(&gitOpsDeployment)
			Expect(err).To(Succeed())

			// Check GitOpsDeployment is having repository data as given in Binding.
			Eventually(gitOpsDeployment, "2m", "1s").Should(gitopsDeplFixture.HaveSpecSource(managedgitopsv1alpha1.ApplicationSource{
				RepoURL:        binding.Status.Components[0].GitOpsRepository.URL,
				Path:           binding.Status.Components[0].GitOpsRepository.Path,
				TargetRevision: binding.Status.Components[0].GitOpsRepository.Branch,
			}))
		})

		It("should create a GitOpsDeployment that references cluster credentials specified in Environment", func() {

			// ToDo: solve GITOPSRVC-217, and remove this constraint
			if fixture.IsRunningAgainstKCP() {
				Skip("Skipping this test because of race condition when running on KCP based env")
			}

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
			err := k8s.Create(&secret)
			Expect(err).To(BeNil())

			err = k8s.Get(&environment)
			Expect(err).To(BeNil())

			environment.Spec.UnstableConfigurationFields = &appstudiosharedv1.UnstableEnvironmentConfiguration{
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					TargetNamespace:          fixture.GitOpsServiceE2ENamespace,
					APIURL:                   "https://api-url",
					ClusterCredentialsSecret: "my-secret",
				},
			}

			err = k8s.Update(&environment)
			Expect(err).To(BeNil())

			By("generating the Binding, and waiting for the corresponding GitOpsDeployment to exist")

			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding)
			Expect(err).To(BeNil())

			binding.Status = buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components,
				"https://github.com/redhat-appstudio/gitops-repository-template", "main", "fdhyqtw",
				[]string{"components/componentA/overlays/staging"})

			err = k8s.UpdateStatus(&binding)
			Expect(err).To(BeNil())

			By("waiting for the the controller to Reconcile the GitOpsDeplyoment")
			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentName,
					Namespace: binding.Namespace,
				},
			}

			Eventually(&gitopsDeployment, "60s", "1s").Should(k8s.ExistByName())

			err = k8s.Get(&gitopsDeployment)
			Expect(err).To(BeNil())

			Expect(gitopsDeployment.Spec.Destination.Environment).To(Equal("managed-environment-"+environment.Name),
				"the destination should be the environment")
			Expect(gitopsDeployment.Spec.Destination.Namespace).
				To(Equal(environment.Spec.UnstableConfigurationFields.KubernetesClusterCredentials.TargetNamespace),
					"the namespace of the GitOpsDeployment should come from the Environment")

		})

		It("Should append ASEB labels with prefix `appstudio.openshift.io` to GitopsDeployment", func() {
			By("Create ApplicationSnapshotEnvironmentBindingResource")
			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			binding.ObjectMeta.Labels = map[string]string{"appstudio.openshift.io": "testing"}
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			By("Update Status field of ApplicationSnapshotEnvironmentBindingResource")
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status = buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template", "main", "fdhyqtw", []string{"components/componentA/overlays/staging"})
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")
			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName: binding.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentName,
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeployments))

			By("Verify whether `gitopsDeployment.ObjectMeta.Labels` is updated with ASEB labels")
			gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentName,
					Namespace: binding.Namespace,
				},
			}

			err = k8s.Get(&gitopsDeployment)
			Expect(err).To(BeNil())
			Expect(gitopsDeployment.ObjectMeta.Labels).ToNot(BeNil())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(Equal(map[string]string{"appstudio.openshift.io": "testing"}))
		})

		It("Should verify if ASEB labels with prefix `appstudio.openshift.io` is not present then GitopsDeployment labels should not get updated", func() {
			By("Create ApplicationSnapshotEnvironmentBindingResource")
			binding := buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err := k8s.Create(&binding)
			Expect(err).To(Succeed())

			By("Update Status field of ApplicationSnapshotEnvironmentBindingResource")
			err = k8s.Get(&binding)
			Expect(err).To(Succeed())
			binding.Status = buildApplicationSnapshotEnvironmentBindingStatus(binding.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template", "main", "fdhyqtw", []string{"components/componentA/overlays/staging"})
			err = k8s.UpdateStatus(&binding)
			Expect(err).To(Succeed())

			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")
			gitOpsDeploymentName := appstudiocontroller.GenerateBindingGitOpsDeploymentName(binding, binding.Spec.Components[0].Name)

			expectedGitOpsDeployments := []appstudiosharedv1.BindingStatusGitOpsDeployment{{
				ComponentName: binding.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentName,
			}}

			Eventually(binding, "2m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeployments))

			By("Verify whether `gitopsDeployment.ObjectMeta.Labels` is not updated with ASEB labels")
			gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentName,
					Namespace: binding.Namespace,
				},
			}

			err = k8s.Get(&gitopsDeployment)
			Expect(err).To(BeNil())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(BeNil())
			Expect(gitopsDeployment.ObjectMeta.Labels).ToNot(Equal(map[string]string{"appstudio.openshift.io": "testing"}))
		})

	})

})

func buildApplicationSnapshotEnvironmentBindingResource(name, appName, envName, snapShotName string, replica int, componentNames []string) appstudiosharedv1.ApplicationSnapshotEnvironmentBinding {
	// Create ApplicationSnapshotEnvironmentBinding CR.
	binding := appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
			Application: appName,
			Environment: envName,
			Snapshot:    snapShotName,
		},
	}

	components := []appstudiosharedv1.BindingComponent{}
	for _, name := range componentNames {
		components = append(components, appstudiosharedv1.BindingComponent{
			Name:          name,
			Configuration: appstudiosharedv1.BindingComponentConfiguration{Replicas: replica},
		})
	}

	binding.Spec.Components = components
	return binding
}

func buildApplicationSnapshotEnvironmentBindingStatus(components []appstudiosharedv1.BindingComponent, url,
	branch, commitID string, path []string) appstudiosharedv1.ApplicationSnapshotEnvironmentBindingStatus {

	// Create ApplicationSnapshotEnvironmentBindingStatus object.
	status := appstudiosharedv1.ApplicationSnapshotEnvironmentBindingStatus{}

	componentStatus := []appstudiosharedv1.ComponentStatus{}

	for i, component := range components {
		componentStatus = append(componentStatus, appstudiosharedv1.ComponentStatus{
			Name: component.Name,
			GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
				URL: url, Branch: branch, Path: path[i], GeneratedResources: []string{}, CommitID: commitID,
			},
		})
	}

	status.Components = componentStatus
	return status
}

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
