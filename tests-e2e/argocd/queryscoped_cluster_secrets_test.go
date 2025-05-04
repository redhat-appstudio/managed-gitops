package argocd

import (
	"context"
	"encoding/json"
	"strings"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	argosharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/application"
	k8s "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Argo CD Application tests", func() {
	Context("Verify that adding a query parameter to an Argo CD cluster secret will cause Argo CD to treat it as a different cluster", func() {

		argoCDNamespace := dbutil.GetGitOpsEngineSingleInstanceNamespace()

		var k8sClient client.Client

		users := []string{"user-a", "user-b"}

		// Clean up all the resources that this test creates
		cleanupTestResources := func() {
			config, err := fixture.GetSystemKubeConfig()
			Expect(err).ToNot(HaveOccurred())

			var secretList corev1.SecretList
			err = k8sClient.List(context.Background(), &secretList, &client.ListOptions{Namespace: argoCDNamespace})
			Expect(err).ToNot(HaveOccurred())

			for index := range secretList.Items {
				secret := secretList.Items[index]
				if strings.HasPrefix(secret.Name, "test-") {
					err = k8s.Delete(&secret, k8sClient)
					Expect(err).ToNot(HaveOccurred())
				}
			}

			for _, userName := range users {
				err = application.DeleteArgoCDApplication("test-"+userName, argoCDNamespace)
				Expect(err).ToNot(HaveOccurred())

				err = fixture.DeleteNamespace(userName, config)
				Expect(err).ToNot(HaveOccurred())
			}

			By("cleaning up old ClusterRole/ClusterRoleBinding")
			for _, userName := range users {
				clusterRole, clusterRoleBinding := k8s.GenerateReadOnlyClusterRoleandBinding(userName)
				err = k8sClient.Delete(context.Background(), &clusterRole)
				if err != nil && !apierr.IsNotFound(err) {
					Expect(err).ToNot(HaveOccurred())
				}

				err = k8sClient.Delete(context.Background(), &clusterRoleBinding)
				if err != nil && !apierr.IsNotFound(err) {
					Expect(err).ToNot(HaveOccurred())
				}
			}

			Expect(fixture.EnsureCleanSlate()).To(Succeed())
		}

		// For each user, create: Namespace, ServiceAccounts, Roles/RoleBindings, and optionally, ClusterRole/ClusterRoleBindings.
		// - The SAs and R/RB are scoped to only access the particular count
		// - Optionally, a CR/CRB are created to allow Argo CD to read all resources on the cluster (but with no write permissions)
		createServiceAccountsAndSecretsForTest := func(clusterScopedSecrets bool) {
			klog := log.FromContext(context.Background())

			By("creating an ServiceAccounts for each user, scoped to only deploy to their namespace, plus cluster-scoped read")
			userServiceAccountTokens := map[string]string{}

			for _, userName := range users {
				userToken, err := k8s.CreateNamespaceScopedUserAccount(context.Background(), userName, clusterScopedSecrets, k8sClient, klog)
				Expect(err).ToNot(HaveOccurred())
				userServiceAccountTokens[userName] = userToken
			}

			By("creating Argo CD Cluster secrets for those user's service accounts, scoped to deploy only to their namespace")
			for userName, userToken := range userServiceAccountTokens {

				jsonString, err := generateClusterSecretJSON(userToken)
				Expect(err).ToNot(HaveOccurred())

				_, apiURL, err := fixture.ExtractKubeConfigValues()
				Expect(err).ToNot(HaveOccurred())

				clusterResourcesField := ""
				namespacesField := ""

				if !clusterScopedSecrets {
					clusterResourcesField = "false"
					namespacesField = userName
				}

				secret := buildArgoCDClusterSecret("test-"+userName, argoCDNamespace, "test-"+userName, apiURL+"?user="+userName,
					jsonString, clusterResourcesField, namespacesField)

				Expect(secret.Data["server"]).To(ContainSubstring("?"), "the api url must contain a query parameter character, for these tests to be valid")

				err = k8sClient.Create(context.Background(), &secret)
				Expect(err).ToNot(HaveOccurred())

			}
		}

		BeforeEach(func() {
			kubeConfig, err := fixture.GetSystemKubeConfig()
			Expect(err).ToNot(HaveOccurred())

			k8sClient, err = fixture.GetKubeClient(kubeConfig)
			Expect(err).ToNot(HaveOccurred())

			cleanupTestResources()
		})

		AfterEach(func() {

			_ = fixture.ReportRemainingArgoCDApplications(k8sClient)

			cleanupTestResources()
		})

		DescribeTable("ensure users can deploy to their own namespace, using different Argo CD cluster secrets, each with a query parameter",

			func(clusterScoped bool) {

				if fixture.IsGitHubK3D() {
					Skip("when running on GitHub CI via K3d, we don't have an API endpoint IP we can use, so skip")
					return
				}

				createServiceAccountsAndSecretsForTest(clusterScoped)

				for _, userName := range users {
					By("creating an Argo CD application to deploy to the user's namespace")

					argoCDApplication := appv1alpha1.Application{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "test-" + userName,
							Namespace:  argoCDNamespace,
							Finalizers: []string{"resources-finalizer.argocd.argoproj.io"},
						},
						Spec: appv1alpha1.ApplicationSpec{
							Source: &appv1alpha1.ApplicationSource{
								RepoURL: "https://github.com/redhat-appstudio/managed-gitops/",
								Path:    "resources/test-data/sample-gitops-repository/environments/overlays/dev",
							},
							Destination: appv1alpha1.ApplicationDestination{
								Name:      "test-" + userName,
								Namespace: userName,
							},
							SyncPolicy: &appv1alpha1.SyncPolicy{
								Automated: &appv1alpha1.SyncPolicyAutomated{Prune: true, SelfHeal: true},
							},
						},
					}
					argoCDApplication.Finalizers = []string{"resources-finalizer.argocd.argoproj.io"}
					err := k8sClient.Create(context.Background(), &argoCDApplication)
					Expect(err).ToNot(HaveOccurred())

					By("ensuring GitOpsDeployment should have expected health and status")

					Eventually(argoCDApplication, "2m", "1s").Should(
						SatisfyAll(
							application.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced),
							application.HaveHealthStatusCode(health.HealthStatusHealthy)))

					By("ensuring the resources of the GitOps repo are successfully deployed")

					componentADepl := &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{Name: "component-a", Namespace: userName},
					}
					componentBDepl := &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{Name: "component-b", Namespace: userName},
					}
					Eventually(componentADepl, "60s", "1s").Should(k8s.ExistByName(k8sClient))
					Eventually(componentBDepl, "60s", "1s").Should(k8s.ExistByName(k8sClient))

					By("deleting the Argo CD Application")

					err = k8sClient.Delete(context.Background(), &argoCDApplication)
					Expect(err).ToNot(HaveOccurred())

					By("ensuring the resources of the GitOps repo are successfully deleted")

					Eventually(componentADepl, "60s", "1s").ShouldNot(k8s.ExistByName(k8sClient))
					Eventually(componentBDepl, "60s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

				}
			},
			Entry("user should be able to deploy to their namespace, when using cluster-scoped argo cd cluster secrets", true),
			Entry("user should be able to deploy to their namespace, when using namespace-scoped argo cd cluster secrets", false),
		)

		DescribeTable("ensure users cannot deploy to another user's namespace, when using different Argo CD cluster secrets for each user, each with a query parameter", func(clusterScoped bool) {

			if fixture.IsGitHubK3D() {
				Skip("when running on GitHub CI via K3d, we don't have an API endpoint IP we can use, so skip")
				return
			}

			createServiceAccountsAndSecretsForTest(clusterScoped)

			Expect(users).To(HaveLen(2))

			for idx, userName := range users {

				// we should use user-a serviceaccount to deploy to user-b namespace, and vice versa
				otherUser := users[(idx+1)%2]

				By("creating an Argo CD application to deploy to the user's namespace")

				// Attempt to deploy into a Namespace we don't have access to
				argoCDApplication := appv1alpha1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-" + userName,
						Namespace:  argoCDNamespace,
						Finalizers: []string{"resources-finalizer.argocd.argoproj.io"},
					},
					Spec: appv1alpha1.ApplicationSpec{
						Source: &appv1alpha1.ApplicationSource{
							RepoURL: "https://github.com/redhat-appstudio/managed-gitops/",
							Path:    "resources/test-data/sample-gitops-repository/environments/overlays/dev",
						},
						Destination: appv1alpha1.ApplicationDestination{
							Name:      "test-" + userName,
							Namespace: otherUser, // Attempt to deploy into a Namespace we don't have access to
						},
						SyncPolicy: &appv1alpha1.SyncPolicy{
							Automated: &appv1alpha1.SyncPolicyAutomated{Prune: true, SelfHeal: true},
						},
					},
				}
				argoCDApplication.Finalizers = []string{"resources-finalizer.argocd.argoproj.io"}
				err := k8sClient.Create(context.Background(), &argoCDApplication)
				Expect(err).ToNot(HaveOccurred())

				By("ensuring Argo CD Application should have expected health and status")

				// If Argo CD cluster secret is cluster-scoped: it can see the application is out of sync
				expectedStatusCode := appv1alpha1.SyncStatusCodeOutOfSync
				if !clusterScoped {
					// If the Argo CD application is namespaced-scoped: it cannot access the resources in the other user's namespace
					expectedStatusCode = appv1alpha1.SyncStatusCodeUnknown
				}

				Eventually(argoCDApplication, "1m", "1s").Should(
					SatisfyAll(
						application.HaveSyncStatusCode(expectedStatusCode),
						application.HaveHealthStatusCode(health.HealthStatusMissing)))

				Consistently(argoCDApplication, "30s", "1s").Should(
					SatisfyAll(
						application.HaveSyncStatusCode(expectedStatusCode),
						application.HaveHealthStatusCode(health.HealthStatusMissing)))

				By("ensuring the expected failure message is present")

				err = k8s.Get(&argoCDApplication, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				if clusterScoped {
					// If Argo CD cluster secret is cluster scoped, we should expect this message:
					Expect(argoCDApplication.Status.OperationState).ToNot(BeNil())
					Expect(argoCDApplication.Status.OperationState.Message).
						To(ContainSubstring("User \"system:serviceaccount:" + userName + ":" + userName + "-sa\" cannot create resource"))
					Expect(argoCDApplication.Status.OperationState.Message).
						To(ContainSubstring("is forbidden:"))

				} else {
					// If Argo CD cluster secret is namespace scoped, we should expect the 'is not managed' error message:
					var containsExpectedCondition bool

					for _, condition := range argoCDApplication.Status.Conditions {
						if condition.Type != "ComparisonError" {
							continue
						}

						if strings.Contains(condition.Message, "is not managed") {
							containsExpectedCondition = true
							break
						}
					}

					Expect(containsExpectedCondition).To(BeTrue())
				}

			}

		},
			Entry("user cannot deploy to another users namepace, when using cluster-scoped argo cd cluster secrets", true),
			Entry("user cannot deploy to another users namepace, when using namespace-scoped argo cd cluster secrets", false))

	})
})

func buildArgoCDClusterSecret(secretName, secretNamespace, clusterName, clusterServer, clusterConfigJSON, clusterResources, clusterNamespaces string) corev1.Secret {
	res := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: secretNamespace,
			Labels: map[string]string{
				sharedutil.ArgoCDSecretTypeIdentifierKey: sharedutil.ArgoCDSecretClusterTypeValue,
			},
		},
		Data: map[string][]byte{
			"name":   ([]byte)(clusterName),
			"server": ([]byte)(clusterServer),
			"config": ([]byte)(string(clusterConfigJSON)),
		},
	}

	if clusterResources != "" {
		res.Data["clusterResources"] = ([]byte)(clusterResources)
	}

	if clusterNamespaces != "" {
		res.Data["namespaces"] = ([]byte)(clusterNamespaces)
	}

	return res
}

func generateClusterSecretJSON(bearerToken string) (string, error) {

	clusterSecretConfigJSON := argosharedutil.ClusterSecretConfigJSON{
		BearerToken: bearerToken,
		TLSClientConfig: argosharedutil.ClusterSecretTLSClientConfigJSON{
			Insecure: true,
		},
	}

	jsonString, err := json.Marshal(clusterSecretConfigJSON)
	return string(jsonString), err

}
