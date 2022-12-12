package core

import (
	"context"
	"time"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/utils"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	appFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/application"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("GitOpsDeploymentSyncRun E2E tests", func() {
	Context("Create, update and delete GitOpsDeploymentSyncRun", func() {

		var (
			ctx                      context.Context
			k8sClient                client.Client
			gitOpsDeploymentResource managedgitopsv1alpha1.GitOpsDeployment
		)

		BeforeEach(func() {
			ctx = context.Background()

			var err error

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("create a GitOpsDeployment with 'Manual' sync policy")
			gitOpsDeploymentResource = buildGitOpsDeploymentResource(name,
				repoURL, "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual)
			gitOpsDeploymentResource.Spec.Destination.Environment = ""
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			err = k8sClient.Create(ctx, &gitOpsDeploymentResource)
			Expect(err).To(BeNil())

			By("check if the GitOpsDeployment is OutOfSync")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeOutOfSync),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeMissing),
				),
			)
		})

		It("creating a new GitOpsDeploymentSyncRun should sync an Argo CD Application", func() {

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := buildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err := k8sClient.Create(ctx, &syncRunCR)
			Expect(err).To(BeNil())

			By("check if the GitOpsDeployment is Synced and Healthy")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)
		})

		It("updating the spec of an existing GitOpsDeploymentSyncRun CR should not trigger a new sync", func() {

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := buildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err := k8sClient.Create(ctx, &syncRunCR)
			Expect(err).To(BeNil())

			By("check if the GitOpsDeployment is Synced and Healthy")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			By("update the revision field of GitOpsDeploymentSyncRun and verify that there is no Sync")
			syncRunCR.Spec.RevisionID = "xyz"
			err = k8sClient.Update(ctx, &syncRunCR)
			Expect(err).To(BeNil())

			By("verify there is no new sync triggered by checking history")
			app := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocd.GenerateArgoCDApplicationName(string(gitOpsDeploymentResource.UID)),
					Namespace: "gitops-service-argocd",
				},
			}

			Consistently(func() int {
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
				if err == nil {
					return len(app.Status.History)
				}
				return 0
			}, "20s", "1s").Should(Equal(1))

		})

		It("applying the same CR with no changes should not trigger a new sync", func() {
			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := buildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err := k8sClient.Create(ctx, &syncRunCR)
			Expect(err).To(BeNil())

			By("check if the GitOpsDeployment is Synced and Healthy")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			By("update the SyncRun CR with no changes")
			err = k8sClient.Update(ctx, &syncRunCR)
			Expect(err).To(BeNil())

			By("verify that there is no new sync triggered by checking history")
			app := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocd.GenerateArgoCDApplicationName(string(gitOpsDeploymentResource.UID)),
					Namespace: "gitops-service-argocd",
				},
			}

			Consistently(func() int {
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
				if err == nil {
					return len(app.Status.History)
				}
				return 0
			}, "20s", "1s").Should(Equal(1))
		})

		It("deleting the GitOpsDeploymentSyncRun should terminate a running Sync operation", func() {
			gitOpsDeploymentResource = buildGitOpsDeploymentResource("test-deply-with-presync",
				"https://github.com/managed-gitops-test-data/deployment-presync-hook", "guestbook",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual)
			gitOpsDeploymentResource.Spec.Destination.Environment = ""
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			err := k8sClient.Create(ctx, &gitOpsDeploymentResource)
			Expect(err).To(BeNil())

			By("check if the GitOpsDeployment is OutOfSync")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeOutOfSync),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeMissing),
				),
			)

			By("configure the Application to get stuck in 'Syncing' state")
			argocdCM := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd-cm",
					Namespace: "gitops-service-argocd",
				},
			}

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&argocdCM), &argocdCM)
			Expect(err).To(BeNil())

			addCustomHealthCheckForDeployment(ctx, k8sClient, &argocdCM)

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := buildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err = k8sClient.Create(ctx, &syncRunCR)
			Expect(err).To(BeNil())

			By("verify whether the Application is stuck in 'Syncing' state")
			app := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocd.GenerateArgoCDApplicationName(string(gitOpsDeploymentResource.UID)),
					Namespace: "gitops-service-argocd",
				},
			}

			opState := appv1.OperationState{
				Phase:   "Running",
				Message: "waiting for completion of hook apps/Deployment/guestbook-ui",
			}

			syncState := appv1.ApplicationStatus{
				Sync: appv1.SyncStatus{
					Status: appv1.SyncStatusCodeOutOfSync,
				},
			}

			Eventually(app, "60s", "1s").Should(
				SatisfyAll(
					appFixture.HaveSyncStatusCode(syncState),
					appFixture.HaveOperationState(opState),
				),
			)

			By("delete the GitOpsDeploymentSyncRun and verify if the Sync operation is terminated")
			err = k8sClient.Delete(ctx, &syncRunCR)
			Expect(err).To(BeNil())

			opState.Phase = "Failed"
			opState.Message = "Operation terminated"

			Eventually(app, "3m", "1s").Should(
				SatisfyAll(
					appFixture.HaveSyncStatusCode(syncState),
					appFixture.HaveOperationState(opState),
				),
			)

			By("revert the changes done to the argocd-cm configmap")
			removeCustomHealthCheckForDeployment(ctx, k8sClient, &argocdCM)
		})

		It("deleting the GitOpsDeploymentSyncRun CR should not terminate if no Sync operation is in progress", func() {

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := buildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err := k8sClient.Create(ctx, &syncRunCR)
			Expect(err).To(BeNil())

			By("check if the GitOpsDeployment is Synced and Healthy")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			By("record the time at which the operation was finished")
			app := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocd.GenerateArgoCDApplicationName(string(gitOpsDeploymentResource.UID)),
					Namespace: "gitops-service-argocd",
				},
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
			Expect(err).To(BeNil())

			finishedAt := app.Status.OperationState.FinishedAt

			By("delete the GitOpsDeploymentSyncRun and verify if the operation is not terminated")
			err = k8sClient.Delete(ctx, &syncRunCR)
			Expect(err).To(BeNil())

			By("verify whether the finishedAt time hasn't changed after deletion")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
			Expect(err).To(BeNil())

			Consistently(func() bool {
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
				Expect(err).To(BeNil())

				return app.Status.OperationState.FinishedAt.Equal(finishedAt)
			}, "20s", "1s").Should(Equal(true))

		})

		It("should retry if the sync operation failed", func() {

			// When the Application is already stuck in 'Syncing' state, creating new GitOpsDeploymentSyncRuns
			// will fail, since there is an operation in progress. In this scenario, the service should retry
			// syncing the Application with exponential backoff

			gitOpsDeploymentResource = buildGitOpsDeploymentResource("test-deply-with-presync",
				"https://github.com/managed-gitops-test-data/deployment-presync-hook", "guestbook",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual)
			gitOpsDeploymentResource.Spec.Destination.Environment = ""
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			err := k8sClient.Create(ctx, &gitOpsDeploymentResource)
			Expect(err).To(BeNil())

			By("check if the GitOpsDeployment is OutOfSync")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeOutOfSync),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeMissing),
				),
			)

			By("configure the Application to get stuck in 'Syncing' state")
			argocdCM := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd-cm",
					Namespace: "gitops-service-argocd",
				},
			}

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&argocdCM), &argocdCM)
			Expect(err).To(BeNil())

			addCustomHealthCheckForDeployment(ctx, k8sClient, &argocdCM)

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := buildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err = k8sClient.Create(ctx, &syncRunCR)
			Expect(err).To(BeNil())

			By("verify whether the Application is stuck in 'Syncing' state")
			app := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocd.GenerateArgoCDApplicationName(string(gitOpsDeploymentResource.UID)),
					Namespace: "gitops-service-argocd",
				},
			}

			opState := appv1.OperationState{
				Phase:   "Running",
				Message: "waiting for completion of hook apps/Deployment/guestbook-ui",
			}

			syncState := appv1.ApplicationStatus{
				Sync: appv1.SyncStatus{
					Status: appv1.SyncStatusCodeOutOfSync,
				},
			}

			Eventually(app, "60s", "1s").Should(
				SatisfyAll(
					appFixture.HaveSyncStatusCode(syncState),
					appFixture.HaveOperationState(opState),
				),
			)

			By("create a new GitOpsDeploymentSyncRun and ensure that the sync status hasn't changed")
			newSyncRunCR := buildGitOpsDeploymentSyncRunResource("test-syncrun-1", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err = k8sClient.Create(ctx, &newSyncRunCR)
			Expect(err).To(BeNil())

			Consistently(app, "20s", "1s").Should(
				SatisfyAll(
					appFixture.HaveSyncStatusCode(syncState),
					appFixture.HaveOperationState(opState),
				),
			)

			By("terminate the running sync operation")
			removeCustomHealthCheckForDeployment(ctx, k8sClient, &argocdCM)

			argocdNS := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: app.Namespace,
				},
			}

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&argocdNS), &argocdNS)
			Expect(err).To(BeNil())

			cs := utils.NewCredentialService(nil, false)

			err = utils.TerminateOperation(ctx, app.Name, argocdNS, cs, k8sClient, 5*time.Minute, log.FromContext(ctx))
			Expect(err).To(BeNil())

			By("since the operation is terminated, ensure that the service has retried syncing the previous GitOpsDeploymentSyncRun")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)
		})

		It("should handle multiple GitOpsDeploymentSyncRun CRs sequentially", func() {
			gitOpsDeploymentResource = buildGitOpsDeploymentResource("test-deply",
				"https://github.com/managed-gitops-test-data/deployment-presync-hook", "guestbook-without-hook",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual)
			gitOpsDeploymentResource.Spec.Destination.Environment = ""
			gitOpsDeploymentResource.Spec.Destination.Namespace = fixture.GitOpsServiceE2ENamespace

			err := k8sClient.Create(ctx, &gitOpsDeploymentResource)
			Expect(err).To(BeNil())

			By("create multiple SyncRun CRs with different revisions sequentially")

			syncRunCRs := []struct {
				name     string
				ref      string
				revision string
			}{
				{name: "test-syncrun-1", ref: "main", revision: "d390b220553954a12a19b5133ad2313e6b60691e"},
				{name: "test-syncrun-2", ref: "replicas-2", revision: "4403172251fef9664b2b91158005c6b00e0183e4"},
				{name: "test-syncrun-3", ref: "replicas-3", revision: "06e5ee7f57516955cdcc1ea44dccbb5ccba83b56"},
			}

			for _, cr := range syncRunCRs {
				syncRunCR := buildGitOpsDeploymentSyncRunResource(cr.name, fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, cr.ref)

				err := k8sClient.Create(ctx, &syncRunCR)
				Expect(err).To(BeNil())
			}

			app := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocd.GenerateArgoCDApplicationName(string(gitOpsDeploymentResource.UID)),
					Namespace: "gitops-service-argocd",
				},
			}

			By("verify if the GitOpsDeployment is Synced and Healthy")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			By("verify if the number of sync operations is equal to the number of GitOpsDeploymentSynRuns")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&app), &app)
				if err != nil {
					return false
				}
				return len(app.Status.History) == len(syncRunCRs)
			}, "3m", "1s").Should(BeTrue())

			By("verify if the order of sync operations matches the order of GitOpsDeployment CRs")
			for i, cr := range syncRunCRs {
				Expect(app.Status.History[i].Revision).Should(Equal(cr.revision))
			}
		})
	})
})

func addCustomHealthCheckForDeployment(ctx context.Context, k8sClient client.Client, argocdCM *corev1.ConfigMap) {
	argocdCM.Data["resource.customizations.health.apps_Deployment"] = `hs = {}
hs.status = "Progressing"
hs.message = "Custom health check to test Sync operation"
return hs`

	err := k8sClient.Update(ctx, argocdCM)
	Expect(err).To(BeNil())
}

func removeCustomHealthCheckForDeployment(ctx context.Context, k8sClient client.Client, argocdCM *corev1.ConfigMap) {
	delete(argocdCM.Data, "resource.customizations.health.apps_Deployment")
	err := k8sClient.Update(ctx, argocdCM)
	Expect(err).To(BeNil())
}

func buildGitOpsDeploymentSyncRunResource(name, ns, deplyName, revision string) managedgitopsv1alpha1.GitOpsDeploymentSyncRun {
	return managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
			GitopsDeploymentName: deplyName,
			RevisionID:           revision,
		},
	}
}
