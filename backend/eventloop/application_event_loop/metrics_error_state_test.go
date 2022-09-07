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
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
		var sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop
		var debugContext string
		BeforeEach(func() {
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
					Name:      "my-gitops-depl",
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
				eventResourceNamespace:      "",
				workspaceClient:             k8sClient,
				log:                         log.FromContext(context.Background()),
				sharedResourceEventLoop:     shared_resource_loop.NewSharedResourceLoop(),
				workspaceID:                 workspaceID,
				testOnlySkipCreateOperation: true,
			}
			metrics.Callinit()
		})

		It("Should update existing deployment, instead of creating new.", func() {

			numberOfGitOpsDeploymentsInErrorState := testutil.ToFloat64(metrics.GitopsdeplFailures)

			By("passing the invalid GitOpsDeployment into application event reconciler, and expecting an error")

			inputChannel := make(chan *eventlooptypes.EventLoopEvent)
			informWorkCompleteChan := make(chan eventlooptypes.EventLoopMessage)
			signalledShutdown := false

			// _, _, _, _, err := appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)
			// Expect(err).ToNot(BeNil())

			// _, err := sharedutil.CatchPanic(func() error {
			// 	return processMessage(inputChannel, informWorkCompleteChan, sharedResourceEventLoop, string(gitopsDepl.UID), gitopsDepl.Namespace, debugContext, signalledShutdown)
			// })

			err = processMessage(inputChannel, informWorkCompleteChan, sharedResourceEventLoop, string(gitopsDepl.UID), "", debugContext, signalledShutdown)
			Expect(err).ToNot(BeNil())

			newNumberOfGitOpsDeploymentsInErrorState := testutil.ToFloat64(metrics.GitopsdeplFailures)

			Expect(newNumberOfGitOpsDeploymentsInErrorState).To(Equal(numberOfGitOpsDeploymentsInErrorState + 1))

			By("deleting the invalid GitOpsDeployment and calling deploymentModified again")

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())
			_, _, _, _, err = appEventLoopRunnerAction.applicationEventRunner_handleDeploymentModified(ctx, dbQueries)

			newNumberOfGitOpsDeploymentsInErrorState = testutil.ToFloat64(metrics.GitopsdeplFailures)

			Expect(newNumberOfGitOpsDeploymentsInErrorState).To(Equal(numberOfGitOpsDeploymentsInErrorState))

		})
	})
})
