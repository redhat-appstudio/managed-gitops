package core

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiocontroller "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	bindingFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/binding"
	promotionRunFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/promotionrun"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Application Promotion Run E2E Tests.", func() {
	Context("Testing Application Promotion Run Reconciler.", func() {
		var environmentProd appstudiosharedv1.Environment
		var bindingStage appstudiosharedv1.ApplicationSnapshotEnvironmentBinding
		var bindingProd appstudiosharedv1.ApplicationSnapshotEnvironmentBinding
		var promotionRun appstudiosharedv1.ApplicationPromotionRun

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("Create Staging Environment.")
			environmentStage := buildEnvironmentResource("staging", "Staging Environment", "staging", appstudiosharedv1.EnvironmentType_POC)
			err := k8s.Create(&environmentStage)
			Expect(err).To(Succeed())

			By("Create Production Environment.")
			environmentProd = buildEnvironmentResource("prod", "Production Environment", "prod", appstudiosharedv1.EnvironmentType_POC)
			err = k8s.Create(&environmentProd)
			Expect(err).To(Succeed())

			By("Create Snapshot.")
			applicationSnapshot := buildApplicationSnapshotResource("my-snapshot", "new-demo-app", "Staging Snapshot", "Staging Snapshot", "component-a", "quay.io/jgwest-redhat/sample-workload:latest")
			err = k8s.Create(&applicationSnapshot)
			Expect(err).To(Succeed())

			By("Create Staging Binding.")
			bindingStage = buildApplicationSnapshotEnvironmentBindingResource("appa-staging-binding", "new-demo-app", "staging", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&bindingStage)
			Expect(err).To(Succeed())

			By("Update Status field.")
			err = k8s.Get(&bindingStage)
			Expect(err).To(Succeed())
			bindingStage.Status = buildApplicationSnapshotEnvironmentBindingStatus(bindingStage.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template", "main", "fdhyqtw", []string{"components/componentA/overlays/staging", "components/componentB/overlays/staging"})
			err = k8s.UpdateStatus(&bindingStage)
			Expect(err).To(Succeed())

			By("Create Production Binding.")
			bindingProd = buildApplicationSnapshotEnvironmentBindingResource("appa-prod-binding", "new-demo-app", "prod", "my-snapshot", 3, []string{"component-a"})
			err = k8s.Create(&bindingProd)
			Expect(err).To(Succeed())

			By("Update Status field.")
			err = k8s.Get(&bindingProd)
			Expect(err).To(Succeed())
			bindingProd.Status = buildApplicationSnapshotEnvironmentBindingStatus(bindingProd.Spec.Components, "https://github.com/redhat-appstudio/gitops-repository-template", "main", "fdhyqtw", []string{"components/componentA/overlays/staging", "components/componentB/overlays/staging"})
			err = k8s.UpdateStatus(&bindingProd)
			Expect(err).To(Succeed())

			By("Verify that Status.GitOpsDeployments field of Binding is having Component and GitOpsDeployment name.")
			gitOpsDeploymentNameStage := appstudiocontroller.GenerateBindingGitOpsDeploymentName(bindingStage, bindingStage.Spec.Components[0].Name)
			expectedGitOpsDeploymentsStage := []appstudiosharedv1.BindingStatusGitOpsDeployment{
				{ComponentName: bindingStage.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentNameStage},
			}
			Eventually(bindingStage, "3m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeploymentsStage))

			gitOpsDeploymentNameProd := appstudiocontroller.GenerateBindingGitOpsDeploymentName(bindingProd, bindingProd.Spec.Components[0].Name)
			expectedGitOpsDeploymentsProd := []appstudiosharedv1.BindingStatusGitOpsDeployment{
				{ComponentName: bindingProd.Spec.Components[0].Name, GitOpsDeployment: gitOpsDeploymentNameProd},
			}
			Eventually(bindingProd, "3m", "1s").Should(bindingFixture.HaveStatusGitOpsDeployments(expectedGitOpsDeploymentsProd))

			By("Verify that GitOpsDeployments are created.")
			gitOpsDeploymentStage := v1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentNameStage,
					Namespace: bindingStage.Namespace,
				},
			}
			err = k8s.Get(&gitOpsDeploymentStage)
			Expect(err).To(Succeed())

			gitOpsDeploymentProd := v1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitOpsDeploymentNameProd,
					Namespace: bindingProd.Namespace,
				},
			}
			err = k8s.Get(&gitOpsDeploymentProd)
			Expect(err).To(Succeed())

			By("Create PromotionRun CR.")
			promotionRun = buildPromotionRunResource("new-demo-app-manual-promotion", "new-demo-app", "my-snapshot", "prod")
		})

		It("Should create GitOpsDeployments and it should be Synced/Healthy.", func() {
			// ToDo: https://issues.redhat.com/browse/GITOPSRVCE-234
			if fixture.IsRunningAgainstKCP() {
				Skip("Skipping this test in KCP until we fix the race condition")
			}

			// Temporarily skipping it for OpenShift CI.
			if os.Getenv("OPENSHIFT_CI") != "true" {
				By("Create PromotionRun CR.")
				err := k8s.Create(&promotionRun)
				Expect(err).To(Succeed())

				expectedPromotionRunStatus := appstudiosharedv1.ApplicationPromotionRunStatus{
					State:            appstudiosharedv1.PromotionRunState_Complete,
					CompletionResult: appstudiosharedv1.PromotionRunCompleteResult_Success,
					ActiveBindings:   []string{bindingProd.Name},
					EnvironmentStatus: []appstudiosharedv1.PromotionRunEnvironmentStatus{
						{
							Step:            1,
							EnvironmentName: environmentProd.Name,
							Status:          appstudiosharedv1.ApplicationPromotionRunEnvironmentStatus_Success,
							DisplayStatus:   appstudiocontroller.StatusMessageAllGitOpsDeploymentsAreSyncedHealthy,
						},
					},
				}

				Eventually(promotionRun, "3m", "1s").Should(promotionRunFixture.HaveStatusComplete(expectedPromotionRunStatus))
			}
		})

		It("Should not support Auto Promotion.", func() {

			By("Create PromotionRun CR.")
			promotionRun.Spec.ManualPromotion = appstudiosharedv1.ManualPromotionConfiguration{}
			promotionRun.Spec.AutomatedPromotion = appstudiosharedv1.AutomatedPromotionConfiguration{
				InitialEnvironment: "staging",
			}
			err := k8s.Create(&promotionRun)
			Expect(err).To(Succeed())

			expectedPromotionRunStatusConditions := appstudiosharedv1.ApplicationPromotionRunStatus{
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
			err := k8s.Create(&promotionRun)
			Expect(err).To(Succeed())

			expectedPromotionRunStatusConditions := appstudiosharedv1.ApplicationPromotionRunStatus{
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

		It("Should reset the Status.Conditions field if error is resolved.", func() {

			By("Create PromotionRun CR with invalid value.")
			promotionRun.Spec.ManualPromotion.TargetEnvironment = ""
			err := k8s.Create(&promotionRun)
			Expect(err).To(Succeed())

			expectedPromotionRunStatusConditions := appstudiosharedv1.ApplicationPromotionRunStatus{
				Conditions: []appstudiosharedv1.PromotionRunCondition{
					{
						Type:    appstudiosharedv1.PromotionRunConditionErrorOccurred,
						Message: appstudiocontroller.ErrMessageTargetEnvironmentHasInvalidValue,
						Status:  appstudiosharedv1.PromotionRunConditionStatusTrue,
						Reason:  appstudiosharedv1.PromotionRunReasonErrorOccurred,
					},
				},
			}

			By("Verify that error is updated in Status.conditions field.")
			Eventually(promotionRun, "3m", "1s").Should(promotionRunFixture.HaveStatusConditions(expectedPromotionRunStatusConditions))

			k8sClient, err := fixture.GetKubeClient()
			Expect(err).To(Succeed())

			err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&promotionRun), &promotionRun)
			Expect(err).To(Succeed())

			By("Update PromotionRun CR with invalid value.")
			promotionRun.Spec.ManualPromotion.TargetEnvironment = "prod"
			err = k8s.Update(&promotionRun)
			Expect(err).To(Succeed())

			expectedPromotionRunStatus := appstudiosharedv1.ApplicationPromotionRunStatus{
				State:            appstudiosharedv1.PromotionRunState_Complete,
				CompletionResult: appstudiosharedv1.PromotionRunCompleteResult_Success,
				ActiveBindings:   []string{bindingProd.Name},
				EnvironmentStatus: []appstudiosharedv1.PromotionRunEnvironmentStatus{
					{
						Step:            1,
						EnvironmentName: environmentProd.Name,
						Status:          appstudiosharedv1.ApplicationPromotionRunEnvironmentStatus_Success,
						DisplayStatus:   appstudiocontroller.StatusMessageAllGitOpsDeploymentsAreSyncedHealthy,
					},
				},
				Conditions: []appstudiosharedv1.PromotionRunCondition{
					{
						Type:    appstudiosharedv1.PromotionRunConditionErrorOccurred,
						Message: "",
						Status:  appstudiosharedv1.PromotionRunConditionStatusFalse,
						Reason:  "",
					},
				},
			}

			By("Verify that error is removed from Status.conditions field.")
			Eventually(promotionRun, "3m", "1s").Should(promotionRunFixture.HaveStatusComplete(expectedPromotionRunStatus))
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
			Type:               envType,
			DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
			ParentEnvironment:  parentEnvironment,
			Tags:               []string{name},
			Configuration: appstudiosharedv1.EnvironmentConfiguration{
				Env: []appstudiosharedv1.EnvVarPair{},
			},
		},
	}

	return environment
}

func buildApplicationSnapshotResource(name, appName, displayName, displayDescription, componentName, containerImage string) appstudiosharedv1.ApplicationSnapshot {
	applicationSnapshot := appstudiosharedv1.ApplicationSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.ApplicationSnapshotSpec{
			Application:        appName,
			DisplayName:        displayName,
			DisplayDescription: displayDescription,
			Components: []appstudiosharedv1.ApplicationSnapshotComponent{
				{
					Name:           componentName,
					ContainerImage: containerImage,
				},
			},
		},
	}
	return applicationSnapshot
}

func buildPromotionRunResource(name, appName, snapshotName, targetEnvironment string) appstudiosharedv1.ApplicationPromotionRun {

	promotionRun := appstudiosharedv1.ApplicationPromotionRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.ApplicationPromotionRunSpec{
			Snapshot:    snapshotName,
			Application: appName,
			ManualPromotion: appstudiosharedv1.ManualPromotionConfiguration{
				TargetEnvironment: targetEnvironment,
			},
		},
	}
	return promotionRun
}
