package appstudio

import (
	"context"
	"reflect"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	appstudiosharedv1beta1 "github.com/redhat-appstudio/application-api/api/v1beta1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Webhook E2E tests", func() {

	Context("validate CR Webhooks", func() {
		var err error
		var k8sClient client.Client
		var environmentV1 appstudiosharedv1.Environment

		BeforeEach(func() {
			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			if !isMutatingWebhookWebhookInstalled("environments", k8sClient) {
				Skip("skipping as environments webhook is not installed")
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			environmentV1 = appstudiosharedv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl-" + uuid.New().String(),
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: appstudiosharedv1.EnvironmentSpec{
					Type:               appstudiosharedv1.EnvironmentType_POC,
					DisplayName:        "My GitOps Deployment-" + uuid.New().String(),
					DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "dev-" + uuid.New().String(),
					Tags:               []string{"test-tag-1-" + uuid.New().String(), "test-tag-2-" + uuid.New().String()},
					Configuration: appstudiosharedv1.EnvironmentConfiguration{
						Env: []appstudiosharedv1.EnvVarPair{
							{Name: "env-key-1-" + uuid.New().String(), Value: "env-value-1-" + uuid.New().String()},
							{Name: "env-key-2-" + uuid.New().String(), Value: "env-value-2-" + uuid.New().String()},
						},
					},
				},
			}
		})

		It("Should validate Environment CR Conversion Webhooks havinf all fields.", func() {

			By("Create Environment CR of v1alpha1 version.")

			environmentV1.Spec.Configuration.Target = appstudiosharedv1.EnvironmentTarget{
				DeploymentTargetClaim: appstudiosharedv1.DeploymentTargetClaimConfig{
					ClaimName: "test-claim-" + uuid.New().String(),
				},
			}

			environmentV1.Spec.UnstableConfigurationFields = &appstudiosharedv1.UnstableEnvironmentConfiguration{
				ClusterType: appstudiosharedv1.ConfigurationClusterType_OpenShift,
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					TargetNamespace:            "test-namespace-" + uuid.New().String(),
					APIURL:                     "https://api.com:6443",
					IngressDomain:              "test-domain-" + uuid.New().String(),
					ClusterCredentialsSecret:   "test-secret-" + uuid.New().String(),
					AllowInsecureSkipTLSVerify: true,
					Namespaces:                 []string{"namespace-1-" + uuid.New().String(), "namespace-2-" + uuid.New().String()},
					ClusterResources:           true,
				},
			}

			err = k8s.Create(&environmentV1, k8sClient)
			Expect(err).To(Succeed())

			By("Verify that CR with v1alpha1 version is converted to v1beta1 version.")

			environmentV2 := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      environmentV1.Name,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}

			// Fetch v1beta1 version CR
			err = k8s.Get(&environmentV2, k8sClient)
			Expect(err).To(BeNil())

			// Check API version is vebeta1
			Expect(reflect.TypeOf(environmentV2) == reflect.TypeOf(appstudiosharedv1beta1.Environment{})).To(BeTrue())

			// Check spec field is having same values as v1alpha1
			Expect(environmentV2.Spec).To(Equal(
				appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        environmentV1.Spec.DisplayName,
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  environmentV1.Spec.ParentEnvironment,
					Tags:               environmentV1.Spec.Tags,
					Configuration: appstudiosharedv1beta1.EnvironmentConfiguration{
						Env: []appstudiosharedv1beta1.EnvVarPair{
							{Name: environmentV1.Spec.Configuration.Env[0].Name, Value: environmentV1.Spec.Configuration.Env[0].Value},
							{Name: environmentV1.Spec.Configuration.Env[1].Name, Value: environmentV1.Spec.Configuration.Env[1].Value},
						},
					},
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						ClusterType: appstudiosharedv1beta1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
							TargetNamespace:            environmentV1.Spec.UnstableConfigurationFields.TargetNamespace,
							APIURL:                     environmentV1.Spec.UnstableConfigurationFields.APIURL,
							IngressDomain:              environmentV1.Spec.UnstableConfigurationFields.IngressDomain,
							ClusterCredentialsSecret:   environmentV1.Spec.UnstableConfigurationFields.ClusterCredentialsSecret,
							AllowInsecureSkipTLSVerify: true,
							Namespaces:                 environmentV1.Spec.UnstableConfigurationFields.Namespaces,
							ClusterResources:           true,
						},

						Claim: appstudiosharedv1beta1.TargetClaim{
							DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
								ClaimName: environmentV1.Spec.Configuration.Target.DeploymentTargetClaim.ClaimName,
							},
						},
					},
				},
			))
		})

		It("Should validate Environment CR Conversion Webhooks with missing fields (test 1).", func() {

			By("Create Environment CR of v1alpha1 version.")

			environmentV1.Spec.UnstableConfigurationFields = &appstudiosharedv1.UnstableEnvironmentConfiguration{
				ClusterType: appstudiosharedv1.ConfigurationClusterType_OpenShift,
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					TargetNamespace:            "test-namespace-" + uuid.New().String(),
					APIURL:                     "https://api.com:6443",
					IngressDomain:              "test-domain-" + uuid.New().String(),
					ClusterCredentialsSecret:   "test-secret-" + uuid.New().String(),
					AllowInsecureSkipTLSVerify: true,
					Namespaces:                 []string{"namespace-1-" + uuid.New().String(), "namespace-2-" + uuid.New().String()},
					ClusterResources:           true,
				},
			}

			By("v1alpha1 version Environment CR should be created without any error.")

			err = k8s.Create(&environmentV1, k8sClient)
			Expect(err).To(Succeed())

			By("Verify that CR with v1alpha1 version is converted to v1beta1 version.")

			environmentV2 := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      environmentV1.Name,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}

			// Fetch v1beta1 version CR
			err = k8s.Get(&environmentV2, k8sClient)
			Expect(err).To(BeNil())

			// Check API version is vebeta1
			Expect(reflect.TypeOf(environmentV2) == reflect.TypeOf(appstudiosharedv1beta1.Environment{})).To(BeTrue())

			// Check spec field is having same values as v1alpha1
			Expect(environmentV2.Spec).To(Equal(
				appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        environmentV1.Spec.DisplayName,
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  environmentV1.Spec.ParentEnvironment,
					Tags:               environmentV1.Spec.Tags,
					Configuration: appstudiosharedv1beta1.EnvironmentConfiguration{
						Env: []appstudiosharedv1beta1.EnvVarPair{
							{Name: environmentV1.Spec.Configuration.Env[0].Name, Value: environmentV1.Spec.Configuration.Env[0].Value},
							{Name: environmentV1.Spec.Configuration.Env[1].Name, Value: environmentV1.Spec.Configuration.Env[1].Value},
						},
					},
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						ClusterType: appstudiosharedv1beta1.ConfigurationClusterType_OpenShift,
						KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
							TargetNamespace:            environmentV1.Spec.UnstableConfigurationFields.TargetNamespace,
							APIURL:                     environmentV1.Spec.UnstableConfigurationFields.APIURL,
							IngressDomain:              environmentV1.Spec.UnstableConfigurationFields.IngressDomain,
							ClusterCredentialsSecret:   environmentV1.Spec.UnstableConfigurationFields.ClusterCredentialsSecret,
							AllowInsecureSkipTLSVerify: true,
							Namespaces:                 environmentV1.Spec.UnstableConfigurationFields.Namespaces,
							ClusterResources:           true,
						},
					},
				},
			))
			Expect(reflect.ValueOf(environmentV2.Spec.Target.Claim).IsZero()).To(BeTrue())
		})

		It("Should validate Environment CR Conversion Webhooks with missing fields (test 2).", func() {

			By("Create Environment CR of v1alpha1 version.")

			environmentV1.Spec.UnstableConfigurationFields = &appstudiosharedv1.UnstableEnvironmentConfiguration{
				ClusterType: appstudiosharedv1.ConfigurationClusterType_OpenShift,
			}

			By("v1alpha1 version Environment CR should be created without any error.")

			err = k8s.Create(&environmentV1, k8sClient)
			Expect(err).To(Succeed())

			By("Verify that CR with v1alpha1 version is converted to v1beta1 version.")

			environmentV2 := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      environmentV1.Name,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}

			// Fetch v1beta1 version CR
			err = k8s.Get(&environmentV2, k8sClient)
			Expect(err).To(BeNil())

			// Check API version is vebeta1
			Expect(reflect.TypeOf(environmentV2) == reflect.TypeOf(appstudiosharedv1beta1.Environment{})).To(BeTrue())

			// Check spec field is having same values as v1alpha1
			Expect(environmentV2.Spec).To(Equal(
				appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        environmentV1.Spec.DisplayName,
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  environmentV1.Spec.ParentEnvironment,
					Tags:               environmentV1.Spec.Tags,
					Configuration: appstudiosharedv1beta1.EnvironmentConfiguration{
						Env: []appstudiosharedv1beta1.EnvVarPair{
							{Name: environmentV1.Spec.Configuration.Env[0].Name, Value: environmentV1.Spec.Configuration.Env[0].Value},
							{Name: environmentV1.Spec.Configuration.Env[1].Name, Value: environmentV1.Spec.Configuration.Env[1].Value},
						},
					},
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						ClusterType: appstudiosharedv1beta1.ConfigurationClusterType_OpenShift,
					},
				},
			))

			Expect(reflect.ValueOf(environmentV2.Spec.Target.Claim).IsZero()).To(BeTrue())
			Expect(reflect.ValueOf(environmentV2.Spec.Target.KubernetesClusterCredentials).IsZero()).To(BeTrue())
		})

		FIt("Should validate Environment CR Conversion Webhooks with missing fields (test 3).", func() {

			By("v1alpha1 version Environment CR should be created without any error.")

			err = k8s.Create(&environmentV1, k8sClient)
			Expect(err).To(Succeed())

			By("Verify that CR with v1alpha1 version is converted to v1beta1 version.")

			environmentV2 := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      environmentV1.Name,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}

			// Fetch v1beta1 version CR
			err = k8s.Get(&environmentV2, k8sClient)
			Expect(err).To(BeNil())

			// Check API version is vebeta1
			Expect(reflect.TypeOf(environmentV2) == reflect.TypeOf(appstudiosharedv1beta1.Environment{})).To(BeTrue())

			// Check spec field is having same values as v1alpha1
			Expect(environmentV2.Spec).To(Equal(
				appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        environmentV1.Spec.DisplayName,
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  environmentV1.Spec.ParentEnvironment,
					Tags:               environmentV1.Spec.Tags,
					Configuration: appstudiosharedv1beta1.EnvironmentConfiguration{
						Env: []appstudiosharedv1beta1.EnvVarPair{
							{Name: environmentV1.Spec.Configuration.Env[0].Name, Value: environmentV1.Spec.Configuration.Env[0].Value},
							{Name: environmentV1.Spec.Configuration.Env[1].Name, Value: environmentV1.Spec.Configuration.Env[1].Value},
						},
					},
				},
			))

			Expect(environmentV2.Spec.Target).To(BeNil())
		})
	})
})

// isMutatingWebhookWebHook installed will check the cluster for mutatingWebhooks that match the given resource
// - resource should be specified in plural form, to match the 'resource' field, for example: gitopsdeployments
func isMutatingWebhookWebhookInstalled(resourceName string, k8sClient client.Client) bool {
	var webhookList admissionv1.MutatingWebhookConfigurationList
	err := k8sClient.List(context.Background(), &webhookList)
	Expect(err).To(BeNil())

	// Iterate through the struct, looking for a match in .spec.webhooks.rules.resources
	for _, mutating := range webhookList.Items {
		for _, webhook := range mutating.Webhooks {
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
