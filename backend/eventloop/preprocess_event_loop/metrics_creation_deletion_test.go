package preprocess_event_loop

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/metrics"

	"github.com/redhat-appstudio/managed-gitops/backend/eventloop"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Preprocess Event Loop Test", func() {

	FContext("Preprocess event loop responds to GitOpsDeployment channel events", func() {

		var err error
		var ctx context.Context
		var scheme *runtime.Scheme
		var argocdNamespace *corev1.Namespace
		var kubesystemNamespace *corev1.Namespace
		var namespace *corev1.Namespace
		var dbQueries db.AllDatabaseQueries

		var k8sClient client.Client

		BeforeEach(func() {

			ctx = context.Background()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			scheme, argocdNamespace, kubesystemNamespace, namespace, err = tests.GenericTestSetup()
			Expect(err).To(BeNil())

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, _, _, _, _, err := db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace, argocdNamespace, kubesystemNamespace).Build()

		})

		It("Should pass events received on input channel to application event loop", func() {

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: namespace.Name,
					UID:       uuid.NewUUID(),
				},
			}
			totalNumberOfGitOpsDeploymentMetrics := testutil.ToFloat64(metrics.Gitopsdepl)
			err = k8sClient.Create(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			fakeControllerEventLoopChannel := make(chan eventlooptypes.EventLoopEvent)
			fakeControllerEventLoop := eventloop.ControllerEventLoop{
				EventLoopInputChannel: fakeControllerEventLoopChannel,
			}

			channel := make(chan eventlooptypes.EventLoopEvent)

			go preprocessEventLoopRouter(channel, &fakeControllerEventLoop)

			event := buildGitOpsDeplEventLoopTestEvent(*gitopsDepl, k8sClient, *namespace)

			fmt.Println("Sending event", event)
			channel <- event

			fmt.Println("Waiting for response")
			response := <-fakeControllerEventLoop.EventLoopInputChannel
			Expect(response.Request).Should(Equal(event.Request))
			Expect(response.EventType).Should(Equal(event.EventType))
			Expect(response.WorkspaceID).Should(Equal(event.WorkspaceID))
			fmt.Println("Received response", response)

			newTotalNumberOfGitOpsDeploymentMetrics := testutil.ToFloat64(metrics.Gitopsdepl)
			fmt.Println("M E T R I C S ->  ", testutil.ToFloat64(metrics.Gitopsdepl))
			Expect(newTotalNumberOfGitOpsDeploymentMetrics).To(Equal(totalNumberOfGitOpsDeploymentMetrics + 1))

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())
			event = buildGitOpsDeplEventLoopTestEvent(*gitopsDepl, k8sClient, *namespace)
			fmt.Println("Sending event", event)
			channel <- event
			fmt.Println("Received response")
			response = <-fakeControllerEventLoop.EventLoopInputChannel

			newTotalNumberOfGitOpsDeploymentMetrics = testutil.ToFloat64(metrics.Gitopsdepl)
			fmt.Println("M E T R I C S ->  ", testutil.ToFloat64(metrics.Gitopsdepl))
			Expect(newTotalNumberOfGitOpsDeploymentMetrics).To(Equal(totalNumberOfGitOpsDeploymentMetrics))

			Expect(response.Request).Should(Equal(event.Request))
			Expect(response.EventType).Should(Equal(event.EventType))
			Expect(response.WorkspaceID).Should(Equal(event.WorkspaceID))
			fmt.Printf("response: %v\n", response)

		})
	})
})
