package core

import (
	"context"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudioshared "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	argocdutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	appFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/application"
	dtfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttarget"
	dtcfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttargetclaim"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/managedenvironment"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("GitOpsDeployment Managed Environment E2E tests", func() {

	Context("Create a new GitOpsDeployment targeting a ManagedEnvironment", func() {

		It("should be healthy and have synced status, and resources should be deployed, when deployed with a ManagedEnv", func() {

			if fixture.IsRunningAgainstKCP() {
				Skip("Skipping this test until we support running gitops operator with KCP")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("creating the GitOpsDeploymentManagedEnvironment")

			kubeConfigContents, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).To(BeNil())

			managedEnv, secret := buildManagedEnvironment(apiServerURL, kubeConfigContents)

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			gitOpsDeploymentResource := buildGitOpsDeploymentResource("my-gitops-depl",
				"https://github.com/redhat-appstudio/managed-gitops", "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			gitOpsDeploymentResource.Spec.Destination.Environment = managedEnv.Name
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace
			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(BeNil())

			// By("Allow argocd to manage the environment")
			// Eventually(func() bool {
			// 	secretList := &corev1.SecretList{}
			// 	if err = k8s.List(secretList, dbutil.DefaultGitOpsEngineSingleInstanceNamespace); err != nil {
			// 		return false
			// 	}
			// 	GinkgoT().Log("found secret list", len(secretList.Items))
			// 	for _, secret := range secretList.Items {
			// 		if strings.Contains(secret.Name, "managed-env") {
			// 			GinkgoT().Log("found secret", secret.Name)
			// 			secret.Data["namespaces"] = []byte(strings.Join([]string{secret.Namespace, fixture.GitOpsServiceE2ENamespace}, ","))
			// 			if err = k8s.Update(&secret); err != nil {
			// 				return false
			// 			}
			// 			return true
			// 		}
			// 	}

			// 	return false
			// }, "60s", "1s").Should(BeTrue())

			By("ensuring GitOpsDeployment should have expected health and status and reconciledState")

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

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
					gitopsDeplFixture.HaveReconciledState(expectedReconciledStateField)))

			secretList := corev1.SecretList{}

			err = k8sClient.List(context.Background(), &secretList, &client.ListOptions{Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace})
			Expect(err).To(BeNil())

			dbQueries, err := db.NewSharedProductionPostgresDBQueries(false)
			Expect(err).To(BeNil())
			defer dbQueries.CloseDatabase()

			mapping := &db.APICRToDatabaseMapping{
				APIResourceType: db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentManagedEnvironment,
				APIResourceUID:  string(managedEnv.UID),
				DBRelationType:  db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
			}
			err = dbQueries.GetDatabaseMappingForAPICR(context.Background(), mapping)
			Expect(err).To(BeNil())

			argoCDClusterSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocdutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: mapping.DBRelationKey}),
					Namespace: dbutil.DefaultGitOpsEngineSingleInstanceNamespace,
				},
			}

			Expect(argoCDClusterSecret).To(k8s.ExistByName(k8sClient))

			By("ensuring the resources of the GitOps repo are successfully deployed")

			componentADepl := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "component-a", Namespace: fixture.GitOpsServiceE2ENamespace},
			}
			componentBDepl := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "component-b", Namespace: fixture.GitOpsServiceE2ENamespace},
			}
			Eventually(componentADepl, "60s", "1s").Should(k8s.ExistByName(k8sClient))
			Eventually(componentBDepl, "60s", "1s").Should(k8s.ExistByName(k8sClient))

			By("deleting the secret and managed environment")
			err = k8s.Delete(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Delete(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			Eventually(argoCDClusterSecret, "60s", "1s").ShouldNot(k8s.ExistByName(k8sClient),
				"once the ManagedEnvironment is deleted, the Argo CD cluster secret should be deleted as well.")

			app := appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocdutil.GenerateArgoCDApplicationName(string(gitOpsDeploymentResource.UID)),
					Namespace: dbutil.GetGitOpsEngineSingleInstanceNamespace(),
				},
			}
			Eventually(app, "60s", "1s").Should(appFixture.HasDestinationField(appv1alpha1.ApplicationDestination{
				Namespace: gitOpsDeploymentResource.Spec.Destination.Namespace,
				Name:      "",
			}), "the Argo CD Application resource's spec.destination field should have an empty environment field")

			By("deleting the GitOpsDeployment")

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())

		})
	})
})

var _ = Describe("Environment E2E tests", func() {

	Context("Create a new Environment and checks whether ManagedEnvironment has been created", func() {

		var (
			k8sClient          client.Client
			kubeConfigContents string
			apiServerURL       string
			secret             *corev1.Secret
		)
		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			var err error
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			kubeConfigContents, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).To(BeNil())

			By("creating managed environment Secret")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env-secret",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Type:       "managed-gitops.redhat.com/managed-environment",
				StringData: map[string]string{"kubeconfig": kubeConfigContents},
			}

			err = k8s.Create(secret, k8sClient)
			Expect(err).To(BeNil())
		})

		It("should ensure that AllowInsecureSkipTLSVerify field of Environment API is equal to AllowInsecureSkipTLSVerify field of GitOpsDeploymentManagedEnvironment", func() {
			By("creating the new 'staging' Environment")
			environment := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "staging",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudioshared.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration: appstudioshared.EnvironmentConfiguration{
						Env: []appstudioshared.EnvVarPair{},
					},
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							TargetNamespace:            fixture.GitOpsServiceE2ENamespace,
							APIURL:                     apiServerURL,
							ClusterCredentialsSecret:   secret.Name,
							AllowInsecureSkipTLSVerify: true,
						},
					},
				},
			}

			err := k8s.Create(&environment, k8sClient)
			Expect(err).To(Succeed())

			By("checks if managedEnvironment CR has been created and AllowInsecureSkipTLSVerify field is equal to AllowInsecureSkipTLSVerify field of Environment API")
			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-" + environment.Name,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}

			Eventually(managedEnvCR, "2m", "1s").Should(
				SatisfyAll(
					managedenvironment.HaveAllowInsecureSkipTLSVerify(environment.Spec.UnstableConfigurationFields.AllowInsecureSkipTLSVerify),
				),
			)

			err = k8s.Get(&environment, k8sClient)
			Expect(err).To(BeNil())

			By("update AllowInsecureSkipTLSVerify field of Environment to false and verify whether it updates the AllowInsecureSkipTLSVerify field of GitOpsDeploymentManagedEnvironment")
			environment.Spec.UnstableConfigurationFields = &appstudioshared.UnstableEnvironmentConfiguration{
				KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
					TargetNamespace:            fixture.GitOpsServiceE2ENamespace,
					APIURL:                     apiServerURL,
					ClusterCredentialsSecret:   secret.Name,
					AllowInsecureSkipTLSVerify: false,
				},
			}

			err = k8s.Update(&environment, k8sClient)
			Expect(err).To(BeNil())

			Eventually(managedEnvCR, "2m", "1s").Should(
				SatisfyAll(
					managedenvironment.HaveAllowInsecureSkipTLSVerify(environment.Spec.UnstableConfigurationFields.AllowInsecureSkipTLSVerify),
				),
			)

		})

		It("create an Environment with DeploymentTargetClaim and verify if a valid ManagedEnvironment is created", func() {

			By("create a new DeploymentTarget with the secret credentials")
			dt := appstudioshared.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dt",
					Namespace: secret.Namespace,
				},
				Spec: appstudioshared.DeploymentTargetSpec{
					DeploymentTargetClassName: "test-class",
					KubernetesClusterCredentials: appstudioshared.DeploymentTargetKubernetesClusterCredentials{
						APIURL:                   apiServerURL,
						ClusterCredentialsSecret: secret.Name,
						DefaultNamespace:         fixture.GitOpsServiceE2ENamespace,
					},
				},
			}
			err := k8s.Create(&dt, k8sClient)
			Expect(err).To(BeNil())

			By("create a DeploymentTargetClaim that can bind to the above Environment")
			dtc := appstudioshared.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dtc",
					Namespace: dt.Namespace,
				},
				Spec: appstudioshared.DeploymentTargetClaimSpec{
					TargetName:                dt.Name,
					DeploymentTargetClassName: dt.Spec.DeploymentTargetClassName,
				},
			}
			err = k8s.Create(&dtc, k8sClient)
			Expect(err).To(BeNil())

			By("verify if the DT and DTC are bound together")
			Eventually(dtc, "2m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudioshared.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudioshared.AnnBindCompleted, appstudioshared.AnnBinderValueTrue),
			))

			Eventually(dt, "2m", "1s").Should(
				dtfixture.HasStatusPhase(appstudioshared.DeploymentTargetPhase_Bound))

			By("creating a new Environment refering the above DTC")
			environment := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudioshared.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration: appstudioshared.EnvironmentConfiguration{
						Env: []appstudioshared.EnvVarPair{},
						Target: appstudioshared.EnvironmentTarget{
							DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}

			err = k8s.Create(&environment, k8sClient)
			Expect(err).To(Succeed())

			By("verify if the managed environment CR is created with the required fields")
			managedEnvCR := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-" + environment.Name,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}
			Eventually(managedEnvCR, "2m", "1s").Should(k8s.ExistByName(k8sClient))

			Expect(managedEnvCR.Spec.APIURL).To(Equal(dt.Spec.KubernetesClusterCredentials.APIURL))
			Expect(managedEnvCR.Spec.ClusterCredentialsSecret).To(Equal(dt.Spec.KubernetesClusterCredentials.ClusterCredentialsSecret))
		})

		It("should update the Managed Environment if the DeploymentTarget credential is modified", func() {
			By("create a new DeploymentTarget with the secret credentials")
			dt := appstudioshared.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dt",
					Namespace: secret.Namespace,
				},
				Spec: appstudioshared.DeploymentTargetSpec{
					DeploymentTargetClassName: "test-class",
					KubernetesClusterCredentials: appstudioshared.DeploymentTargetKubernetesClusterCredentials{
						APIURL:                   apiServerURL,
						ClusterCredentialsSecret: secret.Name,
						DefaultNamespace:         fixture.GitOpsServiceE2ENamespace,
					},
				},
			}
			err := k8s.Create(&dt, k8sClient)
			Expect(err).To(BeNil())

			By("create a DeploymentTargetClaim that can bind to the above Environment")
			dtc := appstudioshared.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dtc",
					Namespace: dt.Namespace,
				},
				Spec: appstudioshared.DeploymentTargetClaimSpec{
					TargetName:                dt.Name,
					DeploymentTargetClassName: dt.Spec.DeploymentTargetClassName,
				},
			}
			err = k8s.Create(&dtc, k8sClient)
			Expect(err).To(BeNil())

			By("verify if the DT and DTC are bound together")
			Eventually(dtc, "2m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudioshared.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudioshared.AnnBindCompleted, appstudioshared.AnnBinderValueTrue),
			))

			Eventually(dt, "2m", "1s").Should(
				dtfixture.HasStatusPhase(appstudioshared.DeploymentTargetPhase_Bound))

			By("creating a new Environment refering the above DTC")
			environment := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudioshared.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration: appstudioshared.EnvironmentConfiguration{
						Env: []appstudioshared.EnvVarPair{},
						Target: appstudioshared.EnvironmentTarget{
							DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}

			err = k8s.Create(&environment, k8sClient)
			Expect(err).To(Succeed())

			By("verify if the managed environment CR is created with the required fields")
			managedEnvCR := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-" + environment.Name,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}
			Eventually(managedEnvCR, "2m", "1s").Should(k8s.ExistByName(k8sClient))

			Expect(managedEnvCR.Spec.APIURL).To(Equal(dt.Spec.KubernetesClusterCredentials.APIURL))
			Expect(managedEnvCR.Spec.ClusterCredentialsSecret).To(Equal(dt.Spec.KubernetesClusterCredentials.ClusterCredentialsSecret))

			By("update the DeploymentTarget credential details")
			newSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-secret",
					Namespace: dt.Namespace,
				},
			}
			err = k8s.Create(&newSecret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Get(&dt, k8sClient)
			Expect(err).To(BeNil())

			dt.Spec.KubernetesClusterCredentials.APIURL = "https://new-url"
			dt.Spec.KubernetesClusterCredentials.ClusterCredentialsSecret = newSecret.Name

			err = k8s.Update(&dt, k8sClient)
			Expect(err).To(BeNil())

			By("verify if the managed environment CR is updated with the new details")
			expectedEnvSpec := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
				APIURL:                   dt.Spec.KubernetesClusterCredentials.APIURL,
				ClusterCredentialsSecret: dt.Spec.KubernetesClusterCredentials.ClusterCredentialsSecret,
			}
			Eventually(*managedEnvCR, "2m", "1s").Should(managedenvironment.HaveCredentials(expectedEnvSpec))
		})
	})
})

func buildManagedEnvironment(apiServerURL string, kubeConfigContents string) (managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, corev1.Secret) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-managed-env-secret",
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Type:       "managed-gitops.redhat.com/managed-environment",
		StringData: map[string]string{"kubeconfig": kubeConfigContents},
	}

	managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-managed-env",
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
			APIURL:                     apiServerURL,
			ClusterCredentialsSecret:   secret.Name,
			AllowInsecureSkipTLSVerify: true,
		},
	}

	return *managedEnv, *secret
}
