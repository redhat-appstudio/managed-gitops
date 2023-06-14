package argocd

import (
	"context"
	"os"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	appFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/application"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ArgoCD AppProject E2E tests", func() {
	Context("Matching git repos", func() {
		var (
			appProject appv1.AppProject
			app        appv1.Application
			config     *rest.Config
			ctx        context.Context
			err        error
			k8sClient  client.Client
			secret     corev1.Secret
		)

		BeforeEach(func() {
			By("Delete old namespaces and kube-system resources")
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			ctx = context.Background()

			config, err = fixture.GetSystemKubeConfig()
			Expect(err).To(BeNil())

			k8sClient, err = fixture.GetKubeClient(config)
			Expect(err).To(BeNil())

			appProject = appv1.AppProject{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-project",
					Namespace: dbutil.GetGitOpsEngineSingleInstanceNamespace(),
				},
				Spec: appv1.AppProjectSpec{
					Destinations: []appv1.ApplicationDestination{
						{
							Server:    "https://kubernetes.default.svc",
							Namespace: fixture.GitOpsServiceE2ENamespace,
						},
					},
				},
			}

			app = appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argo-app-01",
					Namespace: dbutil.GetGitOpsEngineSingleInstanceNamespace(),
				},
				Spec: appv1.ApplicationSpec{
					Project: appProject.Name,
					Source: appv1.ApplicationSource{
						Path:           "resources/test-data/sample-gitops-repository/environments/overlays/dev",
						TargetRevision: "HEAD",
					},
					Destination: appv1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: fixture.GitOpsServiceE2ENamespace,
					},
					SyncPolicy: &appv1.SyncPolicy{
						Automated: &appv1.SyncPolicyAutomated{},
					},
				},
			}

			secret = corev1.Secret{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-project-secret",
					Namespace: app.Namespace,
					Labels: map[string]string{
						"argocd.argoproj.io/secret-type": "repository",
					},
				},
				Data: nil,
				Type: "",
			}

			// Ensure there's no lingering app project or secret
			err = k8sClient.Delete(ctx, &appProject)
			if err != nil {
				Expect(errors.IsNotFound(err)).To(BeTrue())
			}
			err = k8sClient.Delete(ctx, &secret)
			if err != nil {
				Expect(errors.IsNotFound(err)).To(BeTrue())
			}
		})

		It("Should succeed if the app project and the application both use an https url", func() {
			By("Creating an app project with an https url")
			appProject.Spec.SourceRepos = []string{"https://github.com/redhat-appstudio/managed-gitops"}
			Expect(k8sClient.Create(ctx, &appProject)).To(Succeed())

			By("Creating an application with an https url")
			app.Spec.Source.RepoURL = "https://github.com/redhat-appstudio/managed-gitops"
			Expect(k8sClient.Create(ctx, &app)).To(Succeed())

			Eventually(app, "2m", "1s").Should(
				SatisfyAll(
					appFixture.HaveSyncStatusCode(appv1.SyncStatusCodeSynced),
					appFixture.HaveHealthStatusCode(health.HealthStatusHealthy),
				))
		})

		// This test requires the following environment variable to be set.
		//		GITHUB_SSH_KEY: The SSH key of the private GitHub repository
		// If the variable is not set, the test will be skipped.
		It("Should succeed if the app project and the application both use a git url", func() {
			const giturl = "git@github.com:managed-gitops-test-data/private-repo-test.git"

			By("Reading the SSH private key from the environment")
			sshKey, present := os.LookupEnv("GITHUB_SSH_KEY")
			if !present || sshKey == "" {
				Skip("GITHUB_SSH_KEY environment variable not set")
			}

			By("Creating the Secret with the SSH key")
			stringData := map[string]string{
				"type":          "git",
				"url":           giturl,
				"sshPrivateKey": sshKey,
			}
			secret.StringData = stringData
			Expect(k8sClient.Create(ctx, &secret)).To(Succeed())

			By("Creating an app project with a git url")
			appProject.Spec.SourceRepos = []string{giturl}
			Expect(k8sClient.Create(ctx, &appProject)).To(Succeed())

			By("Creating an application with a git url")
			app.Spec.Source.RepoURL = giturl
			app.Spec.Source.Path = "resources"
			Expect(k8sClient.Create(ctx, &app)).To(Succeed())

			Eventually(app, "2m", "1s").Should(
				SatisfyAll(
					appFixture.HaveSyncStatusCode(appv1.SyncStatusCodeSynced),
					appFixture.HaveHealthStatusCode(health.HealthStatusHealthy),
				))
		})

		It("Should fail if the app project specifies a git url and the application specifies an https url", func() {
			By("Creating an app project with a git url")
			appProject.Spec.SourceRepos = []string{"git@github.com:redhat-appstudio/managed-gitops.git"}
			Expect(k8sClient.Create(ctx, &appProject)).To(Succeed())

			By("Creating an application with an https url")
			app.Spec.Source.RepoURL = "https://github.com/redhat-appstudio/managed-gitops"
			Expect(k8sClient.Create(ctx, &app)).To(Succeed())

			Eventually(app, "2m", "1s").Should(
				appFixture.HaveStatusConditionMessage("application repo https://github.com/redhat-appstudio/managed-gitops is not permitted in project '" + appProject.Name + "'"),
			)
		})

		It("Should fail if the app project specifies an http url and the application specifies a git url", func() {
			By("Creating an app project with an https url")
			appProject.Spec.SourceRepos = []string{"https://github.com/redhat-appstudio/managed-gitops"}
			Expect(k8sClient.Create(ctx, &appProject)).To(Succeed())

			By("Creating an application with a git url")
			app.Spec.Source.RepoURL = "git@github.com:redhat-appstudio/managed-gitops.git"
			Expect(k8sClient.Create(ctx, &app)).To(Succeed())

			Eventually(app, "2m", "1s").Should(
				appFixture.HaveStatusConditionMessage("application repo git@github.com:redhat-appstudio/managed-gitops.git is not permitted in project '" + appProject.Name + "'"),
			)
		})
	})
})
