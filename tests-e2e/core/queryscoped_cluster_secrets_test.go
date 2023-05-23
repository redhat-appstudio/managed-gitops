package core

import (
	"context"
	"fmt"
	"strings"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"

	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	k8s "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Query-scoped GitOpsDeployment tests", func() {

	Context("Verify that adding a query parameter to an Argo CD cluster secret will cause Argo CD to treat it as a different cluster", func() {

		argoCDNamespace := dbutil.GetGitOpsEngineSingleInstanceNamespace()

		var k8sClient client.Client

		users := []string{"user-a", "user-b"}

		// Wait for X number of Argo CD Applications to exist in the Argo CD namespace
		// - optionally, this function may also be used to delete any existing Argo CD Applications.
		eventuallyThereShouldBeXArgoCDApplications := func(expectedApplications int, deleteArgoCDApplications bool, removeFinalizers bool) {
			var argoCDApplicationList appv1alpha1.ApplicationList
			Eventually(func() bool {
				err := k8sClient.List(context.Background(), &argoCDApplicationList)
				if err != nil {
					fmt.Println(err)
					return false
				}

				if deleteArgoCDApplications {
					// Remove finalizers from Argo CD Applications
					for _, argoCDApp := range argoCDApplicationList.Items {

						err := k8sClient.Delete(context.Background(), &argoCDApp)
						if err != nil {
							fmt.Println(err)
							return false
						}
					}
				}

				if removeFinalizers {

					// Remove finalizers from Argo CD Applications
					for _, argoCDApp := range argoCDApplicationList.Items {

						err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&argoCDApp), &argoCDApp)
						if err != nil {
							fmt.Println(err)
							return false
						}
						argoCDApp.Finalizers = []string{}
						err = k8sClient.Update(context.Background(), &argoCDApp)
						if err != nil {
							fmt.Println(err)
							return false
						}
					}
				}

				fmt.Println("Argo CD Applications:", len(argoCDApplicationList.Items))
				return len(argoCDApplicationList.Items) == expectedApplications

			}, "60s", "1s").Should(BeTrue(), "only X Argo CD Application should be present after we wait a minute for the old to be deleted")

		}

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
				err = fixture.DeleteNamespace(userName, config)
				Expect(err).To(BeNil())
			}

			By("cleaning up old ClusterRole/ClusterRoleBinding")
			for _, userName := range users {
				clusterRole, clusterRoleBinding := k8s.GenerateReadOnlyClusterRoleandBinding(userName)
				err = k8sClient.Delete(context.Background(), &clusterRole)
				if err != nil && !apierr.IsNotFound(err) {
					Expect(err).To(BeNil())
				}

				err = k8sClient.Delete(context.Background(), &clusterRoleBinding)
				if err != nil && !apierr.IsNotFound(err) {
					Expect(err).To(BeNil())
				}
			}

			eventuallyThereShouldBeXArgoCDApplications(0, true, true)

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
				Expect(err).To(BeNil())
				userServiceAccountTokens[userName] = userToken
			}

			By("creating Argo CD Cluster secrets for those user's service accounts, scoped to deploy only to their namespace")

			for destUserNamespace := range userServiceAccountTokens {

				for userName, userToken := range userServiceAccountTokens {

					_, apiServerURL, err := fixture.ExtractKubeConfigValues()
					Expect(err).To(BeNil())

					// We actually need a managed environment secret containing a kubeconfig that has the bearer token
					kubeConfigContents := k8s.GenerateKubeConfig(apiServerURL, userName, userToken)
					secret := corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "managed-env-deploys-to-" + userName,
							Namespace: destUserNamespace,
						},
						Type:       "managed-gitops.redhat.com/managed-environment",
						StringData: map[string]string{"kubeconfig": kubeConfigContents},
					}
					err = k8s.Create(&secret, k8sClient)
					Expect(err).To(BeNil())

					managedEnv := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: secret.Namespace,
							Name:      secret.Name,
						},
						Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
							APIURL:                     apiServerURL,
							ClusterCredentialsSecret:   secret.Name,
							AllowInsecureSkipTLSVerify: true,
							CreateNewServiceAccount:    false,
						},
					}

					if !clusterScopedSecrets {
						managedEnv.Spec.Namespaces = []string{userName}
					}

					err = k8s.Create(&managedEnv, k8sClient)
					Expect(err).To(BeNil())

				}

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

					By("creating a GitOpsDeployment to deploy to the user's namespace")

					gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-" + userName,
							Namespace: userName,
							// Finalizers: []string{"resources-finalizer.argocd.argoproj.io"},
						},
						Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
							Source: managedgitopsv1alpha1.ApplicationSource{
								RepoURL: "https://github.com/redhat-appstudio/managed-gitops/",
								Path:    "resources/test-data/sample-gitops-repository/environments/overlays/dev",
							},
							Destination: managedgitopsv1alpha1.ApplicationDestination{
								Environment: "managed-env-deploys-to-" + userName,
								Namespace:   userName,
							},
							Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
						},
					}

					err := k8sClient.Create(context.Background(), &gitopsDeployment)
					Expect(err).To(BeNil())

					By("ensuring GitOpsDeployment should have expected health and status")

					Eventually(gitopsDeployment, "2m", "1s").Should(
						SatisfyAll(
							gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
							gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)))

					By("ensuring the resources of the GitOps repo are successfully deployed")

					componentADepl := &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{Name: "component-a", Namespace: userName},
					}
					componentBDepl := &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{Name: "component-b", Namespace: userName},
					}
					Eventually(componentADepl, "60s", "1s").Should(k8s.ExistByName(k8sClient))
					Eventually(componentBDepl, "60s", "1s").Should(k8s.ExistByName(k8sClient))

					By("finding the Argo CD Application that corresponds to the GitOpsDeployment")
					var argoCDApplicationList appv1alpha1.ApplicationList
					err = k8sClient.List(context.Background(), &argoCDApplicationList)
					Expect(err).To(BeNil())
					Expect(len(argoCDApplicationList.Items)).To(Equal(1))

					item := argoCDApplicationList.Items[0]

					clusterSecret := corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      item.Spec.Destination.Name,
							Namespace: item.Namespace,
						},
					}
					By("ensuring the Secret that is pointed to by the Argo CD Application has the expected query parameter")
					err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&clusterSecret), &clusterSecret)
					Expect(err).To(BeNil())
					Expect(clusterSecret.Data["server"]).To(ContainSubstring("?managedEnvironment="))

					By("deleting the GitOpsDeployment")

					err = k8sClient.Delete(context.Background(), &gitopsDeployment)
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

				By("creating a GitOpsDeployment to deploy to the user's namespace")

				gitopsDeployment := managedgitopsv1alpha1.GitOpsDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-" + userName,
						Namespace: userName,
						// Finalizers: []string{"resources-finalizer.argocd.argoproj.io"},
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
						Source: managedgitopsv1alpha1.ApplicationSource{
							RepoURL: "https://github.com/redhat-appstudio/managed-gitops/",
							Path:    "resources/test-data/sample-gitops-repository/environments/overlays/dev",
						},
						Destination: managedgitopsv1alpha1.ApplicationDestination{
							Environment: "managed-env-deploys-to-" + userName,
							Namespace:   otherUser,
						},
						Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
					},
				}

				err := k8sClient.Create(context.Background(), &gitopsDeployment)
				Expect(err).To(BeNil())

				By("ensuring GitOpDeployment should have expected health and status")

				// If Argo CD cluster secret is cluster-scoped: it can see the application is out of sync
				expectedStatusCode := managedgitopsv1alpha1.SyncStatusCodeOutOfSync
				if !clusterScoped {
					// If the Argo CD application is namespaced-scoped: it cannot access the resources in the other user's namespace
					expectedStatusCode = managedgitopsv1alpha1.SyncStatusCodeUnknown
				}

				Eventually(gitopsDeployment, "2m", "1s").Should(
					SatisfyAll(
						gitopsDeplFixture.HaveSyncStatusCode(expectedStatusCode),
						gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeMissing)))

				Consistently(gitopsDeployment, "30s", "1s").Should(
					SatisfyAll(
						gitopsDeplFixture.HaveSyncStatusCode(expectedStatusCode),
						gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeMissing)))

				By("ensuring the expected failure message is present")

				err = k8s.Get(&gitopsDeployment, k8sClient)
				Expect(err).To(BeNil())

				By("finding the Argo CD Application that corresponds to the GitOpsDeployment")

				eventuallyThereShouldBeXArgoCDApplications(1, false, false)

				var argoCDApplicationList appv1alpha1.ApplicationList
				err = k8sClient.List(context.Background(), &argoCDApplicationList)
				Expect(err).To(BeNil())

				argoCDApplication := argoCDApplicationList.Items[0]

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

				By("deleting the GitOpsDeployment")

				err = k8sClient.Delete(context.Background(), &gitopsDeployment)
				Expect(err).To(BeNil())

				eventuallyThereShouldBeXArgoCDApplications(0, false, true)

			}

		},
			Entry("user cannot deploy to another users namepace, when using cluster-scoped argo cd cluster secrets", true),
			Entry("user cannot deploy to another users namepace, when using namespace-scoped argo cd cluster secrets", false))

	})
})
