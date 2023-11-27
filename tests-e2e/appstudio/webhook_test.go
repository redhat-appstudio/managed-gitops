package appstudio

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	environmentFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/environment"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	syncRunFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeploymentsyncrun"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	managedEnvFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/managedenvironment"
	promotionRunFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/promotionrun"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Webhook E2E tests", func() {
	// Run tests in order as next CR depends in previous CR.
	Context("validate CR Webhooks", Ordered, func() {
		var err error
		var ctx context.Context
		var k8sClient client.Client
		var snapshot appstudiosharedv1.Snapshot
		var promotionRun appstudiosharedv1.PromotionRun
		var binding appstudiosharedv1.SnapshotEnvironmentBinding

		It("Should validate Snapshot CR Webhooks.", func() {
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			if !isWebhookInstalled("snapshots", k8sClient) {
				Skip("skipping as snapshots webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			By("Create Snapshot.")
			snapshot = buildSnapshotResource("my-snapshot", "new-demo-app", "Staging Snapshot", "Staging Snapshot", "component-a", "quay.io/jgwest-redhat/sample-workload:latest")
			err = k8s.Create(&snapshot, k8sClient)
			Expect(err).To(Succeed())

			By("Validate Snapshot CR Webhooks.")

			// Fetch the latest version
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&snapshot), &snapshot)
			Expect(err).To(Succeed())

			By("Validate Spec.Application field Webhook.")

			temp := snapshot.Spec.Application // Keep old value
			snapshot.Spec.Application = "new-app-name"
			err = k8sClient.Update(ctx, &snapshot)

			Expect(err).NotTo(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("application field cannot be updated to %s", snapshot.Spec.Application))).To(BeTrue())
			snapshot.Spec.Application = temp // Revert value for next test

			By("Validate Spec.Components.Name field Webhook.")

			temp = snapshot.Spec.Components[0].Name // Keep old value
			snapshot.Spec.Components[0].Name = "new-components-name"
			err = k8sClient.Update(ctx, &snapshot)

			Expect(err).NotTo(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("components cannot be updated to %+v", snapshot.Spec.Components))).To(BeTrue())
			snapshot.Spec.Components[0].Name = temp // Revert value for next test

			By("Validate Spec.Components.ContainerImage field Webhook.")

			snapshot.Spec.Components[0].ContainerImage = "new-containerImage-name"
			err = k8sClient.Update(ctx, &snapshot)

			Expect(err).NotTo(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("components cannot be updated to %+v", snapshot.Spec.Components))).To(BeTrue())

		})

		It("Should validate SnapshotEnvironmentBinding CR Webhooks.", func() {

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			if !isWebhookInstalled("snapshotenvironmentbindings", k8sClient) {
				Skip("skipping as snapshotenvironmentbindings webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			By("Create Application.")

			application := buildApplication("new-demo-app", fixture.GitOpsServiceE2ENamespace, fixture.RepoURL)
			err = k8s.Create(&application, k8sClient)
			Expect(err).To(Succeed())

			By("Create SnapshotEnvironmentBindings")

			binding = buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			By("Validate SnapshotEnvironmentBinding CR Webhooks.")

			// Fetch the latest version
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&binding), &binding)
			Expect(err).To(Succeed())

			By("Validate Spec.Application field Webhook.")

			temp := binding.Spec.Application // Keep old value
			binding.Spec.Application = "new-app-name"
			err = k8sClient.Update(ctx, &binding)

			Expect(err).NotTo(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("application field cannot be updated to %s", binding.Spec.Application))).To(BeTrue())
			binding.Spec.Application = temp // Revert value for next test

			By("Validate Spec.Environment field Webhook.")

			temp = binding.Spec.Environment // Keep old value
			binding.Spec.Environment = "new-env-name"
			err = k8sClient.Update(ctx, &binding)

			Expect(err).NotTo(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("environment field cannot be updated to %s", binding.Spec.Environment))).To(BeTrue())
			binding.Spec.Environment = temp // Revert value for next test
		})

		It("Should validate SnapshotEnvironmentBinding CR Webhooks for duplicate combination of Application and Environment", func() {
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			if !isWebhookInstalled("snapshots", k8sClient) {
				Skip("skipping as snapshots webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			By("Create Application.")
			application := buildApplication("new-demo-app", fixture.GitOpsServiceE2ENamespace, fixture.RepoURL)
			err = k8s.Create(&application, k8sClient)
			Expect(err).To(Succeed())

			By("Create SnapshotEnvironmentBindingResource.")
			binding = buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding, k8sClient)
			Expect(err).To(Succeed())

			By("Create SnapshotEnvironmentBindingResource with same Application and Environment as above.")
			binding2 := buildSnapshotEnvironmentBindingResource("appa-staging-binding-1", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&binding2, k8sClient)
			Expect(err).ToNot(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("duplicate combination of Application (%s) and Environment (%s). Duplicated by: %s", binding2.Spec.Application, binding2.Spec.Environment, binding.Name))).To(BeTrue())
		})

		It("Should validate PromotionRun CR Webhooks.", func() {

			if !isWebhookInstalled("promotionruns", k8sClient) {
				Skip("skipping as promotionruns webhook is not installed")
			}

			By("Create PromotionRun CR.")

			promotionRun = promotionRunFixture.BuildPromotionRunResource("new-demo-app-manual-promotion", "new-demo-app", "my-snapshot", "prod")
			err = k8s.Create(&promotionRun, k8sClient)
			Expect(err).To(Succeed())

			By("Validate PromotionRun CR Webhooks.")

			// Fetch the latest version
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&promotionRun), &promotionRun)
			Expect(err).To(Succeed())

			By("Validate Spec.Application field Webhook.")

			temp := promotionRun.Spec.Application // Keep old value
			promotionRun.Spec.Application = "new-app-name"
			err = k8sClient.Update(ctx, &promotionRun)

			Expect(err).NotTo(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("spec cannot be updated to %+v", promotionRun.Spec))).To(BeTrue())
			promotionRun.Spec.Application = temp // Revert value for next test

			By("Validate Spec.Snapshot field Webhook.")

			temp = promotionRun.Spec.Snapshot // Keep old value
			promotionRun.Spec.Snapshot = "new-snapshot-name"
			err = k8sClient.Update(ctx, &promotionRun)

			Expect(err).NotTo(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("spec cannot be updated to %+v", promotionRun.Spec))).To(BeTrue())
			promotionRun.Spec.Snapshot = temp // Revert value for next test

			By("Validate Spec.ManualPromotion field Webhook.")

			temp = promotionRun.Spec.ManualPromotion.TargetEnvironment // Keep old value
			promotionRun.Spec.ManualPromotion.TargetEnvironment = "new-env-name"
			err = k8sClient.Update(ctx, &promotionRun)

			Expect(err).NotTo(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("spec cannot be updated to %+v", promotionRun.Spec))).To(BeTrue())
			promotionRun.Spec.ManualPromotion.TargetEnvironment = temp // Revert value for next test

			By("Validate Spec.AutomatedPromotion field Webhook.")

			promotionRun.Spec.AutomatedPromotion.InitialEnvironment = "new-env-name"
			err = k8sClient.Update(ctx, &promotionRun)

			Expect(err).NotTo(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("spec cannot be updated to %+v", promotionRun.Spec))).To(BeTrue())
		})

		It("Should validate Environment CR Webhooks.", func() {
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			if !isWebhookInstalled("environments", k8sClient) {
				Skip("skipping as environments webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("Validate that Environment name longer than 63 char is not allowed.")

			environment := buildEnvironmentResource(strings.Repeat("abcde", 13), "my-environment", "", "")
			err = k8s.Create(&environment, k8sClient)
			Expect(err).NotTo(Succeed())
			Expect(strings.Contains(err.Error(), fmt.Sprintf("invalid environment name: %s", environment.Name))).To(BeTrue())

		})

		It("Should validate Environment CR Webhooks for invalid KubernetesClusterCredentials API URL.", func() {
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			if !isWebhookInstalled("environments", k8sClient) {
				Skip("skipping as environments webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			kubeConfigContents, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).ToNot(HaveOccurred())

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env-secret",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Type:       sharedutil.ManagedEnvironmentSecretType,
				StringData: map[string]string{"kubeconfig": kubeConfigContents},
			}

			err = k8s.Create(secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("create environment with invalid kubernetesClusterCredentials API URL.")
			environment := buildEnvironmentResource("staging", "my-environment", "", appstudiosharedv1.EnvironmentType_POC)
			environment.Spec.Target = &appstudiosharedv1.TargetConfiguration{
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					TargetNamespace:            fixture.GitOpsServiceE2ENamespace,
					APIURL:                     "api/test-url.com:6443",
					ClusterCredentialsSecret:   secret.Name,
					AllowInsecureSkipTLSVerify: true,
				}}

			err = k8s.Create(&environment, k8sClient)
			Expect(err).ToNot(Succeed())
			Expect(strings.Contains(err.Error(), "API URL must be an absolute URL starting with an 'https' scheme")).To(BeTrue())

			By("create environment with valid kubernetesClusterCredentials API URL.")
			environment.Spec.Target = &appstudiosharedv1.TargetConfiguration{
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					TargetNamespace:            fixture.GitOpsServiceE2ENamespace,
					APIURL:                     apiServerURL,
					ClusterCredentialsSecret:   secret.Name,
					AllowInsecureSkipTLSVerify: true,
				},
			}

			err = k8s.Create(&environment, k8sClient)
			Expect(err).To(Succeed())

			By("verify that Environment's status condition is nil, indicating no errors")
			Consistently(environment, 20*time.Second, 1*time.Second).Should(environmentFixture.HaveEmptyEnvironmentConditions())

			By("fetch latest version of environment")
			err = k8s.Get(&environment, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("update environment with invalid kubernetesClusterCredentials API URL.")
			environment.Spec.Target = &appstudiosharedv1.TargetConfiguration{
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					TargetNamespace:            fixture.GitOpsServiceE2ENamespace,
					APIURL:                     "api/test-url.com:6443",
					ClusterCredentialsSecret:   secret.Name,
					AllowInsecureSkipTLSVerify: true,
				},
			}

			err = k8s.Update(&environment, k8sClient)
			Expect(err).ToNot(Succeed())
			Expect(strings.Contains(err.Error(), "API URL must be an absolute URL starting with an 'https' scheme")).To(BeTrue())
		})

		It("Should validate create GitOpsDeployment CR Webhooks for invalid .spec.Type field.", func() {

			if !isWebhookInstalled("gitopsdeployments", k8sClient) {
				Skip("skipping as gitopsdeployments webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource(fixture.GitopsDeploymentName,
				fixture.RepoURL, fixture.GitopsDeploymentPath,
				"unknown")
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).NotTo(Succeed())
			Expect(err.Error()).Should(ContainSubstring("spec type must be manual or automated"))
		})

		It("Should validate create GitOpsDeployment CR Webhooks for invalid syncOptions.", func() {
			if !isWebhookInstalled("gitopsdeployments", k8sClient) {
				Skip("skipping as gitopsdeployments webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource(fixture.GitopsDeploymentName,
				fixture.RepoURL, fixture.GitopsDeploymentPath,
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			gitOpsDeploymentResource.Spec.SyncPolicy = &managedgitopsv1alpha1.SyncPolicy{
				SyncOptions: managedgitopsv1alpha1.SyncOptions{
					"CreateNamespace=foo",
				},
			}
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).NotTo(Succeed())
			Expect(err.Error()).Should(ContainSubstring("the specified sync option in .spec.syncPolicy.syncOptions is either mispelled or is not supported by GitOpsDeployment"))
		})

		It("Should validate create GitOpsDeployment CR Webhooks for empty Environment field with non-empty namespace", func() {
			if !isWebhookInstalled("gitopsdeployments", k8sClient) {
				Skip("skipping as gitopsdeployments webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource(fixture.GitopsDeploymentName,
				fixture.RepoURL, fixture.GitopsDeploymentPath,
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			gitOpsDeploymentResource.Spec.Destination.Environment = ""
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).NotTo(Succeed())
			Expect(err.Error()).Should(ContainSubstring("the environment field should not be empty when the namespace is non-empty"))
		})

		It("Should validate update GitOpsDeployment CR Webhooks for invalid syncOptions.", func() {

			if !isWebhookInstalled("gitopsdeployments", k8sClient) {
				Skip("skipping as gitopsdeployments webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource(fixture.GitopsDeploymentName,
				fixture.RepoURL, fixture.GitopsDeploymentPath,
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			gitOpsDeploymentResource.Spec.SyncPolicy = &managedgitopsv1alpha1.SyncPolicy{
				SyncOptions: managedgitopsv1alpha1.SyncOptions{
					managedgitopsv1alpha1.SyncOptions_CreateNamespace_true,
				},
			}

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitOpsDeploymentResource), &gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			gitOpsDeploymentResource.Spec.SyncPolicy = &managedgitopsv1alpha1.SyncPolicy{
				SyncOptions: managedgitopsv1alpha1.SyncOptions{
					"CreateNamespace=foo",
				},
			}

			err = k8s.Update(&gitOpsDeploymentResource, k8sClient)
			Expect(err).NotTo(Succeed())
			Expect(err.Error()).Should(ContainSubstring("the specified sync option in .spec.syncPolicy.syncOptions is either mispelled or is not supported by GitOpsDeployment"))

		})

		It("Should validate update GitOpsDeployment CR Webhooks for empty Environment field with non-empty namespace.", func() {

			if !isWebhookInstalled("gitopsdeployments", k8sClient) {
				Skip("skipping as gitopsdeployments webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			kubeConfigContents, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).ToNot(HaveOccurred())

			managedEnv, secret := managedEnvFixture.BuildManagedEnvironment(apiServerURL, kubeConfigContents, true)

			err = k8s.Create(&secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource(fixture.GitopsDeploymentName,
				fixture.RepoURL, fixture.GitopsDeploymentPath,
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			gitOpsDeploymentResource.Spec.Destination.Environment = managedEnv.Name
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitOpsDeploymentResource), &gitOpsDeploymentResource)
			Expect(err).To(Succeed())

			gitOpsDeploymentResource.Spec.Destination.Environment = ""

			err = k8s.Update(&gitOpsDeploymentResource, k8sClient)
			Expect(err).NotTo(Succeed())
			Expect(err.Error()).Should(ContainSubstring("the environment field should not be empty when the namespace is non-empty"))

		})

		It("Should validate create GitOpsDeploymentManagedEnvironment CR Webhooks.", func() {

			if !isWebhookInstalled("gitopsdeploymentmanagedenvironments", k8sClient) {
				Skip("skipping as gitopsdeploymentmanagedenvironments webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL: "smtp://api-url",
				},
			}
			err = k8s.Create(&managedEnvCR, k8sClient)
			Expect(err).NotTo(Succeed())

		})

		It("Should validate update GitOpsDeploymentManagedEnvironment CR Webhooks.", func() {

			if !isWebhookInstalled("gitopsdeploymentmanagedenvironments", k8sClient) {
				Skip("skipping as gitopsdeploymentmanagedenvironments webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())
			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL: "https://api-url",
				},
			}
			err = k8s.Create(&managedEnvCR, k8sClient)
			Expect(err).To(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).To(Succeed())

			managedEnvCR.Spec.APIURL = "smtp://api-url"

			err = k8s.Update(&managedEnvCR, k8sClient)
			Expect(err).NotTo(Succeed())

		})

		It("Should validate create GitOpsDeploymentRepositoryCredential CR Webhooks.", func() {

			if !isWebhookInstalled("gitopsdeploymentrepositorycredentials", k8sClient) {
				Skip("skipping as gitopsdeploymentrepositorycredentials webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			gitOpsDeploymentRepositoryCredential := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gitopsdeploymenrepositorycredential",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
					Repository: "smtp://fakegithub.com/test/test-repository",
					Secret:     "test-secret",
				}}

			err = k8s.Create(&gitOpsDeploymentRepositoryCredential, k8sClient)
			Expect(err).NotTo(Succeed())

		})

		It("Should validate update GitOpsDeploymentRepositoryCredential CR Webhooks.", func() {

			if !isWebhookInstalled("gitopsdeploymentrepositorycredentials", k8sClient) {
				Skip("skipping as gitopsdeploymentrepositorycredentials webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			gitOpsDeploymentRepositoryCredential := managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gitopsdeploymenrepositorycredential",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
					Repository: "ssh://fakegithub.com/test/test-repository",
					Secret:     "test-secret",
				}}

			err = k8s.Create(&gitOpsDeploymentRepositoryCredential, k8sClient)
			Expect(err).To(Succeed())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&gitOpsDeploymentRepositoryCredential), &gitOpsDeploymentRepositoryCredential)
			Expect(err).To(Succeed())

			gitOpsDeploymentRepositoryCredential.Spec.Repository = "smtp://api-url"

			err = k8s.Update(&gitOpsDeploymentRepositoryCredential, k8sClient)
			Expect(err).NotTo(Succeed())

		})

		It("Should validate create GitOpsDeploymentSyncRun CR Webhooks.", func() {

			if !isWebhookInstalled("gitopsdeploymentsyncruns", k8sClient) {
				Skip("skipping as sync run webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("create a GitOpsDeployment with 'Manual' sync policy")
			gitOpsDeploymentResource := gitopsDeplFixture.BuildGitOpsDeploymentResource(fixture.GitopsDeploymentName,
				fixture.RepoURL, fixture.GitopsDeploymentPath,
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual)

			err = k8sClient.Create(ctx, &gitOpsDeploymentResource)
			Expect(err).ToNot(HaveOccurred())

			gitOpsDeploymentSyncRun := syncRunFixture.BuildGitOpsDeploymentSyncRunResource("zyxwvutsrqponmlkjihgfedcba-abcdefghijklmnoqrstuvwxyz", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err = k8s.Create(&gitOpsDeploymentSyncRun, k8sClient)
			Expect(err).NotTo(Succeed())

		})
	})
})

// isWebHook installed will check the cluster for validatingwebhooks that match the given resource
// - resource should be specified in plural form, to match the 'resource' field, for example: gitopsdeployments
func isWebhookInstalled(resourceName string, k8sClient client.Client) bool {

	var webhookList admissionv1.ValidatingWebhookConfigurationList
	err := k8sClient.List(context.Background(), &webhookList)
	Expect(err).ToNot(HaveOccurred())

	// Iterate through the struct, looking for a match in .spec.webhooks.rules.resources
	for _, validating := range webhookList.Items {

		for _, webhook := range validating.Webhooks {

			for _, rule := range webhook.Rules {

				for _, resource := range rule.Resources {

					if resource == resourceName {
						return true
					}

				}
			}
		}
	}

	return false

}
