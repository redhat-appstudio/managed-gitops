package core

import (
	"context"

	argocdoperatorv1alpha1 "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	appEventLoop "github.com/redhat-appstudio/managed-gitops/backend/eventloop/application_event_loop"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	appFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/application"
	gitopsDeplFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeployment"
	syncRunFixture "github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/gitopsdeploymentsyncrun"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("GitOpsDeploymentSyncRun E2E tests", func() {

	const (
		name    = "my-gitops-depl"
		repoURL = "https://github.com/redhat-appstudio/managed-gitops"
	)

	Context("Create, update and delete GitOpsDeploymentSyncRun", func() {

		var (
			ctx                      context.Context
			k8sClient                client.Client
			gitOpsDeploymentResource managedgitopsv1alpha1.GitOpsDeployment
		)

		argocdCR := &argocdoperatorv1alpha1.ArgoCD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitops-service-argocd",
				Namespace: "gitops-service-argocd",
			},
		}

		BeforeEach(func() {
			ctx = context.Background()

			var err error

			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			k8sClient, err = fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("create a GitOpsDeployment with 'Manual' sync policy")
			gitOpsDeploymentResource = gitopsDeplFixture.BuildGitOpsDeploymentResource(name,
				repoURL, "resources/test-data/sample-gitops-repository/environments/overlays/dev",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual)

			err = k8sClient.Create(ctx, &gitOpsDeploymentResource)
			Expect(err).ToNot(HaveOccurred())

			By("check if the GitOpsDeployment is OutOfSync")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeOutOfSync),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeMissing),
				),
			)
		})

		AfterEach(func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(argocdCR), argocdCR)
			Expect(err == nil || apierr.IsNotFound(err)).To(BeTrue())

			removeCustomHealthCheckForDeployment(ctx, k8sClient, argocdCR)
		})

		It("creating a new GitOpsDeploymentSyncRun should sync an Argo CD Application", func() {

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := syncRunFixture.BuildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err := k8sClient.Create(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			By("check if the GitOpsDeployment is Synced and Healthy")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			By("check if GitOpsDeploymentSyncRun is updated with the right conditions")
			conditions := []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
				getDefaultSyncRunCondition(),
			}
			Eventually(syncRunCR, "60s", "1s").Should(SatisfyAll(syncRunFixture.HaveConditions(conditions)))

		})

		It("updating the spec of an existing GitOpsDeploymentSyncRun CR should not trigger a new sync", func() {

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := syncRunFixture.BuildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err := k8sClient.Create(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			By("check if the GitOpsDeployment is Synced and Healthy")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			By("update the revision field of GitOpsDeploymentSyncRun and verify that there is no Sync")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&syncRunCR), &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			syncRunCR.Spec.RevisionID = "xyz"
			err = k8sClient.Update(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			By("check if GitOpsDeploymentSyncRun status is updated with the error")
			conditions := []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
				{
					Type:    managedgitopsv1alpha1.GitOpsDeploymentSyncRunConditionErrorOccurred,
					Message: appEventLoop.ErrRevisionIsImmutable,
					Reason:  getDefaultSyncRunReason(),
					Status:  managedgitopsv1alpha1.GitOpsConditionStatusTrue,
				},
			}
			Eventually(syncRunCR, "60s", "1s").Should(SatisfyAll(syncRunFixture.HaveConditions(conditions)))

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
			syncRunCR := syncRunFixture.BuildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err := k8sClient.Create(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			By("check if GitOpsDeploymentSyncRun is updated with the right condition")
			conditions := []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
				getDefaultSyncRunCondition(),
			}
			Eventually(syncRunCR, "60s", "1s").Should(SatisfyAll(syncRunFixture.HaveConditions(conditions)))

			By("check if the GitOpsDeployment is Synced and Healthy")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			By("update the SyncRun CR with no changes")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&syncRunCR), &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Update(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			By("check if GitOpsDeploymentSyncRun conditions is not updated twice")
			Eventually(syncRunCR, "60s", "1s").Should(SatisfyAll(syncRunFixture.HaveConditions(conditions)))

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
			gitOpsDeploymentResource = gitopsDeplFixture.BuildGitOpsDeploymentResource("test-deply-with-presync",
				"https://github.com/managed-gitops-test-data/deployment-presync-hook", "guestbook",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual)

			err := k8sClient.Create(ctx, &gitOpsDeploymentResource)
			Expect(err).ToNot(HaveOccurred())

			By("check if the GitOpsDeployment is OutOfSync")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeOutOfSync),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeMissing),
				),
			)

			By("configure the Application to get stuck in 'Syncing' state")

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(argocdCR), argocdCR)
			Expect(err).ToNot(HaveOccurred())

			addCustomHealthCheckForDeployment(ctx, k8sClient, argocdCR)

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := syncRunFixture.BuildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err = k8sClient.Create(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

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

			Eventually(app, "60s", "1s").Should(
				SatisfyAll(
					appFixture.HaveSyncStatusCode(appv1.SyncStatusCodeOutOfSync),
					appFixture.HaveOperationState(opState),
				),
			)

			By("delete the GitOpsDeploymentSyncRun and verify if the Sync operation is terminated")
			err = k8sClient.Delete(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			opState.Phase = "Failed"
			opState.Message = "Operation terminated"

			Eventually(app, "3m", "1s").Should(
				SatisfyAll(
					appFixture.HaveSyncStatusCode(appv1.SyncStatusCodeOutOfSync),
					appFixture.HaveOperationState(opState),
				),
			)

			By("revert the changes done to the argocd-cm configmap")
			removeCustomHealthCheckForDeployment(ctx, k8sClient, argocdCR)
		})

		It("deleting the GitOpsDeploymentSyncRun CR should not terminate if no Sync operation is in progress", func() {

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := syncRunFixture.BuildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err := k8sClient.Create(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			By("check if GitOpsDeploymentSyncRun is updated with the right condition")
			conditions := []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{getDefaultSyncRunCondition()}
			Eventually(syncRunCR, "60s", "1s").Should(SatisfyAll(syncRunFixture.HaveConditions(conditions)))

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
			Expect(err).ToNot(HaveOccurred())

			finishedAt := app.Status.OperationState.FinishedAt

			By("delete the GitOpsDeploymentSyncRun and verify if the operation is not terminated")
			err = k8sClient.Delete(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			By("verify whether the finishedAt time hasn't changed after deletion")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
			Expect(err).ToNot(HaveOccurred())

			Consistently(func() bool {
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
				Expect(err).ToNot(HaveOccurred())

				return app.Status.OperationState.FinishedAt.Equal(finishedAt)
			}, "20s", "1s").Should(BeTrue())

		})

		It("should sync if the previous sync operation is terminated", func() {

			gitOpsDeploymentResource = gitopsDeplFixture.BuildGitOpsDeploymentResource("test-deply-with-presync",
				"https://github.com/managed-gitops-test-data/deployment-presync-hook", "guestbook",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual)

			err := k8sClient.Create(ctx, &gitOpsDeploymentResource)
			Expect(err).ToNot(HaveOccurred())

			By("check if the GitOpsDeployment is OutOfSync")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeOutOfSync),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeMissing),
				),
			)

			By("configure the Application to get stuck in 'Syncing' state")

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(argocdCR), argocdCR)
			Expect(err).ToNot(HaveOccurred())

			addCustomHealthCheckForDeployment(ctx, k8sClient, argocdCR)

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := syncRunFixture.BuildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err = k8sClient.Create(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

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

			Eventually(app, "60s", "1s").Should(
				SatisfyAll(
					appFixture.HaveSyncStatusCode(appv1.SyncStatusCodeOutOfSync),
					appFixture.HaveOperationState(opState),
				),
			)

			By("terminate the running sync operation by deleting the old sync run CR")
			removeCustomHealthCheckForDeployment(ctx, k8sClient, argocdCR)

			err = k8sClient.Delete(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			By("create a new GitOpsDeploymentSyncRun and ensure that the sync status hasn't changed")
			newSyncRunCR := syncRunFixture.BuildGitOpsDeploymentSyncRunResource("test-syncrun-1", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err = k8sClient.Create(ctx, &newSyncRunCR)
			Expect(err).ToNot(HaveOccurred())

			By("check if GitOpsDeploymentSyncRun is updated with the right condition")
			conditions := []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
				getDefaultSyncRunCondition(),
			}
			Eventually(newSyncRunCR, "60s", "1s").Should(SatisfyAll(syncRunFixture.HaveConditions(conditions)))

			By("ensure that the new SyncRun CR is processed successfully")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)
		})

		It("should handle multiple GitOpsDeploymentSyncRun CRs sequentially", func() {
			gitOpsDeploymentResource = gitopsDeplFixture.BuildGitOpsDeploymentResource("test-deply",
				"https://github.com/managed-gitops-test-data/deployment-presync-hook", "guestbook-without-hook",
				managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual)

			err := k8sClient.Create(ctx, &gitOpsDeploymentResource)
			Expect(err).ToNot(HaveOccurred())

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
				syncRunCR := syncRunFixture.BuildGitOpsDeploymentSyncRunResource(cr.name, fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, cr.ref)

				err := k8sClient.Create(ctx, &syncRunCR)
				Expect(err).ToNot(HaveOccurred())

				By("check if GitOpsDeploymentSyncRun is updated with the right condition")
				conditions := []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
					getDefaultSyncRunCondition(),
				}
				Eventually(syncRunCR, "60s", "1s").Should(SatisfyAll(syncRunFixture.HaveConditions(conditions)))
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

		It("should refresh before syncing the Application", func() {
			By("add refresh annotation and verify if it'll be overwritten after refresh")
			app := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocd.GenerateArgoCDApplicationName(string(gitOpsDeploymentResource.UID)),
					Namespace: "gitops-service-argocd",
				},
			}

			By("create a GitOpsDeploymentSyncRun")
			syncRunCR := syncRunFixture.BuildGitOpsDeploymentSyncRunResource("test-syncrun", fixture.GitOpsServiceE2ENamespace, gitOpsDeploymentResource.Name, "main")

			err := k8sClient.Create(ctx, &syncRunCR)
			Expect(err).ToNot(HaveOccurred())

			By("check if the GitOpsDeployment is Synced and Healthy")
			Eventually(gitOpsDeploymentResource, ArgoCDReconcileWaitTime, "1s").Should(
				SatisfyAll(
					gitopsDeplFixture.HaveSyncStatusCode(managedgitopsv1alpha1.SyncStatusCodeSynced),
					gitopsDeplFixture.HaveHealthStatusCode(managedgitopsv1alpha1.HeathStatusCodeHealthy),
				),
			)

			By("check if GitOpsDeploymentSyncRun is updated with the right conditions")
			conditions := []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
				getDefaultSyncRunCondition(),
			}
			Eventually(syncRunCR, "60s", "1s").Should(SatisfyAll(syncRunFixture.HaveConditions(conditions)))

			By("verify if the refresh annotation is removed by Argo CD")
			Eventually(func() bool {
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
				if err != nil {
					return false
				}
				_, found := app.Annotations[appv1.AnnotationKeyRefresh]
				return !found
			}, ArgoCDReconcileWaitTime, "1s").Should(BeTrue())
		})
	})
})

func addCustomHealthCheckForDeployment(ctx context.Context, k8sClient client.Client, argocdCR *argocdoperatorv1alpha1.ArgoCD) {
	healthStatusMessage := `hs = {}
hs.status = "Progressing"
hs.message = "Custom health check to test Sync operation"
return hs`
	err := k8s.UntilSuccess(k8sClient, func(k8sClient client.Client) error {
		if err := k8s.Get(argocdCR, k8sClient); err != nil {
			return err
		}
		argocdCR.Spec.ResourceHealthChecks = []argocdoperatorv1alpha1.ResourceHealthCheck{
			{
				Group: "apps",
				Kind:  "Deployment",
				Check: healthStatusMessage,
			},
		}
		return k8sClient.Update(ctx, argocdCR)
	})
	Expect(err).ToNot(HaveOccurred())
}

func removeCustomHealthCheckForDeployment(ctx context.Context, k8sClient client.Client, argocdCR *argocdoperatorv1alpha1.ArgoCD) {
	err := k8s.UntilSuccess(k8sClient, func(k8sClient client.Client) error {
		if err := k8s.Get(argocdCR, k8sClient); err != nil {
			return err
		}
		argocdCR.Spec.ResourceHealthChecks = []argocdoperatorv1alpha1.ResourceHealthCheck{}
		return k8sClient.Update(ctx, argocdCR)
	})
	Expect(err).ToNot(HaveOccurred())
}

func getDefaultSyncRunReason() managedgitopsv1alpha1.SyncRunReasonType {
	return managedgitopsv1alpha1.SyncRunReasonType(
		managedgitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred,
	)
}

func getDefaultSyncRunCondition() managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition {
	return managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
		Type:    managedgitopsv1alpha1.GitOpsDeploymentSyncRunConditionErrorOccurred,
		Message: "",
		Reason:  "",
		Status:  managedgitopsv1alpha1.GitOpsConditionStatusFalse,
	}
}
