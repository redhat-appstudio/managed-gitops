package appstudio

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	appstudiocontroller "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	bindingFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/binding"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	promotionRunFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/promotionrun"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Application Promotion Run E2E Tests.", func() {
	Context("Testing Application Promotion Run Reconciler.", func() {
		var environmentProd appstudiosharedv1.Environment
		var bindingStage appstudiosharedv1.SnapshotEnvironmentBinding
		var bindingProd appstudiosharedv1.SnapshotEnvironmentBinding
		var application appstudiosharedv1.Application
		var promotionRun appstudiosharedv1.PromotionRun

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			// Staging environment must be in a different namespace from the production environment, else we get a
			// problem with the same resource being owned by two different applications.
			// See Jira issue https://issues.redhat.com/browse/GITOPSRVCE-544
			// To do this, we need to create the namespace and also a managed environment secret

			const serviceAccountName = "gitops-promotion-run-test-service-account"
			const secondNamespace = "new-e2e-test-namespace2"

			By("creating another namespace for one of the environments")
			clientconfig, err := fixture.GetSystemKubeConfig()
			Expect(err).ShouldNot(HaveOccurred())
			err = fixture.EnsureDestinationNamespaceExists(secondNamespace, dbutil.GetGitOpsEngineSingleInstanceNamespace(), clientconfig)
			Expect(err).ShouldNot(HaveOccurred())

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("creating a ServiceAccount and a Secret")
			serviceAccount := corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}
			err = k8s.Create(&serviceAccount, k8sClient)
			Expect(err).To(Succeed())

			// Now create the cluster role and cluster role binding
			err = k8s.CreateOrUpdateClusterRoleAndRoleBinding(context.Background(), "123", k8sClient, serviceAccountName, serviceAccount.Namespace, k8s.ArgoCDManagerNamespacePolicyRules)
			Expect(err).ToNot(HaveOccurred())

			// Create the bearer token for the service account
			tokenSecret, err := k8s.CreateServiceAccountBearerToken(context.Background(), k8sClient, serviceAccount.Name, serviceAccount.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(tokenSecret).NotTo(BeNil())

			_, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).ToNot(HaveOccurred())

			// We actually need a managed environment secret containing a kubeconfig that has the bearer token
			kubeConfigContents := k8s.GenerateKubeConfig(apiServerURL, fixture.GitOpsServiceE2ENamespace, tokenSecret)
			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env-secret",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Type:       "managed-gitops.redhat.com/managed-environment",
				StringData: map[string]string{"kubeconfig": kubeConfigContents},
			}
			err = k8s.Create(&secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("Create Staging Environment.")
			environmentStage := buildEnvironmentResource("staging", "Staging Environment", "staging", appstudiosharedv1.EnvironmentType_POC)

			// Staging environment must be in a different namespace from the production environment, else we get a
			// problem with the same resource being owned by two different applications.
			// See Jira issue https://issues.redhat.com/browse/GITOPSRVCE-544
			environmentStage.Spec.Target = &appstudiosharedv1.TargetConfiguration{
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					TargetNamespace:            secondNamespace,
					APIURL:                     apiServerURL,
					ClusterCredentialsSecret:   secret.Name,
					AllowInsecureSkipTLSVerify: true,
				},
			}
			err = k8s.Create(&environmentStage, k8sClient)
			Expect(err).To(Succeed())

			By("Create Production Environment.")
			environmentProd = buildEnvironmentResource("prod", "Production Environment", "prod", appstudiosharedv1.EnvironmentType_POC)
			err = k8s.Create(&environmentProd, k8sClient)
			Expect(err).To(Succeed())

			By("Create Application.")
			application = buildApplication("new-demo-app", fixture.GitOpsServiceE2ENamespace, fixture.RepoURL)
			err = k8s.Create(&application, k8sClient)
			Expect(err).To(Succeed())

			By("Create Snapshot.")
			snapshot := buildSnapshotResource("my-snapshot", "new-demo-app", "Staging Snapshot", "Staging Snapshot", "component-a", "quay.io/jgwest-redhat/sample-workload:latest")
			err = k8s.Create(&snapshot, k8sClient)
			Expect(err).To(Succeed())

			By("Create Staging Binding.")
			bindingStage = buildSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&bindingStage, k8sClient)
			Expect(err).To(Succeed())

			By("Update Status field.")
			gitOpsDeploymentNameStage := appstudiocontroller.GenerateBindingGitOpsDeploymentName(bindingStage, bindingStage.Spec.Components[0].Name)
			// don't care about the deployment status
			err = buildAndUpdateBindingStatus(bindingStage.Spec.Components,
				"https://github.com/redhat-appstudio/managed-gitops", "main", "fdhyqtw",
				[]string{"resources/test-data/component-based-gitops-repository-no-route/components/componentA/overlays/staging", "resources/test-data/component-based-gitops-repository-no-route/components/componentB/overlays/staging"}, &bindingStage)
			Expect(err).To(Succeed())

			By("Create Production Binding.")
			bindingProd = buildSnapshotEnvironmentBindingResource("appa-prod-binding", "new-demo-app", "prod",
				"my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&bindingProd, k8sClient)
			Expect(err).To(Succeed())

			By("Update Status field.")
			gitOpsDeploymentNameProd := appstudiocontroller.GenerateBindingGitOpsDeploymentName(bindingProd, bindingProd.Spec.Components[0].Name)
			// don't care about the deployment status
			err = buildAndUpdateBindingStatus(bindingProd.Spec.Components,
				"https://github.com/redhat-appstudio/managed-gitops", "main", "fdhyqtw",
				[]string{"resources/test-data/component-based-gitops-repository-no-route/components/componentA/overlays/staging", "resources/test-data/component-based-gitops-repository-no-route/components/componentB/overlays/staging"}, &bindingProd)
			Expect(err).To(Succeed())

			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")
			expectedGitOpsDeploymentsStage := []appstudiosharedv1.BindingStatusGitOpsDeployment{
				{
					ComponentName:                bindingStage.Spec.Components[0].Name,
					GitOpsDeployment:             gitOpsDeploymentNameStage,
					GitOpsDeploymentSyncStatus:   string(v1alpha1.SyncStatusCodeSynced),
					GitOpsDeploymentHealthStatus: string(v1alpha1.HeathStatusCodeHealthy),
					GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
				},
			}
			Eventually(bindingStage, "3m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeploymentsStage))
			expectedGitOpsDeploymentsProd := []appstudiosharedv1.BindingStatusGitOpsDeployment{
				{
					ComponentName:                bindingProd.Spec.Components[0].Name,
					GitOpsDeployment:             gitOpsDeploymentNameProd,
					GitOpsDeploymentSyncStatus:   string(v1alpha1.SyncStatusCodeSynced),
					GitOpsDeploymentHealthStatus: string(v1alpha1.HeathStatusCodeHealthy),
					GitOpsDeploymentCommitID:     "CurrentlyIDIsUnknownInTestcase",
				},
			}
			Eventually(bindingProd, "3m", "1s").Should(bindingFixture.HaveGitOpsDeploymentsWithStatusProperties(expectedGitOpsDeploymentsProd))

			By("Verify that GitOpsDeployments are created.")
			gitOpsDeploymentStage := v1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentNameStage,
					Namespace: bindingStage.Namespace,
				},
			}
			err = k8s.Get(&gitOpsDeploymentStage, k8sClient)
			Expect(err).To(Succeed())

			gitOpsDeploymentProd := v1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentNameProd,
					Namespace: bindingProd.Namespace,
				},
			}
			err = k8s.Get(&gitOpsDeploymentProd, k8sClient)
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			promotionRun = promotionRunFixture.BuildPromotionRunResource("new-demo-app-manual-promotion", "new-demo-app", "my-snapshot", "prod")
		})

		It("Should create GitOpsDeployments and it should be Synced/Healthy.", func() {
			// ToDo: https://issues.redhat.com/browse/GITOPSRVCE-234

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			err = k8s.Create(&promotionRun, k8sClient)
			Expect(err).To(Succeed())

			now := metav1.Now()
			expectedPromotionRunStatus := appstudiosharedv1.PromotionRunStatus{
				State:            appstudiosharedv1.PromotionRunState_Complete,
				CompletionResult: appstudiosharedv1.PromotionRunCompleteResult_Success,
				ActiveBindings:   []string{bindingProd.Name},
				EnvironmentStatus: []appstudiosharedv1.PromotionRunEnvironmentStatus{
					{
						Step:            1,
						EnvironmentName: environmentProd.Name,
						Status:          appstudiosharedv1.PromotionRunEnvironmentStatus_Success,
						DisplayStatus:   appstudiocontroller.StatusMessageAllGitOpsDeploymentsAreSyncedHealthy,
					},
				},
				Conditions: []appstudiosharedv1.PromotionRunCondition{
					{
						Type:               appstudiosharedv1.PromotionRunConditionErrorOccurred,
						Message:            "",
						LastProbeTime:      now,
						LastTransitionTime: &now,
						Status:             appstudiosharedv1.PromotionRunConditionStatusFalse,
						Reason:             "",
					},
				},
			}

			Eventually(promotionRun, "3m", "1s").Should(promotionRunFixture.HaveStatusComplete(expectedPromotionRunStatus))
		})

		It("Should not support Auto Promotion.", func() {

			By("Create PromotionRun CR.")
			promotionRun.Spec.ManualPromotion = appstudiosharedv1.ManualPromotionConfiguration{}
			promotionRun.Spec.AutomatedPromotion = appstudiosharedv1.AutomatedPromotionConfiguration{
				InitialEnvironment: "staging",
			}

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&promotionRun, k8sClient)
			Expect(err).To(Succeed())

			expectedPromotionRunStatusConditions := appstudiosharedv1.PromotionRunStatus{
				Conditions: []appstudiosharedv1.PromotionRunCondition{
					{
						Type:    appstudiosharedv1.PromotionRunConditionErrorOccurred,
						Message: appstudiocontroller.ErrMessageAutomatedPromotionNotSupported,
						Status:  appstudiosharedv1.PromotionRunConditionStatusTrue,
						Reason:  appstudiosharedv1.PromotionRunReasonErrorOccurred,
					},
				},
			}

			Eventually(promotionRun, "3m", "1s").Should(promotionRunFixture.HaveStatusConditions(expectedPromotionRunStatusConditions))
		})

		It("Should not support invalid value for Target Environment.", func() {

			By("Create PromotionRun CR.")
			promotionRun.Spec.ManualPromotion.TargetEnvironment = ""

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&promotionRun, k8sClient)
			Expect(err).To(Succeed())

			expectedPromotionRunStatusConditions := appstudiosharedv1.PromotionRunStatus{
				Conditions: []appstudiosharedv1.PromotionRunCondition{
					{
						Type:    appstudiosharedv1.PromotionRunConditionErrorOccurred,
						Message: appstudiocontroller.ErrMessageTargetEnvironmentHasInvalidValue,
						Status:  appstudiosharedv1.PromotionRunConditionStatusTrue,
						Reason:  appstudiosharedv1.PromotionRunReasonErrorOccurred,
					},
				},
			}

			Eventually(promotionRun, "3m", "1s").Should(promotionRunFixture.HaveStatusConditions(expectedPromotionRunStatusConditions))
		})
	})
})

func buildEnvironmentResource(name, displayName, parentEnvironment string, envType appstudiosharedv1.EnvironmentType) appstudiosharedv1.Environment {
	environment := appstudiosharedv1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.EnvironmentSpec{
			DisplayName:        displayName,
			DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
			ParentEnvironment:  parentEnvironment,
			Tags:               []string{name},
			Configuration: appstudiosharedv1.EnvironmentConfiguration{
				Env: []appstudiosharedv1.EnvVarPair{
					{Name: "e1", Value: "v1"},
				},
			},
		},
	}

	return environment
}

func buildSnapshotResource(name, appName, displayName, displayDescription, componentName, containerImage string) appstudiosharedv1.Snapshot {
	snapshot := appstudiosharedv1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.SnapshotSpec{
			Application:        appName,
			DisplayName:        displayName,
			DisplayDescription: displayDescription,
			Components: []appstudiosharedv1.SnapshotComponent{
				{
					Name:           componentName,
					ContainerImage: containerImage,
				},
			},
		},
	}
	return snapshot
}
