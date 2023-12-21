package appstudio

import (
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/managedenvironment"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appstudioshared "github.com/redhat-appstudio/application-api/api/v1alpha1"
	app "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	appstudiocontrollers "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	dtfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttarget"
	dtcfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttargetclaim"
	environmentFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/environment"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Environment Status.Conditions tests", func() {

	Context("Errors are set properly in Status.Conditions field of Environment", func() {

		var (
			k8sClient          client.Client
			kubeConfigContents string
			secret             *corev1.Secret
		)

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			var err error
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			kubeConfigContents, _, err = fixture.ExtractKubeConfigValues()
			Expect(err).ToNot(HaveOccurred())

			By("creating managed environment Secret")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env-secret",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Type:       sharedutil.ManagedEnvironmentSecretType,
				StringData: map[string]string{"kubeconfig": kubeConfigContents},
			}

			err = k8s.Create(secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())
		})

		It("ensures that errors are set properly in Environment .Status.Conditions field if DeploymentTargetClaim is not found", func() {

			By("create Environment with non-existing DeploymentTargetClaim")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudioshared.EnvironmentSpec{
					Configuration: appstudioshared.EnvironmentConfiguration{
						Env: []appstudioshared.EnvVarPair{
							{Name: "e1", Value: "v1"},
						},
						Target: appstudioshared.EnvironmentTarget{
							DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
								ClaimName: "test",
							},
						},
					},
				},
			}

			err := k8s.Create(&env, k8sClient)
			Expect(err).To(Succeed())

			By("checking the status component environment condition is correct")
			Eventually(env, "2m", "1s").Should(environmentFixture.HaveEnvironmentCondition(
				metav1.Condition{
					Type:    app.EnvironmentConditionErrorOccurred,
					Status:  metav1.ConditionTrue,
					Reason:  app.EnvironmentReasonErrorOccurred,
					Message: "DeploymentTargetClaim not found while generating the desired Environment resource",
				}))
			Eventually(env, "2m", "1s").Should(environmentFixture.HaveEnvironmentCondition(
				metav1.Condition{
					Type:    app.EnvironmentConditionReconciled,
					Status:  metav1.ConditionFalse,
					Reason:  app.EnvironmentReasonDeploymentTargetClaimNotFound,
					Message: "DeploymentTargetClaim not found while generating the desired Environment resource",
				}))
		})

		It("ensures that errors are set properly in Environment .Status.Conditions field if both cluster credentials and DeploymentTargetClaim are provided", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("create an environment with both cluster credentials and DTC specified")
			environment := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudioshared.EnvironmentSpec{
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							APIURL:                   "https://abc",
							ClusterCredentialsSecret: "test",
						},
					},
					Configuration: appstudioshared.EnvironmentConfiguration{
						Env: []appstudioshared.EnvVarPair{
							{Name: "e1", Value: "v1"},
						},
						Target: appstudioshared.EnvironmentTarget{
							DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
								ClaimName: "testdtc",
							},
						},
					},
				},
			}

			err := k8s.Create(&environment, k8sClient)
			Expect(err).To(Succeed())

			By("checking the status component environment condition is correct")
			Eventually(environment, "2m", "1s").Should(environmentFixture.HaveEnvironmentCondition(
				metav1.Condition{
					Type:    app.EnvironmentConditionErrorOccurred,
					Status:  metav1.ConditionTrue,
					Reason:  app.EnvironmentReasonErrorOccurred,
					Message: "Environment is invalid since it cannot have both DeploymentTargetClaim and credentials configuration set",
				}))
			Eventually(environment, "2m", "1s").Should(environmentFixture.HaveEnvironmentCondition(
				metav1.Condition{
					Type:    app.EnvironmentConditionReconciled,
					Status:  metav1.ConditionFalse,
					Reason:  app.EnvironmentReasonInvalid,
					Message: "Environment is invalid since it cannot have both DeploymentTargetClaim and credentials configuration set",
				}))
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

			kubeConfigContents, apiServerURL, err = fixture.ExtractKubeConfigValues()
			Expect(err).ToNot(HaveOccurred())

			By("creating managed environment Secret")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env-secret",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Type:       sharedutil.ManagedEnvironmentSecretType,
				StringData: map[string]string{"kubeconfig": kubeConfigContents},
			}

			err = k8s.Create(secret, k8sClient)
			Expect(err).ToNot(HaveOccurred())
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
						Env: []appstudioshared.EnvVarPair{
							{Name: "e1", Value: "v1"},
						},
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

			By("verify that Environment's status condition is nil, indicating no errors")
			Consistently(environment, 20*time.Second, 1*time.Second).Should(environmentFixture.HaveEmptyEnvironmentConditions())

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
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			By("verify that Environment's status condition is nil, indicating no errors")
			Consistently(environment, 20*time.Second, 1*time.Second).Should(environmentFixture.HaveEmptyEnvironmentConditions())

			Eventually(managedEnvCR, "2m", "1s").Should(
				SatisfyAll(
					managedenvironment.HaveAllowInsecureSkipTLSVerify(environment.Spec.UnstableConfigurationFields.AllowInsecureSkipTLSVerify),
				),
			)

		})

		It("should ensure the namespace and clusterResources fields of the GitOpsDeploymentManagedEnvironment copied from the same fields in the Environment API", func() {
			By("creating a new Environment")
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
						Env: []appstudioshared.EnvVarPair{
							{Name: "e1", Value: "v1"},
						},
					},
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							TargetNamespace:            fixture.GitOpsServiceE2ENamespace,
							APIURL:                     apiServerURL,
							ClusterCredentialsSecret:   secret.Name,
							AllowInsecureSkipTLSVerify: true,
							ClusterResources:           false,
							Namespaces: []string{
								"namespace-1",
								"namespace-2",
							},
						},
					},
				},
			}

			err := k8s.Create(&environment, k8sClient)
			Expect(err).To(Succeed())

			By("checking that the  GitOpsManagedEnvironment CR has been created with the namespaces and clusterResouces fields set appropriately")
			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-" + environment.Name,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}

			Eventually(managedEnvCR, "2m", "1s").Should(
				SatisfyAll(
					managedenvironment.HaveClusterResources(environment.Spec.UnstableConfigurationFields.ClusterResources),
					managedenvironment.HaveNamespaces(environment.Spec.UnstableConfigurationFields.Namespaces),
				),
			)

			err = k8s.Get(&environment, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("update the namespaces and clusterResources fields of Environment and verify that it updates the corresponding fields of GitOpsDeploymentManagedEnvironment")
			environment.Spec.UnstableConfigurationFields = &appstudioshared.UnstableEnvironmentConfiguration{
				KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
					TargetNamespace:            fixture.GitOpsServiceE2ENamespace,
					APIURL:                     apiServerURL,
					ClusterCredentialsSecret:   secret.Name,
					AllowInsecureSkipTLSVerify: true,
					ClusterResources:           true,
					Namespaces: []string{
						"namespace-1",
						"namespace-2",
						"namespace-3",
					},
				},
			}

			err = k8s.Update(&environment, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			Eventually(managedEnvCR, "2m", "1s").Should(
				SatisfyAll(
					managedenvironment.HaveClusterResources(environment.Spec.UnstableConfigurationFields.ClusterResources),
					managedenvironment.HaveNamespaces(environment.Spec.UnstableConfigurationFields.Namespaces),
				),
			)

			By("remove the namespaces field from Environment and set clusterResources to false and verify that it updates the corresponding fields of GitOpsDeploymentManagedEnvironment")
			environment.Spec.UnstableConfigurationFields = &appstudioshared.UnstableEnvironmentConfiguration{
				KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
					TargetNamespace:            fixture.GitOpsServiceE2ENamespace,
					APIURL:                     apiServerURL,
					ClusterCredentialsSecret:   secret.Name,
					AllowInsecureSkipTLSVerify: true,
					ClusterResources:           false,
					Namespaces:                 nil,
				},
			}

			err = k8s.Update(&environment, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			Eventually(managedEnvCR, "2m", "1s").Should(
				SatisfyAll(
					managedenvironment.HaveClusterResources(environment.Spec.UnstableConfigurationFields.ClusterResources),
					managedenvironment.HaveNamespaces(environment.Spec.UnstableConfigurationFields.Namespaces),
				),
			)

		})

		DescribeTable("create an Environment with DeploymentTargetClaim and verify if a valid ManagedEnvironment is created", func(defaultNamespaceFieldVal string) {
			By("create a new DeploymentTarget and DeploymentTargetClaim with the secret credentials")
			dt, dtc := dtcfixture.BuildDeploymentTargetAndDeploymentTargetClaim(kubeConfigContents, apiServerURL, secret.Name, secret.Namespace, defaultNamespaceFieldVal, fixture.DTName, fixture.DTCName, "test-class", true)
			err := k8s.Create(&dt, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Create(&dtc, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the DT and DTC are bound together")
			Eventually(dtc, "2m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudioshared.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudioshared.AnnBindCompleted, appstudioshared.AnnBinderValueTrue),
			))

			Eventually(dt, "2m", "1s").Should(
				dtfixture.HasStatusPhase(appstudioshared.DeploymentTargetPhase_Bound))

			By("verify if the DT Status has expected Conditions")

			Eventually(dt, "3m", "1s").Should(dtfixture.HaveDeploymentTargetCondition(
				metav1.Condition{
					Type:    appstudiocontrollers.DeploymentTargetConditionTypeErrorOccurred,
					Message: "",
					Status:  metav1.ConditionFalse,
					Reason:  appstudiocontrollers.DeploymentTargetReasonSuccess,
				}))

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
						Env: []appstudioshared.EnvVarPair{
							{Name: "e1", Value: "v1"},
						},
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

			By("verify that Environment's status condition is nil, indicating no errors")
			Consistently(environment, 20*time.Second, 1*time.Second).Should(environmentFixture.HaveEmptyEnvironmentConditions())

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
			Expect(managedEnvCR.Spec.AllowInsecureSkipTLSVerify).To(Equal(dt.Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify))

			// In both cases the ManagedEnv Namespaces value should match the value from DT
			if dt.Spec.KubernetesClusterCredentials.DefaultNamespace != "" {
				Expect(managedEnvCR.Spec.Namespaces).To(Equal([]string{dt.Spec.KubernetesClusterCredentials.DefaultNamespace}))
			} else {
				Expect(managedEnvCR.Spec.Namespaces).To(BeEmpty())
			}

		},
			Entry("DT default namespace field is non-empty", fixture.GitOpsServiceE2ENamespace),
			Entry("DT default namespace field is empty", ""))

		It("create an Environment with secret from SpaceRequest controller and verify if a new ManagedEnvironment secret is created", func() {
			By("create a secret of type Opaque")
			clusterSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-opaque-secret",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Type: corev1.SecretTypeOpaque,
			}
			clusterSecret.Data = secret.Data

			err := k8s.Create(&clusterSecret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("create a new DeploymentTarget and DeploymentTargetClaim with the above credential secret")
			dt, dtc := dtcfixture.BuildDeploymentTargetAndDeploymentTargetClaim(kubeConfigContents, apiServerURL, clusterSecret.Name, clusterSecret.Namespace, fixture.GitOpsServiceE2ENamespace, fixture.DTName, fixture.DTCName, "test-class", true)

			err = k8s.Create(&dt, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Create(&dtc, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the DT and DTC are bound together")
			Eventually(dtc, "2m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudioshared.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudioshared.AnnBindCompleted, appstudioshared.AnnBinderValueTrue),
			))

			Eventually(dt, "2m", "1s").Should(
				dtfixture.HasStatusPhase(appstudioshared.DeploymentTargetPhase_Bound))

			By("verify if the DT Status has expected Conditions")

			Eventually(dt, "3m", "1s").Should(dtfixture.HaveDeploymentTargetCondition(
				metav1.Condition{
					Type:    appstudiocontrollers.DeploymentTargetConditionTypeErrorOccurred,
					Message: "",
					Status:  metav1.ConditionFalse,
					Reason:  appstudiocontrollers.DeploymentTargetReasonSuccess,
				}))

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
						Env: []appstudioshared.EnvVarPair{
							{Name: "e1", Value: "v1"},
						},
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

			By("verify that Environment's status condition is nil, indicating no errors")
			Consistently(environment, 20*time.Second, 1*time.Second).Should(environmentFixture.HaveEmptyEnvironmentConditions())

			By("verify if the managed environment CR is created with the required fields")
			managedEnvCR := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-" + environment.Name,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}
			Eventually(managedEnvCR, "2m", "1s").Should(k8s.ExistByName(k8sClient))

			Expect(managedEnvCR.Spec.APIURL).To(Equal(dt.Spec.KubernetesClusterCredentials.APIURL))
			Expect(managedEnvCR.Spec.ClusterCredentialsSecret).To(Equal("managed-environment-secret-test-env"))
			Expect(managedEnvCR.Spec.AllowInsecureSkipTLSVerify).To(Equal(dt.Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify))

			By("verify if a new managed-environment secret is created")
			managedEnvSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-secret-" + environment.Name,
					Namespace: environment.Namespace,
				},
			}

			err = k8s.Get(&managedEnvSecret, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			Expect(reflect.DeepEqual(clusterSecret.Data, managedEnvSecret.Data)).To(BeTrue())

			By("update the original secret and verify if the ManagedEnvironment secret is updated")
			clusterSecret.Data["test-key"] = []byte("test-key")

			err = k8s.Update(&clusterSecret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				if err := k8s.Get(&managedEnvSecret, k8sClient); err != nil {
					GinkgoT().Log("failed to get managed environment secret", err)
					return false
				}

				return reflect.DeepEqual(clusterSecret.Data, managedEnvSecret.Data)
			}, "2m", "1s").Should(BeTrue())

			By("delete the original secret and verify if the ManagedEnvironment secret is deleted")
			err = k8s.Delete(&clusterSecret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			Eventually(&managedEnvSecret, "2m", "1s").Should(k8s.NotExist(k8sClient))
		})

		It("should update the Managed Environment if the DeploymentTarget credential is modified", func() {
			By("create a new DeploymentTarget with the secret credentials and create a DeploymentTargetClaim that can bind to the above Environment")
			dt, dtc := dtcfixture.BuildDeploymentTargetAndDeploymentTargetClaim(kubeConfigContents, apiServerURL, secret.Name, secret.Namespace, fixture.GitOpsServiceE2ENamespace, fixture.DTName, fixture.DTCName, "test-class", false)

			err := k8s.Create(&dt, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Create(&dtc, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the DT and DTC are bound together")
			Eventually(dtc, "2m", "1s").Should(SatisfyAll(
				dtcfixture.HasStatusPhase(appstudioshared.DeploymentTargetClaimPhase_Bound),
				dtcfixture.HasAnnotation(appstudioshared.AnnBindCompleted, appstudioshared.AnnBinderValueTrue),
			))

			Eventually(dt, "2m", "1s").Should(
				dtfixture.HasStatusPhase(appstudioshared.DeploymentTargetPhase_Bound))

			By("verify if the DT Status has expected Conditions")

			Eventually(dt, "3m", "1s").Should(dtfixture.HaveDeploymentTargetCondition(
				metav1.Condition{
					Type:    appstudiocontrollers.DeploymentTargetConditionTypeErrorOccurred,
					Message: "",
					Status:  metav1.ConditionFalse,
					Reason:  appstudiocontrollers.DeploymentTargetReasonSuccess,
				}))

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
						Env: []appstudioshared.EnvVarPair{
							{Name: "e1", Value: "v1"},
						},
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

			By("verify that Environment's status condition is nil, indicating no errors")
			Consistently(environment, 20*time.Second, 1*time.Second).Should(environmentFixture.HaveEmptyEnvironmentConditions())

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
				Type: sharedutil.ManagedEnvironmentSecretType,
			}
			err = k8s.Create(&newSecret, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			err = k8s.Get(&dt, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			dt.Spec.KubernetesClusterCredentials.APIURL = "https://new-url"
			dt.Spec.KubernetesClusterCredentials.ClusterCredentialsSecret = newSecret.Name

			err = k8s.Update(&dt, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the managed environment CR is updated with the new details")
			expectedEnvSpec := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
				APIURL:                   dt.Spec.KubernetesClusterCredentials.APIURL,
				ClusterCredentialsSecret: dt.Spec.KubernetesClusterCredentials.ClusterCredentialsSecret,
				Namespaces:               []string{dt.Spec.KubernetesClusterCredentials.DefaultNamespace},
			}
			Eventually(*managedEnvCR, "2m", "1s").Should(managedenvironment.HaveCredentials(expectedEnvSpec))
		})

		It("should verify Deletion of GitOpsDeploymentManagedEnvironment on Non-existent Environment", func() {
			By("creates a GitOpsDeploymentManagedEnvironment with an ownerref to an Environment that doesn't exist")

			kubeConfigContents, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).ToNot(HaveOccurred())

			managedEnv, _ := managedenvironment.BuildManagedEnvironment(apiServerURL, kubeConfigContents, true)

			fakeEnvName := "name"

			managedEnv.Name = "managed-environment-" + fakeEnvName
			managedEnv.OwnerReferences = []metav1.OwnerReference{
				{
					Kind:       "Environment",
					Name:       fakeEnvName,
					APIVersion: managedgitopsv1alpha1.GroupVersion.Group + "/" + managedgitopsv1alpha1.GroupVersion.Version,
					UID:        "123",
				},
			}

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			Eventually(&managedEnv, "60s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

		})
	})
})
