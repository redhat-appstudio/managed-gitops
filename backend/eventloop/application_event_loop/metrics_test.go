package application_event_loop

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/metrics"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Test for Gitopsdeployment metrics counter", func() {

	Context("Prometheus metrics responds to count of active/failed GitopsDeployments", func() {
		var err error
		var workspaceID string
		var ctx context.Context
		var scheme *runtime.Scheme
		var workspace *corev1.Namespace
		var argocdNamespace *corev1.Namespace
		var dbQueries db.AllDatabaseQueries
		var k8sClientOuter client.WithWatch
		var k8sClient *sharedutil.ProxyClient
		var kubesystemNamespace *corev1.Namespace
		var informer sharedutil.ListEventReceiver
		var gitopsDepl *managedgitopsv1alpha1.GitOpsDeployment
		var appEventLoopRunnerAction applicationEventLoopRunner_Action

		It("Should update existing deployment, instead of creating new.", func() {
			metrics.ClearMetrics()

			ctx = context.Background()
			informer = sharedutil.ListEventReceiver{}

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err = tests.GenericTestSetup()
			Expect(err).To(BeNil())

			workspaceID = string(workspace.UID)

			gitopsDepl = &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/abc-org/abc-repo",
						Path:           "/abc-path",
						TargetRevision: "abc-commit"},
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
					Destination: managedgitopsv1alpha1.ApplicationDestination{
						Namespace:   "abc-namespace",
						Environment: "does-not-exist",
					},
				},
			}

			k8sClientOuter = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()

			k8sClient = &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			appEventLoopRunnerAction = applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}

			numberOfGitOpsDeploymentsInErrorState := testutil.ToFloat64(metrics.GitopsdeplFailures)
			totalNumberOfGitOpsDeploymentMetrics := testutil.ToFloat64(metrics.Gitopsdepl)

			By("passing the invalid GitOpsDeployment into application event reconciler, and expecting an error")

			eventLoopEvent := eventlooptypes.EventLoopEvent{
				EventType: eventlooptypes.DeploymentModified,
				Request: reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: gitopsDepl.Namespace,
					Name:      gitopsDepl.Name,
				}},
				Client:      k8sClient,
				ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
				WorkspaceID: workspaceID,
			}

			shutdownSignalled, err := handleDeploymentModified(ctx, &eventLoopEvent, appEventLoopRunnerAction, dbQueries, log.FromContext(context.Background()))
			Expect(shutdownSignalled).To(BeFalse())

			Expect(err).ToNot(BeNil())
			newTotalNumberOfGitOpsDeploymentMetrics := testutil.ToFloat64(metrics.Gitopsdepl)
			newNumberOfGitOpsDeploymentsInErrorState := testutil.ToFloat64(metrics.GitopsdeplFailures)
			Expect(newTotalNumberOfGitOpsDeploymentMetrics).To(Equal(totalNumberOfGitOpsDeploymentMetrics + 1))
			Expect(newNumberOfGitOpsDeploymentsInErrorState).To(Equal(numberOfGitOpsDeploymentsInErrorState + 1))

			By("deleting the invalid GitOpsDeployment and calling deploymentModified again")

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())
			shutdownSignalled, err = handleDeploymentModified(ctx, &eventLoopEvent, appEventLoopRunnerAction, dbQueries, log.FromContext(context.Background()))
			Expect(shutdownSignalled).To(BeFalse())
			Expect(err).To(BeNil())

			newNumberOfGitOpsDeploymentsInErrorState = testutil.ToFloat64(metrics.GitopsdeplFailures)
			newTotalNumberOfGitOpsDeploymentMetrics = testutil.ToFloat64(metrics.Gitopsdepl)
			Expect(newNumberOfGitOpsDeploymentsInErrorState).To(Equal(numberOfGitOpsDeploymentsInErrorState))
			Expect(newTotalNumberOfGitOpsDeploymentMetrics).To(Equal(numberOfGitOpsDeploymentsInErrorState))

		})

		It("Should succeed in creating a valid GitOpsDeployment", func() {
			metrics.ClearMetrics()

			ctx = context.Background()
			informer = sharedutil.ListEventReceiver{}

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err = tests.GenericTestSetup()
			Expect(err).To(BeNil())

			workspaceID = string(workspace.UID)

			gitopsDepl = &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/abc-org/abc-repo",
						Path:           "/abc-path",
						TargetRevision: "abc-commit"},
					Type: managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
					Destination: managedgitopsv1alpha1.ApplicationDestination{
						Namespace: "abc-namespace",
					},
				},
			}

			k8sClientOuter = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).
				Build()

			k8sClient = &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, false)
			Expect(err).To(BeNil())

			appEventLoopRunnerAction = applicationEventLoopRunner_Action{
				getK8sClientForGitOpsEngineInstance: func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
					return k8sClient, nil
				},
				eventResourceName:           gitopsDepl.Name,
				eventResourceNamespace:      gitopsDepl.Namespace,
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
				k8sClientFactory: MockSRLK8sClientFactory{
					fakeClient: k8sClient,
				},
			}

			numberOfGitOpsDeploymentsInErrorState := testutil.ToFloat64(metrics.GitopsdeplFailures)
			totalNumberOfGitOpsDeploymentMetrics := testutil.ToFloat64(metrics.Gitopsdepl)

			By("passing a valid GitOpsDeployment into application event reconciler, and expecting it to not give an error")

			eventLoopEvent := eventlooptypes.EventLoopEvent{
				EventType: eventlooptypes.DeploymentModified,
				Request: reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: gitopsDepl.Namespace,
					Name:      gitopsDepl.Name,
				}},
				Client:      k8sClient,
				ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
				WorkspaceID: workspaceID,
			}

			shutdownSignalled, err := handleDeploymentModified(ctx, &eventLoopEvent, appEventLoopRunnerAction, dbQueries, log.FromContext(context.Background()))
			Expect(shutdownSignalled).To(BeFalse())

			Expect(err).To(BeNil())
			newTotalNumberOfGitOpsDeploymentMetrics := testutil.ToFloat64(metrics.Gitopsdepl)
			newNumberOfGitOpsDeploymentsInErrorState := testutil.ToFloat64(metrics.GitopsdeplFailures)
			Expect(newTotalNumberOfGitOpsDeploymentMetrics).To(Equal(totalNumberOfGitOpsDeploymentMetrics + 1))
			Expect(newNumberOfGitOpsDeploymentsInErrorState).To(Equal(numberOfGitOpsDeploymentsInErrorState))

			By("deleting the GitOpsDeployment and calling deploymentModified again")

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())
			shutdownSignalled, err = handleDeploymentModified(ctx, &eventLoopEvent, appEventLoopRunnerAction, dbQueries, log.FromContext(context.Background()))
			Expect(shutdownSignalled).To(BeFalse())
			Expect(err).To(BeNil())

			newTotalNumberOfGitOpsDeploymentMetrics = testutil.ToFloat64(metrics.Gitopsdepl)
			newNumberOfGitOpsDeploymentsInErrorState = testutil.ToFloat64(metrics.GitopsdeplFailures)
			Expect(newTotalNumberOfGitOpsDeploymentMetrics).To(Equal(totalNumberOfGitOpsDeploymentMetrics))
			Expect(newNumberOfGitOpsDeploymentsInErrorState).To(Equal(numberOfGitOpsDeploymentsInErrorState))
		})
	})
})
