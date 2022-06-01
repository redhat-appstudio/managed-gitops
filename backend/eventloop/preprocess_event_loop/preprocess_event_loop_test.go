package preprocess_event_loop

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Preprocess Event Loop Test", func() {

	Context("Preprocess event loop responds to GitOpsDeployment channel events", func() {

		var err error
		var ctx context.Context
		var scheme *runtime.Scheme
		var argocdNamespace *corev1.Namespace
		var kubesystemNamespace *corev1.Namespace
		var namespace *corev1.Namespace
		var dbQueries db.AllDatabaseQueries
		var managedEnv *db.ManagedEnvironment
		var engineInstance *db.GitopsEngineInstance

		var k8sClient client.Client

		BeforeEach(func() {

			ctx = context.Background()

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			scheme, argocdNamespace, kubesystemNamespace, namespace, err = eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, managedEnv, _, engineInstance, _, err = db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace, argocdNamespace, kubesystemNamespace).Build()

			err = nil

		})

		It("Should pass events received on input channel to application event loop", func() {

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: namespace.Name,
					UID:       uuid.NewUUID(),
				},
			}

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

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			event = buildGitOpsDeplEventLoopTestEvent(*gitopsDepl, k8sClient, *namespace)

			fmt.Println("Sending event", event)
			channel <- event
			fmt.Println("Received response")
			response = <-fakeControllerEventLoop.EventLoopInputChannel
			Expect(response.Request).Should(Equal(event.Request))
			Expect(response.EventType).Should(Equal(event.EventType))
			Expect(response.WorkspaceID).Should(Equal(event.WorkspaceID))
			fmt.Printf("response: %v\n", response)

		})

		It("should report 2 GitOpsDeployment events, if a GitOpsDeployment is created, then deleted (without the preprocess event loop processing the event), then created with the same name/namespace", func() {

			By("creating an Application row that will be connected to our GitOpsDeployment, in DB and K8s")

			_, firstGitopsDepl, _ := createApplicationConnectedToGitOpsDeploymentForTest(ctx, namespace, engineInstance, managedEnv, k8sClient, dbQueries)

			By("starting the preprocess event loop")

			fixture := startPreprocessEventLoopRouter()

			By("deleting the GitOpsDeployment, so we can recreate it next")
			err = k8sClient.Delete(ctx, &firstGitopsDepl)
			Expect(err).To(BeNil())

			By("creating a name GitOpsDeployment with the same name/namespace/namespace uid as the old one, but with a different UID")
			secondGitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: namespace.Name,
					UID:       "test-a-different-uid",
				},
			}
			err = k8sClient.Create(ctx, secondGitopsDepl)
			Expect(err).To(BeNil())

			By("creating and send a new event to the preprocess event loop, for the new GitOpsDeployment that was created")

			event := buildGitOpsDeplEventLoopTestEvent(*secondGitopsDepl, k8sClient, *namespace)

			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			By("waiting and counting the number of events forwarded by the event loop")
			expectExactlyANumberOfEvents(2, &fixture.responsesReceived)

			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(firstGitopsDepl.GetUID())),
				"The first event received should match the first GitOpsDeployment")
			Expect(fixture.responsesReceived[1].AssociatedGitopsDeplUID).To(Equal(string(secondGitopsDepl.GetUID())),
				"The second event received should match the second GitOpsDeployment")

		})

		It("should ensure that the local, internal cache of preprocess event loop is used to find the UID of a deleted GitOpsDeployment resource, if the item exists in the local cache", func() {

			By("creating an Application row that will be connected to our GitOpsDeployment, in DB and K8s")

			_, firstGitopsDepl, _ := createApplicationConnectedToGitOpsDeploymentForTest(ctx, namespace, engineInstance, managedEnv, k8sClient, dbQueries)

			By("starting the preprocess event loop")

			fixture := startPreprocessEventLoopRouter()

			By("creating and sending a new event to the preprocess event loop, for the new GitOpsDeployment that was created")

			event := buildGitOpsDeplEventLoopTestEvent(firstGitopsDepl, k8sClient, *namespace)

			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(1, &fixture.responsesReceived)

			By("reseting the database to a fresh state")
			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			By("deleting the GitOpsDeployment, so we can recreate it next")
			err = k8sClient.Delete(ctx, &firstGitopsDepl)
			Expect(err).To(BeNil())

			By("creating a name GitOpsDeployment with the same name/namespace/namespace uid as the old one, but with a different UID")
			secondGitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: namespace.Name,
					UID:       "test-a-different-uid",
				},
			}
			err = k8sClient.Create(ctx, secondGitopsDepl)
			Expect(err).To(BeNil())

			By("sending the event again, even though the database has been deleted")

			event = buildGitOpsDeplEventLoopTestEvent(*secondGitopsDepl, k8sClient, *namespace)
			GinkgoWriter.Println("Sending event again", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(3, &fixture.responsesReceived)

			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(firstGitopsDepl.GetUID())),
				"The first event received should match the first GitOpsDeployment")
			Expect(fixture.responsesReceived[1].AssociatedGitopsDeplUID).To(Equal(string(firstGitopsDepl.GetUID())),
				"The first event received should match the first GitOpsDeployment")
			Expect(fixture.responsesReceived[2].AssociatedGitopsDeplUID).To(Equal(string(secondGitopsDepl.GetUID())),
				"The second event received should match the second GitOpsDeployment")

		})

		It("should ensure that the preprocess event loop does not output an event for a GitOpsDeployment that"+
			" doesn't exist in the database, or in the K8s namespace.", func() {

			By("simulating a deleted GitOpsDeploment K8s resource: we create the struct, but don't add it to the k8s client.")
			firstGitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: namespace.Name,
					UID:       "test-gitops-depl-uid",
				},
			}

			By("starting the preprocess event loop")
			fixture := startPreprocessEventLoopRouter()

			By("sending a new event to the preprocess event loop, for the GitOpsDeployment, that simulates a request of a deleted GitOpsDeployment resource")

			event := buildGitOpsDeplEventLoopTestEvent(*firstGitopsDepl, k8sClient, *namespace)

			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			Consistently(func() bool {
				GinkgoWriter.Println("responses received length is", len(fixture.responsesReceived))
				return len(fixture.responsesReceived) == 0
			}, "1s", "100ms").Should(Equal(true), "The preprocess event loop should not output any events, for resources to don't exist in either the database or the K8s namespace.")

		})

		It("should ensure a single event is emitted for a GitOpsDeployment is in the local cache, and the k8s resource is unmodified from the cached value", func() {

			_, gitopsDepl, _ := createApplicationConnectedToGitOpsDeploymentForTest(ctx, namespace, engineInstance, managedEnv, k8sClient, dbQueries)

			By("starting the preprocess event loop")

			fixture := startPreprocessEventLoopRouter()

			By("creating and sending a new event to the preprocess event loop, for the new GitOpsDeployment that was created")

			event := buildGitOpsDeplEventLoopTestEvent(gitopsDepl, k8sClient, *namespace)

			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(1, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))
			fixture.responsesReceived = []eventlooptypes.EventLoopEvent{}

			By("sending the same event, simulating a change to the resource")
			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(1, &fixture.responsesReceived, "ensure the second event should also be output")
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))
		})

		It("should ensure a single event is emitted for a GitOpsDeployment that doesn't exist in the namespace, "+
			"or the cache, but there does exist a DeploymentToApplicationMapping", func() {

			By("setting up a deleted GitOpsDeployment resource, with a DeploymentToApplicationMapping connecting it to an Application")
			_, gitopsDepl, _ := createApplicationConnectedToGitOpsDeploymentForTest(ctx, namespace, engineInstance, managedEnv, k8sClient, dbQueries)

			err = k8sClient.Delete(ctx, &gitopsDepl)
			Expect(err).To(BeNil())

			By("starting the preprocess event loop")
			fixture := startPreprocessEventLoopRouter()

			By("sending an event for the gitopsdeployment resource that no longer exists, but is still referenced in the DB")

			event := buildGitOpsDeplEventLoopTestEvent(gitopsDepl, k8sClient, *namespace)

			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(1, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))
			fixture.responsesReceived = []eventlooptypes.EventLoopEvent{}

		})

		It("should ensure that multiple event are emitted for a GitOpsDeployment that doesn't exist in the namespace, "+
			"or the cache, but there exist multiple DeploymentToApplicationMappings", func() {

			By("setting up a deleted GitOpsDeployment resource, with a DeploymentToApplicationMapping connecting it to an Application")
			_, gitopsDepl, _ := createApplicationConnectedToGitOpsDeploymentForTest(ctx, namespace, engineInstance, managedEnv, k8sClient, dbQueries)

			err = k8sClient.Delete(ctx, &gitopsDepl)
			Expect(err).To(BeNil())

			By("setting up a second fake deleted GitOpsDeploymrent resource, with a DTAM connecting it to a second old Application")
			secondApp := &db.Application{
				Application_id:          "test-app-id-2",
				Name:                    "my-app-2",
				Spec_field:              "{}",
				Engine_instance_inst_id: engineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnv.Managedenvironment_id,
			}
			err = dbQueries.CreateApplication(ctx, secondApp)
			Expect(err).To(BeNil())

			secondGitOpsDeplToAppMapping := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: "test-ref-to-second-deleted-gitops-depl",
				DeploymentName:                        gitopsDepl.Name,
				DeploymentNamespace:                   namespace.Name,
				NamespaceUID:                          string(namespace.UID),
				Application_id:                        secondApp.Application_id,
			}
			err = dbQueries.CreateDeploymentToApplicationMapping(ctx, secondGitOpsDeplToAppMapping)
			Expect(err).To(BeNil())

			By("starting the preprocess event loop")
			fixture := startPreprocessEventLoopRouter()

			By("sending an event for the gitopsdeployment resource that no longer exists, but is still referenced in the DB")

			event := buildGitOpsDeplEventLoopTestEvent(gitopsDepl, k8sClient, *namespace)

			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(2, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))
			Expect(fixture.responsesReceived[1].AssociatedGitopsDeplUID).To(Equal(string(secondGitOpsDeplToAppMapping.Deploymenttoapplicationmapping_uid_id)))

		})

	})

	Context("Preprocess event loop responds to GitOpsDeploymentSyncRun channel events", func() {

		var err error
		var ctx context.Context
		var scheme *runtime.Scheme
		var argocdNamespace *corev1.Namespace
		var kubesystemNamespace *corev1.Namespace
		var namespace *corev1.Namespace
		var dbQueries db.AllDatabaseQueries
		var managedEnv *db.ManagedEnvironment
		var engineInstance *db.GitopsEngineInstance

		var k8sClient client.Client

		BeforeEach(func() {

			ctx = context.Background()

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			scheme, argocdNamespace, kubesystemNamespace, namespace, err = eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, managedEnv, _, engineInstance, _, err = db.CreateSampleData(dbQueries)
			Expect(err).To(BeNil())

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace, argocdNamespace, kubesystemNamespace).Build()

			err = nil

		})

		It("Should pass events for orphaned GitOpsDeploymentSyncRuns", func() {

			By("creating a GitOpsDeploymentSyncRun pointing to a GitOpsDeployment that doesn't exist")

			gitopsDeplSyncRun := managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-sync-run",
					Namespace: namespace.Name,
					UID:       "test-gitops-depl-sync-run-uuid",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: "test-gitops-depl-doesnt-exist",
					RevisionID:           "",
				},
			}
			err = k8sClient.Create(ctx, &gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			By("starting the preprocess event loop")

			fixture := startPreprocessEventLoopRouter()

			By("sending an event for a new gitopsdeploymentsyncrun resource")

			event := buildGitOpsDeplSyncRunEventLoopTestEvent(gitopsDeplSyncRun, k8sClient, *namespace)

			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(1, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(eventloop.OrphanedResourceGitopsDeplUID)),
				"since the gitopsdeployment doesn't exist, the associated gitops depl uid should be 'orphaned'")
		})

		It("Should pass GitOpsDeploymentSyncRun events that have already been processed in the database", func() {

			By("creating an Application row that will be connected to our GitOpsDeployment, in DB and K8s")
			app, gitopsDepl, _ := createApplicationConnectedToGitOpsDeploymentForTest(ctx, namespace, engineInstance, managedEnv, k8sClient, dbQueries)

			By("creating a GitOpsDeploymentSyncRun connected to the GitOpsDeployment, in DB and K8s")
			gitopsDeplSyncRun := createGitopsDeploymentSyncRun(ctx, app, gitopsDepl, namespace, k8sClient, dbQueries)

			By("starting the preprocess event loop")
			fixture := startPreprocessEventLoopRouter()

			By("sending an event for a new gitopsdeploymentsyncrun resource")
			event := buildGitOpsDeplSyncRunEventLoopTestEvent(gitopsDeplSyncRun, k8sClient, *namespace)

			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(1, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))
			fixture.responsesReceived = []eventlooptypes.EventLoopEvent{}

			By("sending the event again, now that the values should be cached in the preprocess event loop")
			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event
			expectExactlyANumberOfEvents(1, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))
			fixture.responsesReceived = []eventlooptypes.EventLoopEvent{}

			By("erasing the database, and sending the event again, to ensure it is coming from the cache")
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())
			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event
			expectExactlyANumberOfEvents(1, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))

		})

		It("Should handle the case where gitopsdeployment is created, then the k8s resource is deleted, then a new one k8s is created with the same name", func() {

			By("creating an Application row that will be connected to our GitOpsDeployment, in DB and K8s")

			app, gitopsDepl, _ := createApplicationConnectedToGitOpsDeploymentForTest(ctx, namespace, engineInstance, managedEnv, k8sClient, dbQueries)

			By("creating a GitOpsDeploymentSyncRun connected to the GitOpsDeployment, in DB and K8s")
			gitopsDeplSyncRun := createGitopsDeploymentSyncRun(ctx, app, gitopsDepl, namespace, k8sClient, dbQueries)

			By("starting the preprocess event loop")

			fixture := startPreprocessEventLoopRouter()

			By("sending an event for a new gitopsdeploymentsyncrun resource")

			event := buildGitOpsDeplSyncRunEventLoopTestEvent(gitopsDeplSyncRun, k8sClient, *namespace)

			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(1, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))
			fixture.responsesReceived = []eventlooptypes.EventLoopEvent{}

			err := k8sClient.Delete(ctx, &gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			secondGitopsDeplSyncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gitopsDeplSyncRun.Name,
					Namespace: namespace.Name,
					UID:       "test-a-different-uid-from-first-one",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: gitopsDepl.Name,
					RevisionID:           "",
				},
			}
			err = k8sClient.Create(ctx, secondGitopsDeplSyncRun)
			Expect(err).To(BeNil())

			By("sending an event for a second gitopsdeploymentsyncrun resource")

			event = buildGitOpsDeplSyncRunEventLoopTestEvent(*secondGitopsDeplSyncRun, k8sClient, *namespace)
			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(2, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))
			Expect(fixture.responsesReceived[1].AssociatedGitopsDeplUID).To(Equal(eventloop.OrphanedResourceGitopsDeplUID))

		})

		It("Should handle the case where gitopsdeployment is created, then the k8s resource is deleted, then a new one k8s is created with the same name, and the new one has already been processed and has a syncoperation associated with it", func() {

			By("creating an Application row that will be connected to our GitOpsDeployment, in DB and K8s")

			app, gitopsDepl, _ := createApplicationConnectedToGitOpsDeploymentForTest(ctx, namespace, engineInstance, managedEnv, k8sClient, dbQueries)

			gitopsDeplSyncRun := createGitopsDeploymentSyncRun(ctx, app, gitopsDepl, namespace, k8sClient, dbQueries)

			By("starting the preprocess event loop")

			fixture := startPreprocessEventLoopRouter()

			By("sending an event for a new gitopsdeploymentsyncrun resource")

			event := buildGitOpsDeplSyncRunEventLoopTestEvent(gitopsDeplSyncRun, k8sClient, *namespace)

			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(1, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))
			fixture.responsesReceived = []eventlooptypes.EventLoopEvent{}

			err := k8sClient.Delete(ctx, &gitopsDeplSyncRun)
			Expect(err).To(BeNil())

			// Create a new GitOpsDeployment
			app2, gitopsDepl2, _ := createCustomizedApplicationConnectedToGitOpsDeploymentForTest(ctx, "gitops-depl-two", namespace, engineInstance,
				managedEnv, k8sClient, dbQueries)

			// Create a new GitOpsDeploymentSyncRun, with the same name as the original, but with a different uid, and targetting a different GitOpsDeployment.
			secondGitopsDeplSyncRun := createCustomizedGitopsDeploymentSyncRun(ctx, gitopsDeplSyncRun.Name, "test-different-uid", "test-diff-sync-op",
				app2, gitopsDepl2, namespace, k8sClient, dbQueries)

			By("sending an event for a second gitopsdeploymentsyncrun resource")

			event = buildGitOpsDeplSyncRunEventLoopTestEvent(secondGitopsDeplSyncRun, k8sClient, *namespace)
			GinkgoWriter.Println("Sending event", event)
			fixture.preprocessInputChannel <- event

			expectExactlyANumberOfEvents(2, &fixture.responsesReceived)
			Expect(fixture.responsesReceived[0].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl.GetUID())))
			Expect(fixture.responsesReceived[1].AssociatedGitopsDeplUID).To(Equal(string(gitopsDepl2.GetUID())))

		})

	})

})

// startPreprocessEventLoopRouter starts an instance of the preprocessEventLoop, along with a mock for receiving
// the output of the preprocessEventLoop (a fake controller event loop)
func startPreprocessEventLoopRouter() *preprocessEventLoopRouterTestFixture {

	channel := make(chan eventlooptypes.EventLoopEvent)

	fakeControllerEventLoopChannel := make(chan eventlooptypes.EventLoopEvent)
	fakeControllerEventLoop := eventloop.ControllerEventLoop{
		EventLoopInputChannel: fakeControllerEventLoopChannel,
	}

	go preprocessEventLoopRouter(channel, &fakeControllerEventLoop)

	res := preprocessEventLoopRouterTestFixture{
		preprocessInputChannel:         channel,
		responsesReceived:              []eventlooptypes.EventLoopEvent{},
		fakeControllerEventLoopChannel: fakeControllerEventLoopChannel,
	}

	go func() {
		for {
			response := <-fakeControllerEventLoop.EventLoopInputChannel
			GinkgoWriter.Println(response, "->", response.AssociatedGitopsDeplUID)
			res.responsesReceived = append(res.responsesReceived, response)
		}
	}()

	return &res

}

type preprocessEventLoopRouterTestFixture struct {
	// preprocessInputChannel can be used to send input message to the preprocess event loop
	preprocessInputChannel chan eventlooptypes.EventLoopEvent

	// responsesReceived contains the list of outputs of the preprocess event loop (messages passed to the fake controller event loop)
	responsesReceived []eventlooptypes.EventLoopEvent

	// fakeControllerEventLoopChannel is the backing channel for our fake controller event loop
	fakeControllerEventLoopChannel chan eventlooptypes.EventLoopEvent
}

func createGitopsDeploymentSyncRun(ctx context.Context, app db.Application, gitopsDepl managedgitopsv1alpha1.GitOpsDeployment, namespace *corev1.Namespace,
	k8sClient client.Client, dbQueries db.AllDatabaseQueries) managedgitopsv1alpha1.GitOpsDeploymentSyncRun {

	return createCustomizedGitopsDeploymentSyncRun(ctx, "test-my-sync-run-normal", "test-my-sync-run-normal-uid", "test-my-sync-operation", app, gitopsDepl, namespace, k8sClient, dbQueries)
}

func createCustomizedGitopsDeploymentSyncRun(ctx context.Context, name string, uid string, syncOperationID string, app db.Application, gitopsDepl managedgitopsv1alpha1.GitOpsDeployment, namespace *corev1.Namespace,
	k8sClient client.Client, dbQueries db.AllDatabaseQueries) managedgitopsv1alpha1.GitOpsDeploymentSyncRun {
	var err error

	gitopsDeplSyncRun := managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace.Name,
			UID:       types.UID(uid),
		},
		Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
			GitopsDeploymentName: gitopsDepl.Name,
			RevisionID:           "revision",
		},
	}
	err = k8sClient.Create(ctx, &gitopsDeplSyncRun)
	Expect(err).To(BeNil())

	syncOperation := db.SyncOperation{
		SyncOperation_id:    syncOperationID,
		Application_id:      app.Application_id,
		DeploymentNameField: gitopsDeplSyncRun.Spec.GitopsDeploymentName,
		Revision:            gitopsDeplSyncRun.Spec.RevisionID,
		DesiredState:        db.SyncOperation_DesiredState_Running,
	}
	err = dbQueries.CreateSyncOperation(ctx, &syncOperation)
	Expect(err).To(BeNil())

	mapping := db.APICRToDatabaseMapping{
		APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
		APIResourceUID:       string(gitopsDeplSyncRun.GetUID()),
		APIResourceName:      gitopsDeplSyncRun.Name,
		APIResourceNamespace: gitopsDeplSyncRun.Namespace,
		NamespaceUID:         string(namespace.GetUID()),
		DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
		DBRelationKey:        syncOperation.SyncOperation_id,
	}
	err = dbQueries.CreateAPICRToDatabaseMapping(ctx, &mapping)
	Expect(err).To(BeNil())

	return gitopsDeplSyncRun
}

func expectExactlyANumberOfEvents(expectedEvents int, responsesReceived *[]eventlooptypes.EventLoopEvent, optionalDescription ...string) {

	Expect(len(optionalDescription)).To(BeNumerically("<=", 1), "Only 0 or 1 description fields are supported")

	asyncAssertion := Eventually(func() bool {
		GinkgoWriter.Println("responses received length is", len(*responsesReceived))
		return len(*responsesReceived) == expectedEvents
	}, "500ms", "50ms")

	if len(optionalDescription) == 1 {
		asyncAssertion.Should(Equal(true), optionalDescription[0])
	} else {
		asyncAssertion.Should(Equal(true))
	}

	asyncAssertion = Consistently(func() bool {
		GinkgoWriter.Println("responses received length continues to be", len(*responsesReceived))
		return len(*responsesReceived) == expectedEvents
	}, "500ms", "50ms")

	if len(optionalDescription) == 1 {
		asyncAssertion.Should(Equal(true), optionalDescription[0])
	} else {
		asyncAssertion.Should(Equal(true))
	}

}

// createApplicationConnectedToGitOpsDeploymentForTest creates a Application row, GitOpsDeployment CR, and a DeploymentToApplicationMapping which connects them.
// This function calls the function below with a default value.
func createApplicationConnectedToGitOpsDeploymentForTest(ctx context.Context, namespace *corev1.Namespace, engineInstance *db.GitopsEngineInstance,
	managedEnv *db.ManagedEnvironment, k8sClient client.Client, dbQueries db.AllDatabaseQueries) (db.Application,
	managedgitopsv1alpha1.GitOpsDeployment, db.DeploymentToApplicationMapping) {

	return createCustomizedApplicationConnectedToGitOpsDeploymentForTest(ctx, "my-gitops-depl", namespace, engineInstance, managedEnv, k8sClient, dbQueries)

}

// createCustomizedApplicationConnectedToGitOpsDeploymentForTest creates a Application row, GitOpsDeployment CR, and a DeploymentToApplicationMapping which connects them.
func createCustomizedApplicationConnectedToGitOpsDeploymentForTest(ctx context.Context, name string, namespace *corev1.Namespace, engineInstance *db.GitopsEngineInstance,
	managedEnv *db.ManagedEnvironment, k8sClient client.Client, dbQueries db.AllDatabaseQueries) (db.Application,
	managedgitopsv1alpha1.GitOpsDeployment, db.DeploymentToApplicationMapping) {

	var err error

	app := &db.Application{
		Application_id:          "test-" + name,
		Name:                    "app-" + name,
		Spec_field:              "{}",
		Engine_instance_inst_id: engineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnv.Managedenvironment_id,
	}
	err = dbQueries.CreateApplication(ctx, app)
	Expect(err).To(BeNil())

	By("creating a GitOpsDeployment in the namespace")

	gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace.Name,
			UID:       types.UID("test-" + name + "-uid"),
		},
	}
	err = k8sClient.Create(ctx, gitopsDepl)
	Expect(err).To(BeNil())

	err = k8sClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl)
	Expect(err).To(BeNil())

	By("ensuring the database Application references the GitOpsDeployment")

	deplToAppMapping := &db.DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: string(gitopsDepl.UID),
		DeploymentName:                        gitopsDepl.Name,
		DeploymentNamespace:                   namespace.Name,
		NamespaceUID:                          string(namespace.UID),
		Application_id:                        app.Application_id,
	}

	err = dbQueries.CreateDeploymentToApplicationMapping(ctx, deplToAppMapping)
	Expect(err).To(BeNil())

	return *app, *gitopsDepl, *deplToAppMapping

}

func buildGitOpsDeplSyncRunEventLoopTestEvent(gitopsDeplSyncRun managedgitopsv1alpha1.GitOpsDeploymentSyncRun,
	k8sClient client.Client, workspace corev1.Namespace) eventlooptypes.EventLoopEvent {

	res := eventlooptypes.EventLoopEvent{
		EventType: eventlooptypes.SyncRunModified,
		Request: reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: gitopsDeplSyncRun.Namespace,
				Name:      gitopsDeplSyncRun.Name,
			},
		},
		ReqResource: managedgitopsv1alpha1.GitOpsDeploymentSyncRunTypeName,
		Client:      k8sClient,
		WorkspaceID: eventlooptypes.GetWorkspaceIDFromNamespaceID(workspace),
	}

	return res
}

func buildGitOpsDeplEventLoopTestEvent(gitopsDepl managedgitopsv1alpha1.GitOpsDeployment, k8sClient client.Client, workspace corev1.Namespace) eventlooptypes.EventLoopEvent {

	res := eventlooptypes.EventLoopEvent{
		EventType: eventlooptypes.DeploymentModified,
		Request: reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: gitopsDepl.Namespace,
				Name:      gitopsDepl.Name,
			},
		},
		ReqResource: managedgitopsv1alpha1.GitOpsDeploymentTypeName,
		Client:      k8sClient,
		WorkspaceID: eventlooptypes.GetWorkspaceIDFromNamespaceID(workspace),
	}

	return res
}
