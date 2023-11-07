package core

/* To run the tests, you need to set the following environment variables:
//
// 		GITHUB_USERNAME: The username of the GitHub account
//		e.g. export GITHUB_USERNAME=bruce@wayne.com
//
//		GITHUB_TOKEN: The token of the GitHub account
//		e.g. export GITHUB_TOKEN=1234567890
//
//		GITHUB_SSH_KEY: The SSH key of the private GitHub repository
//		e.g. export GITHUB_SSH_KEY=$(echo "-----BEGIN OPENSSH PRIVATE KEY-----\nFOOBAR\nFOOBAR\nFOOBAR\nFOOBAR\nFOOBAR\n-----END OPENSSH PRIVATE KEY-----") # gitleaks:allow
*/

// To execute them:
// 1. Set the environment variables
// 2. Run the tests with the following command:
//		 cd tests-e2e/tests-e2e/core/; go test -v -run Core -args -ginkgo.v -ginkgo.progress

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	argosharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	gitopsDeplRepoCredFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeploymentrepositorycredential"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Variables and constants used in the tests
// ------------------------------------------------------------------------------------------------
// Optionally you can change the following variables to point to a different private repository (e.g. GitLab
// **NOTE**: The repository must be private and the tests must be able to access it via HTTPS or SSH (see env Variables)
// **NOTE**: The repository must contain a valid configmap.yaml file in the "$privateRepoPath" directory

const (
	gitServer       = "https://github.com"
	privateRepoURL  = "https://github.com/managed-gitops-test-data/private-repo-test.git"
	privateRepoSSH  = "git@github.com:managed-gitops-test-data/private-repo-test.git"
	privateRepoPath = "resources" // Path to the resources folder in the private repo, where the configmap.yaml file is located
)

// Do NOT change any variable or const below this line
// ------------------------------------------------------------------------------------------------

const (
	// For HTTPS/Username & Password
	secretToken     = "private-repo-secret-token" // Name of the secret containing the token for HTTPS access
	repoCredCRToken = "private-repo-https"        // Name of the GitOpsDeploymentRepositoryCredential CR for HTTPS token test

	// For SSH
	secretSSHKey  = "private-repo-secret-ssh" // Name of the secret containing the SSH key for SSH access
	repoCredCRSSH = "private-repo-ssh"        // Name of the GitOpsDeploymentRepositoryCredential CR for SSH key test

	// Artifact to be deployed
	configMapName = "config-map-in-private-repo" // Name of the config map to be deployed from the private repo
)

var (
	errGitHubUsernameNotSet  = errors.New("GITHUB_USERNAME env variable not set")
	errGitHubOffline         = errors.New("GitHub is offline")
	errGitHubRepoURLNotValid = errors.New("GitHub repo URL is not valid")
)

// Main tests
// ------------------------------------------------------------------------------------------------

var _ = Describe("GitOpsRepositoryCredentials E2E tests", func() {

	const (
		deploymentCRToken = "private-https-deploy" // Name of the GitOpsDeployment CR for HTTPS token test
		deploymentCRSSH   = "private-ssh-deploy"   // Name of the GitOpsDeployment CR for SSH key test
	)

	var (
		env       envConfig // Environment configuration, e.g. username, password, sshkey
		err       error
		k8sClient client.Client // Kubernetes client to interact with the cluster
		configMap *corev1.ConfigMap
	)

	BeforeEach(func() {
		// --- Setup --- //
		// Prepare the test environment by reading appropriate environment variables and reaching out to the git server
		// or else skip the test if the environment is not properly configured
		By("0. Get the environment variables required for the test and clean the test environment")

		env, err = getEnvironmentConfig()
		if err != nil {
			Skip(err.Error())
		}

		Expect(env.ready).To(BeTrue()) // Skip the test if GitHub is offline (unreachable for some reason)

		// Get a k8s client to use in the tests
		k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
		Expect(err).To(Succeed())

		// Get the config map to be deployed from the private repo
		configMap = getConfigMapYAML()

		Expect(fixture.EnsureCleanSlate()).To(Succeed())

	})

	Context("Deploy from a private repository, access via username/password", func() {

		It("Should work without HTTPS/Token authentication issues", func() {
			// --- Tests --- //

			if env.username == "" || env.token == "" {
				Skip("username/token not defined")
			}
			Expect(env.username).NotTo(BeEmpty())
			Expect(env.token).NotTo(BeEmpty())

			By("1. Create the Secret with the user/pass credentials")
			stringData := map[string]string{
				"username": env.username,
				"password": env.token,
			}
			Expect(k8s.CreateSecret(fixture.GitOpsServiceE2ENamespace, secretToken, sharedutil.RepositoryCredentialSecretType, stringData, k8sClient)).Error().ToNot(HaveOccurred())

			By("2. Create the GitOpsDeploymentRepositoryCredential CR for HTTPS")
			CR := gitopsDeploymentRepositoryCredentialCRForTokenTest()
			Expect(k8s.Create(&CR, k8sClient)).To(Succeed())

			expectedRepositoryCredentialStatusConditions := []metav1.Condition{
				{
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
					Status: metav1.ConditionFalse,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonValidRepositoryUrl,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
					Status: metav1.ConditionTrue,
				},
			}

			Eventually(CR, "2m", "1s").Should(gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions))
			Consistently(CR, "10s", "1s").Should(gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions))

			By("3. Create the GitOpsDeployment CR")
			gitOpsDeployment := gitopsDeplFixture.BuildGitOpsDeploymentResource(deploymentCRToken, privateRepoURL, privateRepoPath,
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			Expect(k8s.Create(&gitOpsDeployment, k8sClient)).To(Succeed())

			By("4. GitOpsDeployment should have expected health and status")
			Eventually(gitOpsDeployment, "4m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)),
			)

			Eventually(CR, "1m", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			Consistently(CR, "10s", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			By("5. ConfigMap should be deployed")
			Eventually(func() error { return k8s.Get(configMap, k8sClient) }, "1m", "1s").Should(Succeed())

			By("6. Secret should be created by for GitOpsDeploymentRepositoryCredential resource")

			ctx := context.Background()
			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).ToNot(HaveOccurred())

			var apiCRToDatabaseMappings []db.APICRToDatabaseMapping
			Expect(dbQueries.UnsafeListAllAPICRToDatabaseMappings(ctx, &apiCRToDatabaseMappings)).To(Succeed())

			matchFound := false
			for idx := range apiCRToDatabaseMappings {
				apiCRToDBMapping := apiCRToDatabaseMappings[idx]
				if apiCRToDBMapping.APIResourceUID == string(CR.UID) {
					repoCred, err := dbQueries.GetRepositoryCredentialsByID(ctx, apiCRToDBMapping.DBRelationKey)
					Expect(err).ToNot(HaveOccurred())

					// Retrieve the Secret based on the RepositoryCredentials row
					secret := &corev1.Secret{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: argosharedutil.GenerateArgoCDRepoCredSecretName(repoCred), Namespace: "gitops-service-argocd"}, secret)).To(Succeed())

					matchFound = true
					break
				}
			}
			Expect(matchFound).To(BeTrue())
		})
	})

	Context("Deploy from a private repository (access via SSH Key)", func() {

		It("Should work without SSH authentication issues", func() {
			// --- Tests --- //

			if env.sshKey == "" {
				Skip("SSH key not defined")
			}

			Expect(env.sshKey).NotTo(BeEmpty()) // Skip the test if the GITHUB_SSH_KEY environment variable is not set

			By("1. Create the Secret with the SSH key")
			stringData := map[string]string{
				"sshPrivateKey": env.sshKey,
			}
			Expect(k8s.CreateSecret(fixture.GitOpsServiceE2ENamespace, secretSSHKey, sharedutil.RepositoryCredentialSecretType, stringData, k8sClient)).Error().ToNot(HaveOccurred())

			By("2. Create the GitOpsDeploymentRepositoryCredential CR for SSH")
			CR := gitopsDeploymentRepositoryCredentialCRForSSHTest()
			Expect(k8s.Create(&CR, k8sClient)).To(Succeed())

			expectedRepositoryCredentialStatusConditions := []metav1.Condition{
				{
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
					Status: metav1.ConditionFalse,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonValidRepositoryUrl,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
					Status: metav1.ConditionTrue,
				},
			}

			Eventually(CR, "2m", "1s").Should(gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions))
			Consistently(CR, "10s", "1s").Should(gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions))

			By("3. Create the GitOpsDeployment CR")
			gitOpsDeployment := gitopsDeplFixture.BuildGitOpsDeploymentResource(deploymentCRSSH, privateRepoSSH, privateRepoPath,
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			Expect(k8s.Create(&gitOpsDeployment, k8sClient)).To(Succeed())

			By("4. GitOpsDeployment should have expected health and status")
			Eventually(gitOpsDeployment, "4m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)),
			)

			expectedRepositoryCredentialStatusConditions = []metav1.Condition{
				{
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
					Status: metav1.ConditionFalse,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonValidRepositoryUrl,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
					Status: metav1.ConditionTrue,
				},
			}
			Eventually(CR, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			By("5. ConfigMap should be deployed")
			configMap := getConfigMapYAML()
			Eventually(func() error { return k8s.Get(configMap, k8sClient) }, "4m", "1s").Should(Succeed())

			By("6. Secret should be created by for GitOpsDeploymentRepositoryCredential resource")

			ctx := context.Background()
			dbQueries, err := db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).ToNot(HaveOccurred())

			var apiCRToDatabaseMappings []db.APICRToDatabaseMapping
			Expect(dbQueries.UnsafeListAllAPICRToDatabaseMappings(ctx, &apiCRToDatabaseMappings)).To(Succeed())

			matchFound := false
			for idx := range apiCRToDatabaseMappings {
				apiCRToDBMapping := apiCRToDatabaseMappings[idx]
				if apiCRToDBMapping.APIResourceUID == string(CR.UID) {
					repoCred, err := dbQueries.GetRepositoryCredentialsByID(ctx, apiCRToDBMapping.DBRelationKey)
					Expect(err).ToNot(HaveOccurred())

					secret := &corev1.Secret{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: argosharedutil.GenerateArgoCDRepoCredSecretName(repoCred), Namespace: "gitops-service-argocd"}, secret)).To(Succeed())
					matchFound = true
					break
				}
			}
			Expect(matchFound).To(BeTrue())
		})
	})

	Context("Check changes to Gitops Deployment Repository Credential Status Conditions for Private Repo", func() {

		It("Should work without HTTPS/Token authentication issues", func() {

			if env.username == "" || env.token == "" {
				Skip("username/token not defined")
			}
			Expect(env.username).NotTo(BeEmpty())
			Expect(env.token).NotTo(BeEmpty())

			// --- Tests --- //

			By("1. Create the Secret with invalid user/pass credentials")
			stringData := map[string]string{
				"username": env.username,
				"password": "test-password",
			}

			secret, err := k8s.CreateSecret(fixture.GitOpsServiceE2ENamespace, secretToken, sharedutil.RepositoryCredentialSecretType, stringData, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			expectedRepositoryCredentialStatusConditions := []metav1.Condition{
				{
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonInvalidCredentials,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonValidRepositoryUrl,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonInvalidCredentials,
					Status: metav1.ConditionFalse,
				},
			}

			By("2. Create the GitOpsDeploymentRepositoryCredential CR for HTTPS")
			CR := gitopsDeploymentRepositoryCredentialCRForTokenTest()
			Expect(k8s.Create(&CR, k8sClient)).To(Succeed())

			Eventually(CR, "3m", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			Consistently(CR, "10s", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			Expect(k8s.Delete(&secret, k8sClient)).To(Succeed())

			By("3. Create a new Secret with valid credentials")
			stringData = map[string]string{
				"username": env.username,
				"password": env.token,
			}
			Expect(k8s.CreateSecret(fixture.GitOpsServiceE2ENamespace, secretToken, sharedutil.RepositoryCredentialSecretType, stringData, k8sClient)).Error().ToNot(HaveOccurred())

			expectedRepositoryCredentialStatusConditions = []metav1.Condition{
				{
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
					Status: metav1.ConditionFalse,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonValidRepositoryUrl,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
					Status: metav1.ConditionTrue,
				},
			}

			By("4. GitOpsDeploymentRepositoryCredential conditions should have been updated from failure to success")
			Eventually(CR, "1m", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			Consistently(CR, "10s", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

		})
	})

	When("changes are made to the GitOpsDeploymentRepositoryCredential's Secret", func() {

		It("should correctly reconcile the Secret contents", func() {

			if env.username == "" || env.token == "" {
				Skip("username/token not defined")
			}
			Expect(env.username).NotTo(BeEmpty())
			Expect(env.token).NotTo(BeEmpty())

			// --- Tests --- //

			By("create the Secret with invalid user/pass credentials")
			stringData := map[string]string{
				"username": env.username,
				"password": "invalid-contents",
			}
			secret, err := k8s.CreateSecret(fixture.GitOpsServiceE2ENamespace, secretToken, sharedutil.RepositoryCredentialSecretType, stringData, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			By("create the GitOpsDeploymentRepositoryCredential CR for HTTPS, and verifying the status field shows invalid credentials")
			CR := gitopsDeploymentRepositoryCredentialCRForTokenTest()
			Expect(k8s.Create(&CR, k8sClient)).To(Succeed())

			expectedRepositoryCredentialStatusConditions := []metav1.Condition{
				{
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonInvalidCredentials,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonValidRepositoryUrl,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonInvalidCredentials,
					Status: metav1.ConditionFalse,
				},
			}

			Eventually(CR, "1m", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			Consistently(CR, "10s", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			By("updating the existing Secret to a valid username/password, and verifying the status field changes to valid status")

			secret.Data = map[string][]byte{
				"username": []byte(env.username),
				"password": []byte(env.token),
			}

			Expect(k8sClient.Update(context.Background(), &secret)).To(Succeed())

			expectedRepositoryCredentialStatusConditions = []metav1.Condition{
				{
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
					Status: metav1.ConditionFalse,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonValidRepositoryUrl,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonCredentialsUpToDate,
					Status: metav1.ConditionTrue,
				},
			}

			Eventually(CR, "1m", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			Consistently(CR, "10s", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			By("updating the contents to invalid contents again, and verifying the status field changes to invalid credentials")

			secret.Data = map[string][]byte{
				"username": []byte(env.username),
				"password": []byte("invalid-contents"),
			}

			Expect(k8sClient.Update(context.Background(), &secret)).To(Succeed())

			expectedRepositoryCredentialStatusConditions = []metav1.Condition{
				{
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionErrorOccurred,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonInvalidCredentials,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryUrl,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonValidRepositoryUrl,
					Status: metav1.ConditionTrue,
				}, {
					Type:   managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialConditionValidRepositoryCredential,
					Reason: managedgitopsv1alpha1.RepositoryCredentialReasonInvalidCredentials,
					Status: metav1.ConditionFalse,
				},
			}

			Eventually(CR, "1m", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

			Consistently(CR, "10s", "1s").Should(
				SatisfyAll(
					gitopsDeplRepoCredFixture.HaveConditions(expectedRepositoryCredentialStatusConditions)),
			)

		})
	})

})

// Helper functions
// ------------------------------------------------------------------------------------------------

// getGithubUsername returns the value of the GITHUB_TOKEN environment variable, or an error if it is not set
func getGithubUsername() (string, error) {
	var err error
	username, present := os.LookupEnv("GITHUB_USERNAME")

	if !present {
		err = errGitHubUsernameNotSet
	}

	return username, err
}

// getGithubToken returns the value of the GITHUB_TOKEN environment variable
func getGithubToken() string {
	password, _ := os.LookupEnv("GITHUB_TOKEN")

	return password
}

// getGithubSSHKey returns the value of the GITHUB_SSH_KEY environment variable
func getGithubSSHKey() string {
	sshKey, _ := os.LookupEnv("GITHUB_SSH_KEY")

	return sshKey
}

// isGitServerOnline returns true if the git server (e.g. GitHub) is online, or false if it is offline
func isGitServerOnline(gitServer string) bool {
	r, e := http.Head(gitServer)
	return e == nil && r.StatusCode == 200
}

// getEnvironmentConfig returns the environment configuration, or an error if it is not properly set
func getEnvironmentConfig() (envConfig, error) {
	var err error

	var env = envConfig{}

	// Grab the git Server's (e.g. GitHub) username from the environment variable
	env.username, err = getGithubUsername()
	if err != nil {
		return env, err
	}

	// Grab the git Server's (e.g. GitHub) token from the environment variable
	env.token = getGithubToken()

	// Grab the git Server's (e.g. GitHub) SSH key from the environment variable
	env.sshKey = getGithubSSHKey()

	// Check if the git Server (e.g. GitHub) is online or the test environment is blocking the connection.
	env.ready = isGitServerOnline(gitServer)
	if !env.ready {
		err = errGitHubOffline
		return env, err
	}

	// Check if the gitServer is part of the privateRepoURL
	if !strings.Contains(privateRepoURL, gitServer) {
		err = errGitHubRepoURLNotValid
		return env, err
	}

	return env, nil
}

// gitopsDeploymentRepositoryCredentialCRForTokenTest returns a GitOpsDeploymentRepositoryCredential CR for the HTTPS token test
// pointing to the private repo (HTTPS URL format) and using the secret containing the username and the token
func gitopsDeploymentRepositoryCredentialCRForTokenTest() managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential {
	return managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoCredCRToken,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
			Repository: privateRepoURL,
			Secret:     secretToken,
		},
	}
}

// gitopsDeploymentRepositoryCredentialCRForSSHTest returns a GitOpsDeploymentRepositoryCredential CR for the SSH key test
// pointing to the private repo (SSH URL format) and using the secret containing the SSH key
func gitopsDeploymentRepositoryCredentialCRForSSHTest() managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential {
	return managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
		ObjectMeta: metav1.ObjectMeta{
			Name:      repoCredCRSSH,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
			Repository: privateRepoSSH,
			Secret:     secretSSHKey,
		},
	}
}

// getConfigMapYAML returns the YAML representation of the ConfigMap
func getConfigMapYAML() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
	}
}

// Helper Structs
// ------------------------------------------------------------------------------------------------

// envConfig contains the environment variables needed to run the tests
// It is used to avoid calling the os.LookupEnv function multiple times
// and to avoid having to pass the environment variables as parameters to the functions that need them
type envConfig struct {
	ready    bool
	username string
	token    string
	sshKey   string
}
