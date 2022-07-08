package eventloop

import (
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// This test scenario requires, tests to run in order, hence using 'Ordered' decorator.
var _ = Describe("Workspace Event Loop Test", Ordered, func() {

	Context("Workspace event loop responds to channel events", func() {

		var err error
		var scheme *runtime.Scheme
		var apiNamespace *v1.Namespace
		var argocdNamespace *v1.Namespace
		var kubesystemNamespace *v1.Namespace
		var tAELF *testApplicationEventLoopFactory
		var msgTemp eventlooptypes.EventLoopMessage
		var workspaceEventLoopRouter WorkspaceEventLoopRouterStruct

		// Start the workspace event loop using single ApplicationEventLoopFactory object,
		// this way all tests can keep track of number of event loops created by other tests.
		BeforeAll(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err = eventlooptypes.GenericTestSetup()

			Expect(err).To(BeNil())

			tAELF = &testApplicationEventLoopFactory{
				outputChannel: make(chan eventlooptypes.EventLoopMessage),
			}

			// Start the workspace event loop with our custom test factory, so that we can capture output
			workspaceEventLoopRouter = newWorkspaceEventLoopRouterWithFactory(string(apiNamespace.UID), tAELF)
		})

		It("Should pass events received on input channel to application event loop", func() {

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: apiNamespace.Name,
					UID:       "A",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			// Simulate a GitOpsDeployment modified event
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Name,
							Name:      apiNamespace.Name,
						},
					},
					Client:                  k8sClient,
					ReqResource:             managedgitopsv1alpha1.GitOpsDeploymentTypeName,
					AssociatedGitopsDeplUID: string(gitopsDepl.UID),
					WorkspaceID:             string(apiNamespace.UID),
				},
			}

			// Sending an event to the workspace event loop should cause an event to be sent on the output channel
			workspaceEventLoopRouter.SendMessage(msg)
			result := <-tAELF.outputChannel

			Expect(result).NotTo(BeNil())
			Expect(result.Event).To(Equal(msg.Event))
			Expect(result.MessageType).To(Equal(msg.MessageType))
			Expect(tAELF.numberOfEventLoopsCreated).To(Equal(1))
		})

		It("Should not create new application event loop when 2nd event is passed with same ID", func() {
			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: apiNamespace.Name,
					UID:       "A",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			// Simulate a GitOpsDeployment modified event
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Name,
							Name:      apiNamespace.Name,
						},
					},
					Client:                  k8sClient,
					ReqResource:             managedgitopsv1alpha1.GitOpsDeploymentTypeName,
					AssociatedGitopsDeplUID: string(gitopsDepl.UID),
					WorkspaceID:             string(apiNamespace.UID),
				},
			}

			// Sending an event to the workspace event loop should cause an event to be sent on the output channel
			workspaceEventLoopRouter.SendMessage(msg)
			result := <-tAELF.outputChannel

			Expect(result).NotTo(BeNil())
			Expect(result.Event).To(Equal(msg.Event))
			Expect(result.MessageType).To(Equal(msg.MessageType))
			Expect(tAELF.numberOfEventLoopsCreated).To(Equal(1))

		})

		It("Should create new application event loop when 3rd event is passed with diferent ID", func() {
			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: apiNamespace.Name,
					UID:       "B",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			// Simulate a GitOpsDeployment modified event
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Name,
							Name:      apiNamespace.Name,
						},
					},
					Client:                  k8sClient,
					ReqResource:             managedgitopsv1alpha1.GitOpsDeploymentTypeName,
					AssociatedGitopsDeplUID: string(gitopsDepl.UID),
					WorkspaceID:             string(apiNamespace.UID),
				},
			}

			// Sending an event to the workspace event loop should cause an event to be sent on the output channel
			workspaceEventLoopRouter.SendMessage(msg)
			result := <-tAELF.outputChannel

			Expect(result).NotTo(BeNil())
			Expect(result.Event).To(Equal(msg.Event))
			Expect(result.MessageType).To(Equal(msg.MessageType))
			Expect(tAELF.numberOfEventLoopsCreated).To(Equal(2))

		})

		It("Should not pass an orphaned event to application event loop.", func() {

			gitopsDeplSyncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl-sync",
					Namespace: apiNamespace.Name,
					UID:       "C",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: "dummy-deployment",
					RevisionID:           "HEAD",
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDeplSyncRun, apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			// Simulate a GitOpsDeploymentSyncRun event
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.SyncRunModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Name,
							Name:      "my-gitops-depl-sync",
						},
					},
					Client:                  k8sClient,
					ReqResource:             managedgitopsv1alpha1.GitOpsDeploymentSyncRunTypeName,
					AssociatedGitopsDeplUID: "orphaned",
					WorkspaceID:             string(apiNamespace.UID),
				},
			}

			// To share it with the next test
			msgTemp = msg

			// The number of event loops created before we started the test
			originalNumberOfEventLoopsCreated := tAELF.numberOfEventLoopsCreated

			// Create goroutine and pass event,
			//because loop in workspaceEventLoopRouter will keep on waiting for new event to be received and test will get stuck here.
			go func() {

				workspaceEventLoopRouter.SendMessage(msg)
				// We don't read from the channel here, because this will cause us to
				// miss the event when we tried to read from it elsewhere.
			}()

			// Consider test case passed if a new application event loop is not created in 5 seconds.
			Consistently(tAELF.numberOfEventLoopsCreated, "5s").Should(Equal(originalNumberOfEventLoopsCreated),
				"the number of event loops shoulnd't change")
		})

		It("Should unorphan previous GitOpsDeploymentSyncRun event if parent GitOpsDeployment event is passed and new application event loop should be created.", func() {

			By("creating a new GitOpsDeployment resource, that the GitOpsDeploymentSyncRun was missing from the previous step.")

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-deployment",
					Namespace: apiNamespace.Name,
					UID:       "D",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			// Simulate a GitOpsDeployment modified event
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Namespace,
							Name:      gitopsDepl.Name,
						},
					},
					Client:                  k8sClient,
					ReqResource:             managedgitopsv1alpha1.GitOpsDeploymentTypeName,
					AssociatedGitopsDeplUID: string(gitopsDepl.UID),
					WorkspaceID:             string(apiNamespace.UID),
				},
			}

			// Create goroutine and pass event,
			//because loop in workspaceEventLoopRouter will keep on waiting for new event to be received and test will get stuck here.
			go func() {
				workspaceEventLoopRouter.SendMessage(msg)
			}()

			Eventually(func() bool {
				// We use a function here to check if number is 3
				return tAELF.numberOfEventLoopsCreated == 3
			}).Should(BeTrue())

			By("Read the events from the output channel, make sure they are the ones we expect, now that the gitopsdeploymentsyncrun in unorphaned")

			deploymentModifiedMsg := <-tAELF.outputChannel

			// Make sure this is the gitopsdeployment event from above
			Expect(deploymentModifiedMsg).NotTo(BeNil())
			Expect(deploymentModifiedMsg.Event).To(Equal(msg.Event))
			Expect(deploymentModifiedMsg.MessageType).To(Equal(msg.MessageType))

			syncRunModifiedMsg := <-tAELF.outputChannel

			// Make sure this is the gitopsdeploymentsyncrun event from above
			Expect(syncRunModifiedMsg).NotTo(BeNil())
			Expect(syncRunModifiedMsg.Event.Request.Name).To(Equal(msgTemp.Event.Request.Name))
			Expect(syncRunModifiedMsg.MessageType).To(Equal(msgTemp.MessageType))
		})

	})

	Context("Verify that managedEnvProcessed_Event event is handled", func() {

		var err error
		var scheme *runtime.Scheme
		var apiNamespace *v1.Namespace
		var argocdNamespace *v1.Namespace
		var kubesystemNamespace *v1.Namespace

		// Start the workspace event loop using single ApplicationEventLoopFactory object,
		// this way all tests can keep track of number of event loops created by other tests.
		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err = eventlooptypes.GenericTestSetup()

			Expect(err).To(BeNil())
		})

		simulateGitOpsDeployments := func(numberToSimulate int, tAELF *managedEnvironmentTestApplicationEventLoopFactory,
			workspaceEventLoopRouter *WorkspaceEventLoopRouterStruct, k8sClient client.Client) []managedgitopsv1alpha1.GitOpsDeployment {

			res := []managedgitopsv1alpha1.GitOpsDeployment{}

			for x := 0; x < numberToSimulate; x++ {

				gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("my-gitops-depl-%d", x),
						Namespace: apiNamespace.Name,
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
						Source: managedgitopsv1alpha1.ApplicationSource{},
						Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
					},
				}

				tAELF.outputChannel[string(gitopsDepl.UID)] = make(chan eventlooptypes.EventLoopMessage)

				res = append(res, *gitopsDepl)

				// Simulate a GitOpsDeployment modified event
				msg := eventlooptypes.EventLoopMessage{
					MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event: &eventlooptypes.EventLoopEvent{
						EventType: eventlooptypes.DeploymentModified,
						Request: reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: apiNamespace.Name,
								Name:      apiNamespace.Name,
							},
						},
						Client:                  k8sClient,
						ReqResource:             managedgitopsv1alpha1.GitOpsDeploymentTypeName,
						AssociatedGitopsDeplUID: string(gitopsDepl.UID),
						WorkspaceID:             string(apiNamespace.UID),
					},
				}

				By("Sending an event to the workspace event loop should cause an event to be sent on the output channel")
				workspaceEventLoopRouter.SendMessage(msg)

				By("waiting for the respond to be received")
				result := <-tAELF.outputChannel[string(gitopsDepl.UID)]
				Expect(result).NotTo(BeNil())
				Expect(result.Event).To(Equal(msg.Event))
				Expect(result.MessageType).To(Equal(msg.MessageType))

			}

			return res
		}

		It("should only forward a managedenv event if there exists at least 1 active GitOpsDeployment", func() {

			By("Starting the workspace event loop with our custom test factory, so that we can capture output")
			tAELF := &managedEnvironmentTestApplicationEventLoopFactory{
				outputChannel: map[string]chan eventlooptypes.EventLoopMessage{},
			}
			workspaceEventLoopRouter := newWorkspaceEventLoopRouterWithFactory(string(apiNamespace.UID), tAELF)

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			By("sending a 'managedEnvProcessed_Event' event")
			internalEvent := &eventlooptypes.EventLoopEvent{
				EventType: eventlooptypes.ManagedEnvironmentModified,
				Request:   reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "managed-env-namespace", Name: "managed-env"}},
				Client:    k8sClient,
			}
			workspaceEventLoopRouter.channel <- workspaceEventLoopMessage{
				messageType: managedEnvProcessed_Event,
				payload: eventlooptypes.EventLoopMessage{
					MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event:       internalEvent,
				},
			}

			Consistently(func() bool {
				return tAELF.numberOfEventLoopsCreated == 0
			}, "1s", "10ms").Should(Equal(true), "an event loop should never be created, because the managedenv was never forwarded")

		})

		DescribeTable("Create 1, 2, and 3 GitOpsDeployments, and simulate a ManagedEnvironment event. The event should always be "+
			"forwarded to all GitOpsDeployments runners.", func(numGitOpsDeploymentsToSimulate int) {

			By("Starting the workspace event loop with our custom test factory, so that we can capture output")
			tAELF := &managedEnvironmentTestApplicationEventLoopFactory{
				outputChannel: map[string]chan eventlooptypes.EventLoopMessage{},
			}
			workspaceEventLoopRouter := newWorkspaceEventLoopRouterWithFactory(string(apiNamespace.UID), tAELF)

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects( /*gitopsDepl,*/ apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			By("creating simulating GitOpsDeployment events, causing the workspace event router to start an application event loop for them")
			gitopsDeployments := simulateGitOpsDeployments(numGitOpsDeploymentsToSimulate, tAELF, &workspaceEventLoopRouter, k8sClient)

			Expect(tAELF.numberOfEventLoopsCreated).To(Equal(numGitOpsDeploymentsToSimulate),
				"an event loop should be created for each separate GitOpsDeployment")

			By("sending a 'managedEnvProcessed_Event' event to the workspace event loop")
			internalEvent := &eventlooptypes.EventLoopEvent{
				EventType: eventlooptypes.ManagedEnvironmentModified,
				Request:   reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "managed-env-namespace", Name: "managed-env"}},
				Client:    k8sClient,
			}
			workspaceEventLoopRouter.channel <- workspaceEventLoopMessage{
				messageType: managedEnvProcessed_Event,
				payload: eventlooptypes.EventLoopMessage{
					MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event:       internalEvent,
				},
			}

			By("ensuring that each mock GitOpsDeployment runner was forwarded the managed environment event")

			messagesReceived := map[string][]eventlooptypes.EventLoopMessage{}

			for idx := range gitopsDeployments {
				var mutex sync.Mutex

				gitopsDeployment := gitopsDeployments[idx]

				eventLoopMsgsReceived := []eventlooptypes.EventLoopMessage{}

				By("starting a goroutine which writes all received events to eventLoopMsgsReceived for " + string(gitopsDeployment.UID))
				go func() {
					for {
						fromOutputChan := <-tAELF.outputChannel[string(gitopsDeployment.UID)]
						GinkgoWriter.Println("event received on gofunc", string(gitopsDeployment.UID), fromOutputChan)
						mutex.Lock()
						eventLoopMsgsReceived = append(eventLoopMsgsReceived, fromOutputChan)
						mutex.Unlock()
					}
				}()

				By("waiting for exactly 1 message to be received by the mock gitopsdeployment runner")
				Eventually(func() bool {
					mutex.Lock()
					defer mutex.Unlock()
					return len(eventLoopMsgsReceived) >= 1
				}, "3s", "50ms").Should(BeTrue())

				Consistently(func() bool {
					mutex.Lock()
					defer mutex.Unlock()
					return len(eventLoopMsgsReceived) == 1
				}, "500ms", "50ms").Should(BeTrue())

				By("adding all received messages to messagesReceived, for final expect checks")
				messagesReceived[string(gitopsDeployment.UID)] = []eventlooptypes.EventLoopMessage{}
				mutex.Lock()
				defer mutex.Unlock()
				for idx := range eventLoopMsgsReceived {
					messagesReceived[string(gitopsDeployment.UID)] = append(messagesReceived[string(gitopsDeployment.UID)],
						eventLoopMsgsReceived[idx])
				}
			}

			for _, existingGitOpsDeployment := range gitopsDeployments {

				received := messagesReceived[string(existingGitOpsDeployment.UID)]
				Expect(len(received)).To(Equal(1), "all the gitopsdeployment should receive the managed env event")

				msg := received[0]
				Expect(msg.Event.EventType == eventlooptypes.ManagedEnvironmentModified)
			}

		},
			Entry("Simulate 1 existing, active GitOpsDeployment", 1),
			Entry("Simulate 2 existing, active GitOpsDeployment", 2),
			Entry("Simulate 3 existing, active GitOpsDeployment", 3),
		)

	})
})

// testApplicationEventLoopFactory is a mock applicationEventQueueLoopFactory, used for unit tests above.
type testApplicationEventLoopFactory struct {
	mutex                     sync.Mutex
	outputChannel             chan eventlooptypes.EventLoopMessage
	numberOfEventLoopsCreated int
}

var _ applicationEventQueueLoopFactory = &testApplicationEventLoopFactory{}

// Instead of starting a new application event queue loop (like the default implementation of applictionEventQueueLoopFactory)
// we instead just return a previously provided channel.
func (ta *testApplicationEventLoopFactory) startApplicationEventQueueLoop(gitopsDeplID string, workspaceID string,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop) chan eventlooptypes.EventLoopMessage {

	ta.mutex.Lock()
	defer ta.mutex.Unlock()

	// Increase count by 1 if new application event loop is created
	ta.numberOfEventLoopsCreated++
	return ta.outputChannel
}

// testApplicationEventLoopFactory is a mock applicationEventQueueLoopFactory, used for unit tests above.
type managedEnvironmentTestApplicationEventLoopFactory struct {
	outputChannel             map[string]chan eventlooptypes.EventLoopMessage
	numberOfEventLoopsCreated int
	mutex                     sync.Mutex
}

var _ applicationEventQueueLoopFactory = &managedEnvironmentTestApplicationEventLoopFactory{}

// Instead of starting a new application event queue loop (like the default implementation of applictionEventQueueLoopFactory)
// we instead just return a previously provided channel.
func (ta *managedEnvironmentTestApplicationEventLoopFactory) startApplicationEventQueueLoop(gitopsDeplID string, workspaceID string,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop) chan eventlooptypes.EventLoopMessage {

	ta.mutex.Lock()
	defer ta.mutex.Unlock()

	// Increase count by 1 if new application event loop is created
	ta.numberOfEventLoopsCreated++
	return ta.outputChannel[gitopsDeplID]
}
