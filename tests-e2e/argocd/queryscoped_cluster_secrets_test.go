package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/go-logr/logr"
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
	rbacv1 "k8s.io/api/rbac/v1"
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
			Expect(err).To(BeNil())

			var secretList corev1.SecretList
			err = k8sClient.List(context.Background(), &secretList, &client.ListOptions{Namespace: argoCDNamespace})
			Expect(err).To(BeNil())

			for index := range secretList.Items {
				secret := secretList.Items[index]
				if strings.HasPrefix(secret.Name, "test-") {
					err = k8s.Delete(&secret, k8sClient)
					Expect(err).To(BeNil())
				}
			}

			for _, userName := range users {
				err = application.DeleteArgoCDApplication("test-"+userName, argoCDNamespace)
				Expect(err).To(BeNil())

				err = fixture.DeleteNamespace(userName, config)
				Expect(err).To(BeNil())
			}

			By("cleaning up old ClusterRole/ClusterRoleBinding")
			for _, userName := range users {
				clusterRole, clusterRoleBinding := generateReadOnlyClusterRoleandBinding(userName)
				err = k8sClient.Delete(context.Background(), &clusterRole)
				if err != nil && !apierr.IsNotFound(err) {
					Expect(err).To(BeNil())
				}

				err = k8sClient.Delete(context.Background(), &clusterRoleBinding)
				if err != nil && !apierr.IsNotFound(err) {
					Expect(err).To(BeNil())
				}
			}
		}

		// For each user, create: Namespace, ServiceAccounts, Roles/RoleBindings, and optionally, ClusterRole/ClusterRoleBindings.
		// - The SAs and R/RB are scoped to only access the particular count
		// - Optionally, a CR/CRB are created to allow Argo CD to read all resources on the cluster (but with no write permissions)
		createServiceAccountsAndSecretsForTest := func(clusterScopedSecrets bool) {
			klog := log.FromContext(context.Background())

			By("creating an ServiceAccounts for each user, scoped to only deploy to their namespace, plus cluster-scoped read")
			userServiceAccountTokens := map[string]string{}

			for _, userName := range users {
				userToken, err := createNamespaceScopedUserAccount(context.Background(), userName, clusterScopedSecrets, k8sClient, klog)
				Expect(err).To(BeNil())
				userServiceAccountTokens[userName] = userToken
			}

			By("creating Argo CD Cluster secrets for those user's service accounts, scoped to deploy only to their namespace")
			for userName, userToken := range userServiceAccountTokens {

				jsonString, err := generateClusterSecretJSON(userToken)
				Expect(err).To(BeNil())

				_, apiURL, err := fixture.ExtractKubeConfigValues()
				Expect(err).To(BeNil())

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
				Expect(err).To(BeNil())

			}
		}

		BeforeEach(func() {
			kubeConfig, err := fixture.GetSystemKubeConfig()
			Expect(err).To(BeNil())

			k8sClient, err = fixture.GetKubeClient(kubeConfig)
			Expect(err).To(BeNil())

			cleanupTestResources()
		})

		AfterEach(func() {

			_ = fixture.ReportRemainingArgoCDApplications(k8sClient)

			cleanupTestResources()
		})

		DescribeTable("ensure users can deploy to their own namespace, using different Argo CD cluster secrets, each with a query parameter",

			func(clusterScoped bool) {

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
							Source: appv1alpha1.ApplicationSource{
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
					err := k8sClient.Create(context.Background(), &argoCDApplication)
					Expect(err).To(BeNil())

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
					Expect(err).To(BeNil())

					By("ensuring the resources of the GitOps repo are successfully deleted")

					Eventually(componentADepl, "60s", "1s").ShouldNot(k8s.ExistByName(k8sClient))
					Eventually(componentBDepl, "60s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

				}

			},
			Entry("user should be able to deploy to their namespace, when using cluster-scoped argo cd cluster secrets", true),
			Entry("user should be able to deploy to their namespace, when using namespace-scoped argo cd cluster secrets", false),
		)

		DescribeTable("ensure users cannot deploy to another user's namespace, when using different Argo CD cluster secrets for each user, each with a query parameter", func(clusterScoped bool) {

			createServiceAccountsAndSecretsForTest(clusterScoped)

			Expect(len(users)).To(Equal(2))

			for idx, userName := range users {

				// we should use user-a serviceaccount to deploy to user-b namespace, and vice versa
				otherUser := users[(idx+1)%2]

				By("creating an Argo CD application to deploy to the user's namespace")

				argoCDApplication := appv1alpha1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-" + userName,
						Namespace:  argoCDNamespace,
						Finalizers: []string{"resources-finalizer.argocd.argoproj.io"},
					},
					Spec: appv1alpha1.ApplicationSpec{
						Source: appv1alpha1.ApplicationSource{
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
				err := k8sClient.Create(context.Background(), &argoCDApplication)
				Expect(err).To(BeNil())

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
				Expect(err).To(BeNil())

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

func generateReadOnlyClusterRoleandBinding(user string) (rbacv1.ClusterRole, rbacv1.ClusterRoleBinding) {

	clusterRole := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "read-all-" + user,
		},
		Rules: []rbacv1.PolicyRule{{
			Verbs:     []string{"get", "list", "watch"},
			Resources: []string{"*"},
			APIGroups: []string{"*"},
		}},
	}

	clusterRoleBinding := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "read-all-" + user,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Namespace: user,
			Name:      user + "-sa",
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRole.Name,
		},
	}

	return clusterRole, clusterRoleBinding
}

func createNamespaceScopedUserAccount(ctx context.Context, user string, createReadOnlyClusterRoleBinding bool,
	k8sClient client.Client, log logr.Logger) (string, error) {

	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: user,
		},
	}
	if err := k8sClient.Create(ctx, &ns); err != nil {
		return "", fmt.Errorf("error on creating namespace: %v", err)
	}

	saName := user + "-sa"

	sa, err := getOrCreateServiceAccount(ctx, k8sClient, saName, ns.Name, log)
	if err != nil {
		return "", fmt.Errorf("error on get/create service account: %v", err)
	}

	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-all",
			Namespace: ns.Name,
		},
		Rules: []rbacv1.PolicyRule{{
			Verbs:     []string{"*"},
			Resources: []string{"*"},
			APIGroups: []string{"*"},
		}},
	}
	if err := k8sClient.Create(ctx, &role); err != nil {
		return "", fmt.Errorf("error on creating role: %v", err)
	}

	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-all-binding",
			Namespace: ns.Name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      sa.Name,
			Namespace: sa.Namespace,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
	}
	if err := k8sClient.Create(ctx, &roleBinding); err != nil {
		return "", fmt.Errorf("error on creating rolebinding: %v", err)
	}

	if createReadOnlyClusterRoleBinding {
		clusterRole, clusterRoleBinding := generateReadOnlyClusterRoleandBinding(user)

		if err := k8sClient.Create(ctx, &clusterRole); err != nil {
			return "", fmt.Errorf("error on creating role: %v", err)
		}

		if err := k8sClient.Create(ctx, &clusterRoleBinding); err != nil {
			return "", fmt.Errorf("error on creating role: %v", err)
		}
	}

	token, err := k8s.CreateServiceAccountBearerToken(ctx, k8sClient, sa.Name, ns.Name)
	if err != nil {
		return "", fmt.Errorf("error on getting bearer token: %v", err)
	}

	return token, nil
}

func getOrCreateServiceAccount(ctx context.Context, k8sClient client.Client, serviceAccountName string, serviceAccountNS string,
	log logr.Logger) (*corev1.ServiceAccount, error) {

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: serviceAccountNS,
		},
	}

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount); err != nil {
		if !apierr.IsNotFound(err) {
			return nil, fmt.Errorf("unable to retrieve service account '%s': %w", serviceAccount.Name, err)
		}
	} else {
		// Found it, so just return it
		return serviceAccount, nil
	}

	log = log.WithValues("serviceAccount", serviceAccountName, "namespace", serviceAccountNS)

	if err := k8sClient.Create(ctx, serviceAccount); err != nil {
		log.Error(err, "Unable to create ServiceAccount")
		return nil, fmt.Errorf("unable to create service account '%s': %w", serviceAccount.Name, err)
	}

	log.Info(fmt.Sprintf("ServiceAccount %s created in namespace %s", serviceAccountName, serviceAccountNS))

	return serviceAccount, nil
}
