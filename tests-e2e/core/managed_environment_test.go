package core

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/argoproj/argo-cd/v2/common"
	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudioshared "github.com/redhat-appstudio/application-api/api/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	argocdutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	clusteragenteventloop "github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers/managed-gitops/eventloop"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	appFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/application"
	dtfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttarget"
	dtcfixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/deploymenttargetclaim"
	environmentFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/environment"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/managedenvironment"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("GitOpsDeployment Managed Environment E2E tests", func() {

	Context("Create a new GitOpsDeployment targeting a ManagedEnvironment", func() {

		ctx := context.Background()

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())
		})

		It("should be healthy and have synced status, and resources should be deployed, when deployed with a ManagedEnv", func() {

			By("creating the GitOpsDeploymentManagedEnvironment")

			kubeConfigContents, apiServerURL, err := fixture.ExtractKubeConfigValues()
			Expect(err).To(BeNil())

			managedEnv, secret := buildManagedEnvironment(apiServerURL, kubeConfigContents, true)

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			err = k8s.Create(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			gitOpsDeploymentResource := BuildGitOpsDeploymentResource("my-gitops-depl",
				"https://github.com/redhat-appstudio/managed-gitops", "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			gitOpsDeploymentResource.Spec.Destination.Environment = managedEnv.Name
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace
			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(BeNil())

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

			Expect(string(argoCDClusterSecret.Data["server"])).To(ContainSubstring(clusteragenteventloop.ManagedEnvironmentQueryParameter))

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

		It("should be healthy and have synced status, and resources should be deployed, when deployed with a ManagedEnv using an existing SA", func() {

			serviceAccountName := "gitops-managed-environment-test-service-account"

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("creating a ServiceAccount which we will deploy with, using the GitOpsDeploymentManagedEnvironment")
			serviceAccount := corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}
			err = k8sClient.Create(context.Background(), &serviceAccount)
			Expect(err).To(Succeed())

			// Now create the cluster role and cluster role binding
			err = createOrUpdateClusterRoleAndRoleBinding(ctx, "123", k8sClient, serviceAccountName, serviceAccount.Namespace, ArgoCDManagerNamespacePolicyRules)
			Expect(err).To(BeNil())

			// Create Service Account and wait for bearer token
			tokenSecret, err := k8s.CreateServiceAccountBearerToken(ctx, k8sClient, serviceAccount.Name, serviceAccount.Namespace)
			Expect(err).To(BeNil())
			Expect(tokenSecret).NotTo(BeNil())

			By("creating the GitOpsDeploymentManagedEnvironment and Secret")

			_, apiServerURL, err := extractKubeConfigValues()
			Expect(err).To(BeNil())

			kubeConfigContents := generateKubeConfig(apiServerURL, fixture.GitOpsServiceE2ENamespace, tokenSecret)

			managedEnv, secret := buildManagedEnvironment(apiServerURL, kubeConfigContents, false)

			err = k8s.Create(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			By("by creating a GitOpsDeployment pointing to the ManagedEnvironment")

			gitOpsDeploymentResource := BuildGitOpsDeploymentResource("my-gitops-depl",
				"https://github.com/redhat-appstudio/managed-gitops",
				"resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			gitOpsDeploymentResource.Spec.Destination.Environment = managedEnv.Name
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace
			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(BeNil())

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

			err = k8s.Delete(&serviceAccount, k8sClient)
			Expect(err).To(Succeed())

			By("deleting the GitOpsDeployment")

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())
		})

		// Same as previous test but the service account token is not passed through to the managed env
		It("should be unhealthy with no sync status because the managed env does not have a proper token", func() {

			serviceAccountName := "gitops-managed-environment-test-service-account"

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			serviceAccount := corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}
			err = k8sClient.Create(context.Background(), &serviceAccount)
			Expect(err).To(Succeed())

			// Now create the cluster role and cluster role binding
			err = createOrUpdateClusterRoleAndRoleBinding(ctx, "123", k8sClient, serviceAccountName, serviceAccount.Namespace, ArgoCDManagerNamespacePolicyRules)
			Expect(err).To(BeNil())

			// Create Service Account and wait for bearer token
			tokenSecret, err := k8s.CreateServiceAccountBearerToken(ctx, k8sClient, serviceAccount.Name, serviceAccount.Namespace)
			Expect(err).To(BeNil())
			Expect(tokenSecret).NotTo(BeNil())

			By("creating the GitOpsDeploymentManagedEnvironment")

			_, apiServerURL, err := extractKubeConfigValues()
			Expect(err).To(BeNil())

			// Set the tokenSecret to be "" to intentionally fail
			kubeConfigContents := generateKubeConfig(apiServerURL, fixture.GitOpsServiceE2ENamespace, "")

			managedEnv, secret := buildManagedEnvironment(apiServerURL, kubeConfigContents, false)

			err = k8s.Create(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			gitOpsDeploymentResource := BuildGitOpsDeploymentResource("my-gitops-depl",
				"https://github.com/redhat-appstudio/managed-gitops",
				"resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			gitOpsDeploymentResource.Spec.Destination.Environment = managedEnv.Name
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace
			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(BeNil())

			By("ensuring GitOpsDeployment has the expected error condition")

			expectedConditions := []managedgitopsv1alpha1.GitOpsDeploymentCondition{
				{
					Type:    managedgitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred,
					Message: "Unable to reconcile the ManagedEnvironment. Verify that the ManagedEnvironment and Secret are correctly defined, and have valid credentials",
					Status:  managedgitopsv1alpha1.GitOpsConditionStatusTrue,
					Reason:  managedgitopsv1alpha1.GitopsDeploymentReasonErrorOccurred,
				},
			}

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveConditions(expectedConditions),
				),
			)

			By("deleting the secret and managed environment")
			err = k8s.Delete(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Delete(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Delete(&serviceAccount, k8sClient)
			Expect(err).To(Succeed())

			By("deleting the GitOpsDeployment")

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())
		})

		It("should be unhealthy with no sync status because the service account doesn't have sufficient permission", func() {

			serviceAccountName := "gitops-managed-environment-test-service-account"

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			serviceAccount := corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}
			err = k8sClient.Create(context.Background(), &serviceAccount)
			Expect(err).To(Succeed())

			insufficientPermissions := []rbacv1.PolicyRule{
				{
					APIGroups: []string{"*"},
					Resources: []string{"Pods"},
					Verbs:     []string{"get", "list"},
				},
			}
			// Now create the cluster role and cluster role binding
			err = createOrUpdateClusterRoleAndRoleBinding(ctx, "123", k8sClient, serviceAccountName, serviceAccount.Namespace, insufficientPermissions)
			Expect(err).To(BeNil())

			// Create Service Account and wait for bearer token
			tokenSecret, err := k8s.CreateServiceAccountBearerToken(ctx, k8sClient, serviceAccount.Name, serviceAccount.Namespace)
			Expect(err).To(BeNil())
			Expect(tokenSecret).NotTo(BeNil())

			By("creating the GitOpsDeploymentManagedEnvironment")

			_, apiServerURL, err := extractKubeConfigValues()
			Expect(err).To(BeNil())

			kubeConfigContents := generateKubeConfig(apiServerURL, fixture.GitOpsServiceE2ENamespace, tokenSecret)

			managedEnv, secret := buildManagedEnvironment(apiServerURL, kubeConfigContents, false)

			err = k8s.Create(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Create(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			gitOpsDeploymentResource := BuildGitOpsDeploymentResource("my-gitops-depl",
				"https://github.com/redhat-appstudio/managed-gitops",
				"resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			gitOpsDeploymentResource.Spec.Destination.Environment = managedEnv.Name
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace
			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(BeNil())

			By("ensuring GitOpsDeployment has the expected error condition")

			expectedConditions := []managedgitopsv1alpha1.GitOpsDeploymentCondition{
				{
					Type:    managedgitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred,
					Message: "Unable to reconcile the ManagedEnvironment. Verify that the ManagedEnvironment and Secret are correctly defined, and have valid credentials",
					Status:  managedgitopsv1alpha1.GitOpsConditionStatusTrue,
					Reason:  managedgitopsv1alpha1.GitopsDeploymentReasonErrorOccurred,
				},
			}

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveConditions(expectedConditions),
				),
			)

			By("deleting the secret and managed environment")
			err = k8s.Delete(&secret, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Delete(&managedEnv, k8sClient)
			Expect(err).To(BeNil())

			err = k8s.Delete(&serviceAccount, k8sClient)
			Expect(err).To(Succeed())

			By("deleting the GitOpsDeployment")

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(Succeed())
		})

		It("verifies that we can deploy to a namespace-scoped ManagedEnvironment", func() {

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("creating a new namespace to deploy to, and a role/rolebinding/SA with permissions to deploy to it")
			newNamespace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-e2e-test-namespace",
					Annotations: map[string]string{
						common.AnnotationKeyManagedBy: common.AnnotationValueManagedByArgoCD,
					},
				},
			}
			err = k8sClient.Create(ctx, &newNamespace)
			Expect(err).To(Succeed())

			serviceAccount := corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service-account",
					Namespace: newNamespace.Name,
				},
			}
			err = k8sClient.Create(context.Background(), &serviceAccount)
			Expect(err).To(Succeed())

			tokenSecret, err := k8s.CreateServiceAccountBearerToken(ctx, k8sClient, serviceAccount.Name, serviceAccount.Namespace)
			Expect(err).To(BeNil())
			Expect(tokenSecret).ToNot(BeEmpty())

			namespaceRole := rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccount.Name + "-role",
					Namespace: newNamespace.Name,
				},
				Rules: []rbacv1.PolicyRule{{
					Verbs:     []string{"*"},
					Resources: []string{"*"},
					APIGroups: []string{"*"},
				}},
			}
			err = k8s.Create(&namespaceRole, k8sClient)
			Expect(err).To(BeNil())

			namespaceRoleBinding := rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccount.Name + "-role-binding",
					Namespace: newNamespace.Name,
				},
				Subjects: []rbacv1.Subject{{
					Name:      serviceAccount.Name,
					Namespace: newNamespace.Name,
					Kind:      "ServiceAccount",
				}},
				RoleRef: rbacv1.RoleRef{
					Name: namespaceRole.Name,
					Kind: "Role",
				},
			}
			err = k8s.Create(&namespaceRoleBinding, k8sClient)
			Expect(err).To(BeNil())

			By("creating the GitOpsDeploymentManagedEnvironment and its Secret, using that service account token")

			_, apiServerURL, err := extractKubeConfigValues()
			Expect(err).To(BeNil())

			kubeConfigContents := generateKubeConfig(apiServerURL, newNamespace.Name, tokenSecret)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env-secret",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Type:       "managed-gitops.redhat.com/managed-environment",
				StringData: map[string]string{"kubeconfig": kubeConfigContents},
			}
			err = k8s.Create(secret, k8sClient)
			Expect(err).To(BeNil())

			managedEnv := &managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-managed-env",
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
					APIURL:                     apiServerURL,
					ClusterCredentialsSecret:   secret.Name,
					AllowInsecureSkipTLSVerify: true,
					CreateNewServiceAccount:    false,
					Namespaces:                 []string{newNamespace.Name},
				},
			}

			err = k8s.Create(managedEnv, k8sClient)
			Expect(err).To(BeNil())

			gitOpsDeploymentResource := BuildGitOpsDeploymentResource("my-gitops-depl",
				"https://github.com/redhat-appstudio/managed-gitops",
				"resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			gitOpsDeploymentResource.Spec.Destination.Environment = managedEnv.Name
			gitOpsDeploymentResource.Spec.Destination.Namespace = newNamespace.Name
			err = k8s.Create(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(BeNil())

			Eventually(gitOpsDeploymentResource, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)))

			componentBDeployment := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "component-b",
					Namespace: newNamespace.Name,
				},
			}

			Eventually(componentBDeployment).Should(k8s.ExistByName(k8sClient),
				"we check that at least one of the resources is deployed to the expected namespace")

			err = k8s.Delete(&gitOpsDeploymentResource, k8sClient)
			Expect(err).To(BeNil())

			By("creating a second GitOpsDeployment, targeting a different namespace without a role and rolebinding on the serviceaccount of the managedenvironment, which should fail")

			gitOpsDeploymentResource2 := BuildGitOpsDeploymentResource("my-gitops-depl2",
				"https://github.com/redhat-appstudio/managed-gitops",
				"resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			gitOpsDeploymentResource2.Spec.Destination.Environment = managedEnv.Name
			gitOpsDeploymentResource2.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace
			err = k8s.Create(&gitOpsDeploymentResource2, k8sClient)
			Expect(err).To(BeNil())

			Eventually(gitOpsDeploymentResource2, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeUnknown),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeMissing)))

			err = k8s.Delete(&gitOpsDeploymentResource2, k8sClient)
			Expect(err).To(BeNil())

			By("creating a new namespace, and adding a role and rolebinding to the existing serviceaccount and managedenvironment ")

			newNamespace2 := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "new-e2e-test-namespace2",
					Annotations: map[string]string{
						common.AnnotationKeyManagedBy: common.AnnotationValueManagedByArgoCD,
					},
				},
			}
			err = k8sClient.Create(ctx, &newNamespace2)
			Expect(err).To(Succeed())

			namespaceRole2 := rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccount.Name + "-role",
					Namespace: newNamespace2.Name,
				},
				Rules: []rbacv1.PolicyRule{{
					Verbs:     []string{"*"},
					Resources: []string{"*"},
					APIGroups: []string{"*"},
				}},
			}
			err = k8s.Create(&namespaceRole2, k8sClient)
			Expect(err).To(BeNil())

			namespaceRoleBinding2 := rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccount.Name + "-role-binding",
					Namespace: newNamespace2.Name,
				},
				Subjects: []rbacv1.Subject{{
					Name:      serviceAccount.Name,
					Namespace: newNamespace.Name,
					Kind:      "ServiceAccount",
				}},
				RoleRef: rbacv1.RoleRef{
					Name: namespaceRole2.Name,
					Kind: "Role",
				},
			}
			err = k8s.Create(&namespaceRoleBinding2, k8sClient)
			Expect(err).To(BeNil())

			By("updating the managedenvironment, adding the second namespace to the list of managed namespaces")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(managedEnv), managedEnv)
			Expect(err).To(BeNil())
			managedEnv.Spec.Namespaces = []string{newNamespace.Name, newNamespace2.Name}

			err = k8sClient.Update(ctx, managedEnv)
			Expect(err).To(BeNil())

			By("create a new GitOpsDeployment that attempts to deploy to the new namespace, using the exist managedenvironment, which should work")
			gitOpsDeploymentResource3 := BuildGitOpsDeploymentResource("my-gitops-depl3",
				"https://github.com/redhat-appstudio/managed-gitops",
				"resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)

			gitOpsDeploymentResource3.Spec.Destination.Environment = managedEnv.Name
			gitOpsDeploymentResource3.Spec.Destination.Namespace = newNamespace2.Name
			err = k8s.Create(&gitOpsDeploymentResource3, k8sClient)
			Expect(err).To(BeNil())

			Eventually(gitOpsDeploymentResource3, "2m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)),
				"gitopsdeployment and managedenvironment should be able to deploy to the new namespace")

			componentBDeployment = &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "component-b",
					Namespace: newNamespace2.Name,
				},
			}

			Eventually(componentBDeployment).Should(k8s.ExistByName(k8sClient),
				"we check that at least one of the resources is deployed to the new namespace")

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
						Env: []appstudioshared.EnvVarPair{},
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
			Expect(err).To(BeNil())

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
			Expect(err).To(BeNil())

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
			Expect(err).To(BeNil())

			Eventually(managedEnvCR, "2m", "1s").Should(
				SatisfyAll(
					managedenvironment.HaveClusterResources(environment.Spec.UnstableConfigurationFields.ClusterResources),
					managedenvironment.HaveNamespaces(environment.Spec.UnstableConfigurationFields.Namespaces),
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
						APIURL:                     apiServerURL,
						ClusterCredentialsSecret:   secret.Name,
						DefaultNamespace:           fixture.GitOpsServiceE2ENamespace,
						AllowInsecureSkipTLSVerify: true,
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

// extractKubeConfigValues returns contents of k8s config from $KUBE_CONFIG, plus server api url (and error)
func extractKubeConfigValues() (string, string, error) {

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()

	config, err := loadingRules.Load()
	if err != nil {
		return "", "", err
	}

	context, ok := config.Contexts[config.CurrentContext]
	if !ok || context == nil {
		return "", "", fmt.Errorf("no context")
	}

	cluster, ok := config.Clusters[context.Cluster]
	if !ok || cluster == nil {
		return "", "", fmt.Errorf("no cluster")
	}

	var kubeConfigDefault string

	paths := loadingRules.Precedence
	{

		for _, path := range paths {

			GinkgoWriter.Println("Attempting to read kube config from", path)

			// homeDir, err := os.UserHomeDir()
			// if err != nil {
			// 	return "", "", err
			// }

			_, err = os.Stat(path)
			if err != nil {
				GinkgoWriter.Println("Unable to resolve path", path, err)
			} else {
				// Success
				kubeConfigDefault = path
				break
			}

		}

		if kubeConfigDefault == "" {
			return "", "", fmt.Errorf("unable to retrieve kube config path")
		}
	}

	kubeConfigContents, err := os.ReadFile(kubeConfigDefault)
	if err != nil {
		return "", "", err
	}

	return string(kubeConfigContents), cluster.Server, nil
}

func generateKubeConfig(serverURL string, currentNamespace string, token string) string {

	return `
apiVersion: v1
kind: Config
clusters:
  - cluster:
      insecure-skip-tls-verify: true
      server: ` + serverURL + `
    name: cluster-name
contexts:
  - context:
      cluster: cluster-name
      namespace: ` + currentNamespace + `
      user: user-name
    name: context-name
current-context: context-name
preferences: {}
users:
  - name: user-name
    user:
      token: ` + token + `
`

}

func buildManagedEnvironment(apiServerURL string, kubeConfigContents string, createNewServiceAccount bool) (managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, corev1.Secret) {
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
			CreateNewServiceAccount:    createNewServiceAccount,
		},
	}

	return *managedEnv, *secret
}

const (
	ArgoCDManagerClusterRoleNamePrefix        = "argocd-manager-cluster-role-"
	ArgoCDManagerClusterRoleBindingNamePrefix = "argocd-manager-cluster-role-binding-"
)

var (
	ArgoCDManagerNamespacePolicyRules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	}
)

func createOrUpdateClusterRoleAndRoleBinding(ctx context.Context, uuid string, k8sClient client.Client,
	serviceAccountName string, serviceAccountNamespace string, policyRules []rbacv1.PolicyRule) error {

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: ArgoCDManagerClusterRoleNamePrefix + uuid,
		},
	}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterRole), clusterRole); err != nil {

		clusterRole.Rules = policyRules
		if err := k8sClient.Create(ctx, clusterRole); err != nil {
			return fmt.Errorf("unable to create clusterrole: %w", err)
		}
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: ArgoCDManagerClusterRoleBindingNamePrefix + uuid,
		},
	}

	clusterRoleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     clusterRole.Name,
	}

	clusterRoleBinding.Subjects = []rbacv1.Subject{{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      serviceAccountName,
		Namespace: serviceAccountNamespace,
	}}

	if err := k8sClient.Create(ctx, clusterRoleBinding); err != nil {
		return fmt.Errorf("unable to create clusterrole: %w", err)
	}

	return nil
}
