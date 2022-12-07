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
//		e.g. export GITHUB_SSH_KEY=$(echo "-----BEGIN OPENSSH PRIVATE KEY-----\nFOOBAR\nFOOBAR\nFOOBAR\nFOOBAR\nFOOBAR\n-----END OPENSSH PRIVATE KEY-----")
*/

// To execute them:
// 1. Set the environment variables
// 2. Run the tests with the following command:
//		 go test -v -run Core -args -ginkgo.v -ginkgo.progress

import (
	"errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"os"
	"strings"
)

const (
	gitServer       = "https://github.com"
	privateRepoURL  = "https://github.com/managed-gitops-test-data/private-repo-test.git"
	privateRepoSSH  = "git@github.com:managed-gitops-test-data/private-repo-test.git"
	privateRepoPath = "resources" // Path to the resources folder in the private repo
	secretToken     = "private-repo-secret-token"
	secretSSHKey    = "private-repo-secret-ssh"
	configMapName   = "config-map-in-private-repo" // Name of the config map to be deployed from the private repo
)

var (
	errGitHubUsernameNotSet  = errors.New("GITHUB_USERNAME env variable not set")
	errGitHubTokenNotSet     = errors.New("GITHUB_TOKEN env variable not set")
	errGitHubSSHKeyNotSet    = errors.New("GITHUB_SSH_KEY env variable not set")
	errGitHubOffline         = errors.New("GitHub is offline")
	errGitHubRepoURLNotValid = errors.New("GitHub repo URL is not valid")
)

func gitopsDeploymentRepositoryCredentialCRForTokenTest() *managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential {
	return &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "private-repo-https",
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
			Repository: privateRepoURL,
			Secret:     secretToken,
		},
	}
}

func gitopsDeploymentRepositoryCredentialCRForSSHTest() *managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential {
	return &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "private-repo-ssh",
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
			Repository: privateRepoSSH,
			Secret:     secretSSHKey,
		},
	}
}

type envConfig struct {
	ready    bool
	username string
	token    string
	sshKey   string
}

var _ = Describe("GitOpsRepositoryCredentials E2E tests", func() {
	Context("Deploy from a private repository (access via username/password)", func() {
		It("Should work without access authentication issues", func() {
			// --- Setup --- //
			// Prepare the test environment by reading appropriate environment variables and reaching out to the git server
			// or else skip the test if the environment is not properly configured
			By("0. Get the environment variables required for the test")

			var env envConfig
			var err error

			env, err = getEnvironmentConfig()
			if err != nil {
				Skip(err.Error())
			}

			Expect(env.ready).To(BeTrue())        // Skip the test if GitHub is offline (unreachable for some reason)
			Expect(env.username).NotTo(BeEmpty()) // Skip the test if the GITHUB_USERNAME environment variable is not set
			Expect(env.token).NotTo(BeEmpty())    // Skip the test if the GITHUB_TOKEN environment variable is not set
			Expect(env.sshKey).NotTo(BeEmpty())   // Skip the test if the GITHUB_SSH_KEY environment variable is not set

			// Get a k8s client to use in the tests
			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			// --- Tests --- //
			By("1. Clean the test environment")
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("2. Create the Secret with the user/pass credentials")
			stringData := map[string]string{
				"username": env.username,
				"password": env.token,
			}
			Expect(k8s.CreateSecret(fixture.GitOpsServiceE2ENamespace, secretToken, stringData, k8sClient)).To(Succeed())

			// --- Step 3: Create the GitOpsRepositoryCredentials CR // ---
			// It has to be in the same namespace with the Secret we created earlier
			By("3. Create the GitOpsDeploymentRepositoryCredential CR for HTTPS")
			CR := gitopsDeploymentRepositoryCredentialCRForTokenTest()
			Expect(k8s.Create(CR, k8sClient)).To(Succeed())

			// --- Step 4: Create the GitOpsDeployment pointing to the previously created GitOpsRepositoryCredential CR // ---
			By("4. Create the GitOpsDeployment CR")
			gitOpsDeployment := buildGitOpsDeploymentResource("private-https-depl", privateRepoURL, privateRepoPath,
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			Expect(k8s.Create(&gitOpsDeployment, k8sClient)).To(Succeed())

			// --- Step 5: Wait for the GitOpsDeployment to be deployed and be healthy // ---
			By("5. GitOpsDeployment should have expected health and status")
			Eventually(gitOpsDeployment, "4m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)),
			)

			// --- Step 6: Check if the ConfigMap is deployed // ---
			By("6. ConfigMap should be deployed")
			configMap := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}
			Eventually(func() error { return k8s.Get(configMap, k8sClient) }, "4m", "1s").Should(Succeed())
		})
	})

	Context("Deploy from a private repository (access via SSH Key)", func() {
		It("Should work without access authentication issues", func() {
			// --- Setup --- //
			// Prepare the test environment by reading appropriate environment variables and reaching out to the git server
			// or else skip the test if the environment is not properly configured
			By("0. Get the environment variables required for the test")

			var env envConfig
			var err error

			env, err = getEnvironmentConfig()
			if err != nil {
				Skip(err.Error())
			}

			Expect(env.ready).To(BeTrue())        // Skip the test if GitHub is offline (unreachable for some reason)
			Expect(env.username).NotTo(BeEmpty()) // Skip the test if the GITHUB_USERNAME environment variable is not set
			Expect(env.token).NotTo(BeEmpty())    // Skip the test if the GITHUB_TOKEN environment variable is not set
			Expect(env.sshKey).NotTo(BeEmpty())   // Skip the test if the GITHUB_SSH_KEY environment variable is not set

			// Get a k8s client to use in the tests
			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			// --- Tests --- //
			By("1. Clean the test environment")
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("2. Create the Secret with the SSH key")
			stringData := map[string]string{
				"sshPrivateKey": env.sshKey,
			}
			Expect(k8s.CreateSecret(fixture.GitOpsServiceE2ENamespace, secretSSHKey, stringData, k8sClient)).To(Succeed())

			// --- Step 3: Create the GitOpsRepositoryCredentials CR // ---
			// It has to be in the same namespace with the Secret we created earlier
			By("3. Create the GitOpsDeploymentRepositoryCredential CR for SSH")
			CR := gitopsDeploymentRepositoryCredentialCRForSSHTest()
			Expect(k8s.Create(CR, k8sClient)).To(Succeed())

			// --- Step 4: Create the GitOpsDeployment pointing to the previously created GitOpsRepositoryCredential CR // ---
			By("4. Create the GitOpsDeployment CR")
			gitOpsDeployment := buildGitOpsDeploymentResource("private-ssh-depl", privateRepoSSH, privateRepoPath,
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated)
			Expect(k8s.Create(&gitOpsDeployment, k8sClient)).To(Succeed())

			// --- Step 5: Wait for the GitOpsDeployment to be deployed and be healthy // ---
			By("5. GitOpsDeployment should have expected health and status")
			Eventually(gitOpsDeployment, "4m", "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy)),
			)

			// --- Step 6: Check if the ConfigMap is deployed // ---
			By("6. ConfigMap should be deployed")
			configMap := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: fixture.GitOpsServiceE2ENamespace,
				},
			}
			Eventually(func() error { return k8s.Get(configMap, k8sClient) }, "4m", "1s").Should(Succeed())
		})
	})
})

// getGithubUsername returns the value of the GITHUB_TOKEN environment variable, or an error if it is not set
func getGithubUsername() (string, error) {
	var err error
	username, present := os.LookupEnv("GITHUB_USERNAME")

	if !present {
		err = errGitHubUsernameNotSet
	}

	return username, err
}

// getGithubToken returns the value of the GITHUB_TOKEN environment variable, or an error if it is not set
func getGithubToken() (string, error) {
	var err error
	password, present := os.LookupEnv("GITHUB_TOKEN")

	if !present {
		err = errGitHubTokenNotSet
	}

	return password, err
}

// getGithubSSHKey returns the value of the GITHUB_SSH_KEY environment variable, or an error if it is not set
func getGithubSSHKey() (string, error) {
	var err error
	sshKey, present := os.LookupEnv("GITHUB_SSH_KEY")

	if !present {
		err = errGitHubSSHKeyNotSet
	}

	return sshKey, err
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
	env.token, err = getGithubToken()
	if err != nil {
		return env, err
	}

	// Grab the git Server's (e.g. GitHub) SSH key from the environment variable
	env.sshKey, err = getGithubSSHKey()
	if err != nil {
		return env, err
	}

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
