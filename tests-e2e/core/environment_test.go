package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudioshared "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	envFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/environment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

		It("ensures that errors are set properly in Environment .Status.Conditions field if DeploymentTargetClaim is not found", func() {

			By("create Environment with non-existing DeploymentTargetClaim")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudioshared.EnvironmentSpec{
					Configuration: appstudioshared.EnvironmentConfiguration{
						Env: []appstudioshared.EnvVarPair{},
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

			By("checking the status component environment condition is true")
			Eventually(env, "2m", "1s").Should(envFixture.HaveEnvironmentCondition(
				metav1.Condition{
					Type:    "ErrorOccurred",
					Status:  metav1.ConditionTrue,
					Reason:  "ErrorOccurred",
					Message: "DeploymentTargetClaim not found while generating the desired Environment resource",
				}))
		})

		It("ensures that errors are set properly in Environment .Status.Conditions field if both cluster credentials and DeploymentTargetClaim are provided", func() {

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("create an environment with both cluster credentials and DTC specified")
			environment := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudioshared.EnvironmentSpec{
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							APIURL:                   "abc",
							ClusterCredentialsSecret: "test",
						},
					},
					Configuration: appstudioshared.EnvironmentConfiguration{
						Env: []appstudioshared.EnvVarPair{},
						Target: appstudioshared.EnvironmentTarget{
							DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
								ClaimName: "test-dtc",
							},
						},
					},
				},
			}

			err = k8s.Create(&environment, k8sClient)
			Expect(err).To(Succeed())

			By("checking the status component environment condition is true")
			Eventually(environment, "2m", "1s").Should(envFixture.HaveEnvironmentCondition(
				metav1.Condition{
					Type:    "ErrorOccurred",
					Status:  metav1.ConditionTrue,
					Reason:  "ErrorOccurred",
					Message: "Environment is invalid since it cannot have both DeploymentTargetClaim and credentials configuration set",
				}))
		})

	})
})
