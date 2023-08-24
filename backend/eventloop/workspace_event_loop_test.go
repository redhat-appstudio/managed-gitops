package eventloop

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/application_event_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// This test scenario requires tests to run in order, hence using 'Ordered' decorator.
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

		var k8sClient client.WithWatch

		// Start the workspace event loop using single ApplicationEventLoopFactory object,
		// this way all tests can keep track of number of event loops created by other tests.
		BeforeAll(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err = tests.GenericTestSetup()

			Expect(err).ToNot(HaveOccurred())

			tAELF = &testApplicationEventLoopFactory{}

			// Start the workspace event loop with our custom test factory, so that we can capture output
			workspaceEventLoopRouter = newWorkspaceEventLoopRouterWithFactory(string(apiNamespace.UID), tAELF)

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

		})

		It("should pass events received on input channel to application event loop", func() {

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

			err := k8sClient.Create(context.Background(), gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			// Simulate a GitOpsDeployment modified event
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Name,
							Name:      gitopsDepl.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
					WorkspaceID: string(apiNamespace.UID),
				},
			}

			// Sending an event to the workspace event loop should cause an event to be sent on the output channel
			workspaceEventLoopRouter.SendMessage(msg)
			tAELF.waitForFirstInvocation()
			result := <-tAELF.outputChannel

			Expect(result).NotTo(BeNil())

			Expect(result.ResponseChan).ToNot(BeNil())
			result.ResponseChan <- application_event_loop.ResponseMessage{
				RequestAccepted: true,
			}

			Expect(result.Message).NotTo(BeNil())
			Expect(result.Message.Event).To(Equal(msg.Event))
			Expect(result.Message.MessageType).To(Equal(msg.MessageType))
			Expect(tAELF.numberOfEventLoopsCreated).To(Equal(1))
		})

		It("should not create new application event loop when 2nd event is passed with same name/ID", func() {

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

			By("simulating a GitOpsDeployment modified event")
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: gitopsDepl.Namespace,
							Name:      gitopsDepl.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
					WorkspaceID: string(apiNamespace.UID),
				},
			}

			By("sending an event to the workspace event loop should cause an event to be sent on the output channel")
			workspaceEventLoopRouter.SendMessage(msg)
			tAELF.waitForFirstInvocation()
			result := <-tAELF.outputChannel

			Expect(result).NotTo(BeNil())

			Expect(result.ResponseChan).ToNot(BeNil())
			result.ResponseChan <- application_event_loop.ResponseMessage{
				RequestAccepted: true,
			}

			Expect(result.Message).NotTo(BeNil())
			Expect(result.Message.Event).To(Equal(msg.Event))
			Expect(result.Message.MessageType).To(Equal(msg.MessageType))
			Expect(tAELF.numberOfEventLoopsCreated).To(Equal(1))

		})

		It("should not create new application event loop when 3rd event is passed with same name but different UID", func() {
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

			By("deleting the old GitOpsDeployment from the previous test")
			err := k8sClient.Delete(context.Background(), gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			By("creating a new GitOpsDeployment with the same name as the previous one, but a different UID")
			err = k8sClient.Create(context.Background(), gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			By("simulating a GitOpsDeployment modified event")
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Name,
							Name:      gitopsDepl.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
					WorkspaceID: string(apiNamespace.UID),
				},
			}

			By("sending an event to the workspace event loop should cause an event to be sent on the output channel")
			workspaceEventLoopRouter.SendMessage(msg)
			tAELF.waitForFirstInvocation()
			result := <-tAELF.outputChannel

			Expect(result).NotTo(BeNil())

			Expect(result.ResponseChan).ToNot(BeNil())
			result.ResponseChan <- application_event_loop.ResponseMessage{
				RequestAccepted: true,
			}
			Expect(result.Message).NotTo(BeNil())
			Expect(result.Message.Event).To(Equal(msg.Event))
			Expect(result.Message.MessageType).To(Equal(msg.MessageType))
			Expect(tAELF.numberOfEventLoopsCreated).To(Equal(1))

		})

		It("should not pass an orphaned event to application event loop.", func() {

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

			By("creating a new orphaned GitOpsDeploymentSync")
			err = k8sClient.Create(context.Background(), gitopsDeplSyncRun)
			Expect(err).ToNot(HaveOccurred())

			By("simulating a GitOpsDeploymentSyncRun event")
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
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentSyncRunTypeName,
					WorkspaceID: string(apiNamespace.UID),
				},
			}

			// To share it with the next test
			msgTemp = msg

			// The number of event loops created before we started the test
			originalNumberOfEventLoopsCreated := tAELF.numberOfEventLoopsCreated

			// Create goroutine and pass event,
			// because loop in workspaceEventLoopRouter will keep on waiting for new event to be received and test will get stuck here.
			go func() {

				workspaceEventLoopRouter.SendMessage(msg)
				// We don't read from the channel here, because this will cause us to
				// miss the event when we tried to read from it elsewhere.
			}()

			// Consider test case passed if a new application event loop is not created in 5 seconds.
			Consistently(tAELF.numberOfEventLoopsCreated, "5s").Should(Equal(originalNumberOfEventLoopsCreated),
				"the number of event loops should't change")
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

			By("creating a new GitOpsDeployment, to unorphan the GitOpsDeploymentSync")
			err = k8sClient.Create(context.Background(), gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			By("simulating a GitOpsDeployment modified event")
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Name,
							Name:      gitopsDepl.Name,
						},
					},
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
					WorkspaceID: string(apiNamespace.UID),
				},
			}

			// Create goroutine and pass event, because loop in workspaceEventLoopRouter will keep on waiting for new event
			// to be received and test will get stuck here.
			go func() {
				workspaceEventLoopRouter.SendMessage(msg)
			}()

			Eventually(func() bool {
				tAELF.mutex.Lock()
				defer tAELF.mutex.Unlock()

				// We use a function here to check if number is 2
				return tAELF.numberOfEventLoopsCreated == 2
			}, time.Second*240).Should(BeTrue())

			By("reading the events from the output channel, make sure they are the ones we expect, now that the gitopsdeploymentsyncrun in unorphaned")

			tAELF.waitForFirstInvocation()
			deploymentModifiedMsg := <-tAELF.outputChannel

			// Make sure this is the gitopsdeployment event from above
			Expect(deploymentModifiedMsg).NotTo(BeNil())
			Expect(deploymentModifiedMsg.ResponseChan).NotTo(BeNil())
			deploymentModifiedMsg.ResponseChan <- application_event_loop.ResponseMessage{
				RequestAccepted: true,
			}
			Expect(deploymentModifiedMsg.Message).NotTo(BeNil())
			Expect(deploymentModifiedMsg.Message.Event).To(Equal(msg.Event))
			Expect(deploymentModifiedMsg.Message.MessageType).To(Equal(msg.MessageType))

			syncRunModifiedMsg := <-tAELF.outputChannel

			// Make sure this is the gitopsdeploymentsyncrun event from above
			Expect(syncRunModifiedMsg).NotTo(BeNil())
			Expect(syncRunModifiedMsg.ResponseChan).NotTo(BeNil())
			syncRunModifiedMsg.ResponseChan <- application_event_loop.ResponseMessage{
				RequestAccepted: true,
			}
			Expect(syncRunModifiedMsg.Message).NotTo(BeNil())
			Expect(syncRunModifiedMsg.Message.Event.Request.Name).To(Equal(msgTemp.Event.Request.Name))
			Expect(syncRunModifiedMsg.Message.MessageType).To(Equal(msgTemp.MessageType))
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
				err = tests.GenericTestSetup()

			Expect(err).ToNot(HaveOccurred())
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

				res = append(res, *gitopsDepl)

				// Simulate a GitOpsDeployment modified event
				msg := eventlooptypes.EventLoopMessage{
					MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event: &eventlooptypes.EventLoopEvent{
						EventType: eventlooptypes.DeploymentModified,
						Request: reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: apiNamespace.Name,
								Name:      gitopsDepl.Name,
							},
						},
						Client:      k8sClient,
						ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
						WorkspaceID: string(apiNamespace.UID),
					},
				}

				By("sending an event to the workspace event loop should cause an event to be seen on the output channel")
				workspaceEventLoopRouter.SendMessage(msg)

				By("waiting for the responce to be received")
				tAELF.waitForFirstInvocation(gitopsDepl.Name)
				result := <-tAELF.outputChannelMap[gitopsDepl.Name]
				Expect(result).NotTo(BeNil())
				Expect(result.ResponseChan).NotTo(BeNil())
				result.ResponseChan <- application_event_loop.ResponseMessage{
					RequestAccepted: true,
				}

				Expect(result.Message).NotTo(BeNil())
				Expect(result.Message.Event).To(Equal(msg.Event))
				Expect(result.Message.MessageType).To(Equal(msg.MessageType))

			}

			return res
		}

		It("should only forward a managedenv event if there exists at least 1 active GitOpsDeployment", func() {

			By("Starting the workspace event loop with our custom test factory, so that we can capture output")
			tAELF := &managedEnvironmentTestApplicationEventLoopFactory{
				outputChannelMap: map[string]chan application_event_loop.RequestMessage{},
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
				messageType: workspaceEventLoopMessageType_managedEnvProcessed_Event,
				payload: eventlooptypes.EventLoopMessage{
					MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event:       internalEvent,
				},
			}

			Consistently(func() bool {
				return tAELF.numberOfEventLoopsCreated == 0
			}, "1s", "10ms").Should(BeTrue(), "an event loop should never be created, because the managedenv was never forwarded")

		})

		DescribeTable("Create 1, 2, and 3 GitOpsDeployments, and simulate a ManagedEnvironment event. The event should always be "+
			"forwarded to all GitOpsDeployments runners.", func(numGitOpsDeploymentsToSimulate int) {

			By("Starting the workspace event loop with our custom test factory, so that we can capture output")
			tAELF := &managedEnvironmentTestApplicationEventLoopFactory{
				outputChannelMap: map[string]chan application_event_loop.RequestMessage{},
			}
			workspaceEventLoopRouter := newWorkspaceEventLoopRouterWithFactory(string(apiNamespace.UID), tAELF)

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects( /*gitopsDepl,*/ apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			By("creating simulating GitOpsDeployment events, causing the workspace event router to start an application event loop " +
				"for them")
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
				messageType: workspaceEventLoopMessageType_managedEnvProcessed_Event,
				payload: eventlooptypes.EventLoopMessage{
					MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event:       internalEvent,
				},
			}

			By("ensuring that each mock GitOpsDeployment runner was forwarded the managed environment event")

			messagesReceived := map[string][]application_event_loop.RequestMessage{}

			for idx := range gitopsDeployments {
				var mutex sync.Mutex

				gitopsDeployment := gitopsDeployments[idx]

				eventLoopMsgsReceived := []application_event_loop.RequestMessage{}

				By("starting a goroutine which writes all received events to eventLoopMsgsReceived for " + string(gitopsDeployment.UID))
				go func() {
					for {
						tAELF.waitForFirstInvocation(gitopsDeployment.Name)

						fromOutputChan := <-tAELF.outputChannelMap[gitopsDeployment.Name]
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
				messagesReceived[string(gitopsDeployment.UID)] = []application_event_loop.RequestMessage{}
				mutex.Lock()
				defer mutex.Unlock()

				messagesReceived[string(gitopsDeployment.UID)] = append(messagesReceived[string(gitopsDeployment.UID)], eventLoopMsgsReceived...)

			}

			for _, existingGitOpsDeployment := range gitopsDeployments {

				received := messagesReceived[string(existingGitOpsDeployment.UID)]
				Expect(received).To(HaveLen(1), "all the gitopsdeployment should receive the managed env event")

				msg := received[0]
				Expect(msg.Message.Event.EventType).To(Equal(eventlooptypes.ManagedEnvironmentModified))
			}

		},
			Entry("Simulate 1 existing, active GitOpsDeployment", 1),
			Entry("Simulate 2 existing, active GitOpsDeployment", 2),
			Entry("Simulate 3 existing, active GitOpsDeployment", 3),
		)

	})

	Context("processWorkspaceEventLoopMessage tests", func() {
		ctx := context.Background()

		It("repository credentials events should be forwarded", func() {

			By("creating a simple event referencing a RepositroyCredential resource")
			event := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					ReqResource: eventlooptypes.GitOpsDeploymentRepositoryCredentialTypeName,
				},
			}

			wrapperEvent := workspaceEventLoopMessage{
				messageType: workspaceEventLoopMessageType_Event,
				payload:     event,
			}

			responseChannel := make(chan workspaceResourceLoopMessage, 5)

			state := workspaceEventLoopInternalState{
				log: log.FromContext(ctx),
				workspaceResourceLoop: &workspaceResourceEventLoop{
					inputChannel: responseChannel,
				},
			}

			By("calling the function to process the event")
			handleWorkspaceEventLoopMessage(ctx, event, wrapperEvent, state)

			forwardedEvent := <-responseChannel
			Expect(forwardedEvent.messageType).To(Equal(workspaceResourceLoopMessageType_processRepositoryCredential), "event should have been forwarded")

		})

		It("status ticker events should be forwarded", func() {
			By("create a goroutine that rejects the request")
			inactiveChan := make(chan application_event_loop.RequestMessage)
			go func() {
				req := <-inactiveChan
				req.ResponseChan <- application_event_loop.ResponseMessage{
					RequestAccepted: false,
				}

			}()

			By("create an event for the status ticker")
			event := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_StatusCheck,
				Event: &eventlooptypes.EventLoopEvent{
					ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
				},
			}
			wrapperEvent := workspaceEventLoopMessage{
				messageType: workspaceEventLoopMessageType_statusTicker,
				payload:     event,
			}

			state := workspaceEventLoopInternalState{
				applicationMap: map[string]workspaceEventLoop_applicationEventLoopEntry{
					"inactive": {input: inactiveChan},
				},
			}

			By("verify if the request is rejected")
			processWorkspaceEventLoopMessage(ctx, event, wrapperEvent, state, state.log)
			Eventually(func() bool {
				_, exists := state.applicationMap["inactive"]
				return exists
			}).Should(BeFalse(), "the inactive application should have rejected the request, and thus should have been removed from the map")
		})

		DescribeTable("shouldn't forward the event if it is completed/nil/unknown", func(event eventlooptypes.EventLoopMessage, msgType workspaceEventLoopMessageType) {
			wrapperEvent := workspaceEventLoopMessage{
				messageType: msgType,
				payload:     event,
			}

			responseChannel := make(chan workspaceResourceLoopMessage, 5)

			state := workspaceEventLoopInternalState{
				log: log.FromContext(ctx),
				workspaceResourceLoop: &workspaceResourceEventLoop{
					inputChannel: responseChannel,
				},
			}

			By("verify that the event is not forwarded")
			processWorkspaceEventLoopMessage(ctx, event, wrapperEvent, state, state.log)
			Consistently(func() bool {
				select {
				case <-responseChannel:
					return false
				default:
					return true
				}
			}, "3s", "50ms").Should(BeTrue())

		}, Entry("reject the event if it is nil", eventlooptypes.EventLoopMessage{
			MessageType: eventlooptypes.ApplicationEventLoopMessageType_StatusCheck,
		}, workspaceEventLoopMessageType_Event),
			Entry("reject the event if it is already completed", eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_WorkComplete,
				Event: &eventlooptypes.EventLoopEvent{
					ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
				},
			}, workspaceEventLoopMessageType_Event),
			Entry("reject the event if it is of unknown type", eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_WorkComplete,
				Event: &eventlooptypes.EventLoopEvent{
					ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
				},
			}, workspaceEventLoopMessageType("unknown")))

		It("managed env events should be forwarded", func() {

			By("creating a simple event referencing a ManagedEnvironment resource")
			event := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
				},
			}
			wrapperEvent := workspaceEventLoopMessage{
				messageType: workspaceEventLoopMessageType_Event,
				payload:     event,
			}

			responseChannel := make(chan workspaceResourceLoopMessage, 5)

			state := workspaceEventLoopInternalState{
				log: log.FromContext(ctx),
				workspaceResourceLoop: &workspaceResourceEventLoop{
					inputChannel: responseChannel,
				},
			}

			By("calling the function to process the event")
			handleWorkspaceEventLoopMessage(ctx, event, wrapperEvent, state)

			forwardedEvent := <-responseChannel
			Expect(forwardedEvent.messageType).To(Equal(workspaceResourceLoopMessageType_processManagedEnvironment), "event should have been forwarded")

		})

		DescribeTable("should verify applicationMap is updated correctly based on whether application event loop accepts or rejects the request",
			func(acceptRequest bool) {

				By("simulating a GitOpsDeployment event")

				request := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "some-namespace",
						Name:      "some-name",
					},
				}

				event := eventlooptypes.EventLoopMessage{
					MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event: &eventlooptypes.EventLoopEvent{
						Request:     request,
						ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
					},
				}

				wrapperEvent := workspaceEventLoopMessage{
					messageType: workspaceEventLoopMessageType_Event,
					payload:     event,
				}

				responseChannel := make(chan workspaceResourceLoopMessage, 5)

				tAELF := &managedEnvironmentTestApplicationEventLoopFactory{
					outputChannelMap: map[string]chan application_event_loop.RequestMessage{},
				}

				By("simulating a newly initialized workspace event loop")
				state := workspaceEventLoopInternalState{
					namespaceID: "namespace-id",
					log:         log.FromContext(ctx),
					workspaceResourceLoop: &workspaceResourceEventLoop{
						inputChannel: responseChannel,
					},
					applicationMap:       map[string]workspaceEventLoop_applicationEventLoopEntry{},
					applEventLoopFactory: tAELF,
				}

				By("mocking the application event loop: when it is started, we send a fake reply back via this goroutine")
				go func() {
					for {
						tAELF.waitForFirstInvocation(request.Name)

						fromOutputChan := <-tAELF.outputChannelMap[request.Name]
						By("testing both accept and rejected requests in this test")
						fromOutputChan.ResponseChan <- application_event_loop.ResponseMessage{RequestAccepted: acceptRequest}
					}
				}()

				By("call the function being test")
				handleWorkspaceEventLoopMessage(ctx, event, wrapperEvent, state)

				_, exists := state.applicationMap[state.namespaceID+"-"+request.Namespace+"-"+request.Name]
				Expect(exists).To(Equal(acceptRequest), "when the request is accepted, the entry should be in the map, but, the request is rejected it should be removed from the map")

			}, Entry("application event loop accepts the request", true), Entry("application event loop rejects the request", false))

		DescribeTable("it should handle orphaned sync run resources", func(gitOpsDeplCRExists bool, syncOperationRowExists bool) {
			Expect(db.SetupForTestingDBGinkgo()).To(Succeed())

			scheme, argocdNamespace, kubesystemNamespace, apiNamespace, err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			// createTestObjects creates:
			// - GitOpsDeploymentSyncRun and row
			// - GitOpsDeployment (and row)
			// - APICRToDBMapping linking them
			// However, depending on which parameters are used, some of these create steps may be skipped, to allow us to test different parts of the code (to ensure they function as expected)
			createTestObjects := func(createGitOpsDeplCR bool, createSyncOperationRow bool) (managedgitopsv1alpha1.GitOpsDeploymentSyncRun, managedgitopsv1alpha1.GitOpsDeployment) {
				syncRunCR := managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-name",
						Namespace: apiNamespace.Name,
						UID:       uuid.NewUUID(),
					},
					Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
						GitopsDeploymentName: "gitopsdepl-name",
					},
				}

				var gitopsDeplCR managedgitopsv1alpha1.GitOpsDeployment
				{

					dbQueries, err := db.NewUnsafePostgresDBQueries(false, true)
					Expect(err).ToNot(HaveOccurred())

					_, managedEnv, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbQueries)
					Expect(err).ToNot(HaveOccurred())

					application := db.Application{
						Application_id:          "test-application",
						Name:                    "hi",
						Spec_field:              "{}",
						Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
						Managed_environment_id:  managedEnv.Managedenvironment_id,
						Created_on:              time.Now(),
					}
					Expect(dbQueries.CreateApplication(ctx, &application)).To(Succeed())

					syncOperationRow := db.SyncOperation{
						SyncOperation_id:    "test-sync-operation",
						Application_id:      application.Application_id,
						DeploymentNameField: "gitopsdepl-name",
						DesiredState:        db.SyncOperation_DesiredState_Running,
						Created_on:          time.Now(),
						Revision:            "revision",
					}
					if createSyncOperationRow {
						Expect(dbQueries.CreateSyncOperation(ctx, &syncOperationRow)).To(Succeed())
					}

					gitopsDeplCR = managedgitopsv1alpha1.GitOpsDeployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gitopsdepl-name",
							Namespace: apiNamespace.Name,
						},
						Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{},
					}
					if createGitOpsDeplCR {
						Expect(k8sClient.Create(ctx, &gitopsDeplCR)).To(Succeed())
					}

					apiCRToDBMapping := db.APICRToDatabaseMapping{
						APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
						APIResourceUID:       string(syncRunCR.UID),
						APIResourceName:      syncRunCR.Name,
						APIResourceNamespace: apiNamespace.Name,
						NamespaceUID:         string(apiNamespace.UID),
						DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
						DBRelationKey:        syncOperationRow.SyncOperation_id,
					}

					err = dbQueries.CreateAPICRToDatabaseMapping(ctx, &apiCRToDBMapping)
					Expect(err).ToNot(HaveOccurred())

				}

				return syncRunCR, gitopsDeplCR
			}

			syncRunCR, gitopsDeplCR := createTestObjects(gitOpsDeplCRExists, syncOperationRowExists)

			By("creating a GitOpsDeploymentSyncRun event, which we will pass to the function for processing")
			event := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					Client:      k8sClient,
					ReqResource: eventlooptypes.GitOpsDeploymentSyncRunTypeName,
					Request:     ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&syncRunCR)},
				},
			}
			wrapperEvent := workspaceEventLoopMessage{
				messageType: workspaceEventLoopMessageType_Event,
				payload:     event,
			}

			By("starting a goroutine which mocks the start of the application event loop, and automatically accepts all requests from workspace event loop")
			responseChannel := make(chan workspaceResourceLoopMessage, 5)

			tAELF := &managedEnvironmentTestApplicationEventLoopFactory{
				outputChannelMap: map[string]chan application_event_loop.RequestMessage{},
			}
			go func() {
				defer GinkgoRecover()
				for {
					tAELF.waitForFirstInvocation(gitopsDeplCR.Name)

					fromOutputChan := <-tAELF.outputChannelMap[gitopsDeplCR.Name]
					fromOutputChan.ResponseChan <- application_event_loop.ResponseMessage{RequestAccepted: true}
				}
			}()

			state := workspaceEventLoopInternalState{
				log: log.FromContext(ctx),
				workspaceResourceLoop: &workspaceResourceEventLoop{
					inputChannel: responseChannel,
				},
				applEventLoopFactory: tAELF,
				applicationMap:       map[string]workspaceEventLoop_applicationEventLoopEntry{},
				namespaceID:          "namespace-id",
			}

			By("calling the function beign tested")
			handleWorkspaceEventLoopMessage(ctx, event, wrapperEvent, state)

			By("checking whether the application map of the state has been updated: the expected behaviour depends on which resources exist when the function is called.")

			_, exists := state.applicationMap[state.namespaceID+"-"+apiNamespace.Name+"-"+gitopsDeplCR.Name]

			if gitOpsDeplCRExists && syncOperationRowExists {
				// If SyncOperation exists, and points to a valid GitOpsDeployment, then the application event loop should have started, and thus the map entry should be set.
				Expect(exists).To(BeTrue())
			} else if gitOpsDeplCRExists && !syncOperationRowExists {
				// If the SyncOperation row doesn't exist in the database, then there is not a resource to unorphan, so the application event loop should not have started.
				Expect(exists).To(BeFalse())
			} else if !gitOpsDeplCRExists && syncOperationRowExists {
				// If SyncOperation exists, then the application event loop should have started, and thus the map entry should be set.
				Expect(exists).To(BeTrue())
			} else {
				Fail("unexpected permutation")
			}

		},
			Entry("GitOpsDeployment CR and SyncOperationRow exist", true, true),
			Entry("GitOpsDeployment CR exists but SyncOperationRow does not", true, false),
			Entry("GitOpsDeployment CR does not exist but SyncOperationRow does", false, true))

	})

	Context("Test getDBSyncOperationFromAPIMapping", func() {
		var (
			k8sClient client.Client
			ctx       context.Context
			dbQueries db.AllDatabaseQueries
			ns        *v1.Namespace
		)

		BeforeAll(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()

			Expect(err).ToNot(HaveOccurred())
			ns = apiNamespace
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			ctx = context.Background()
			dbQueries, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return an error if the namespace of the SyncRun CR is not found", func() {
			syncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-syncrun",
					Namespace: "unknown",
				},
			}

			syncOperation, err := getDBSyncOperationFromAPIMapping(ctx, dbQueries, k8sClient, syncRun)
			Expect(err).To(HaveOccurred())
			expectedMsg := `failed to get namespace unknown: namespaces "unknown" not found`
			Expect(err.Error()).To(Equal(expectedMsg))
			Expect(syncOperation).To(Equal(db.SyncOperation{}))
		})

		It("should return an error if there are no APICRToDatabaseMapping", func() {
			syncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-syncrun",
					Namespace: ns.Name,
				},
			}

			syncOperation, err := getDBSyncOperationFromAPIMapping(ctx, dbQueries, k8sClient, syncRun)
			Expect(err).To(HaveOccurred())
			expectedMsg := "SEVERE: unexpected number of APICRToDBMappings for GitOpsDeploymentSyncRun test-syncrun: 0"
			Expect(err.Error()).To(Equal(expectedMsg))
			Expect(syncOperation).To(Equal(db.SyncOperation{}))
		})
	})

	Context("handleStatusTickerMessage tests", func() {

		It("should remove any applications in the applicationMap which rejects the status check message", func() {

			readChanAndReturnResponse := func(channel chan application_event_loop.RequestMessage, acceptMessage bool) {
				go func() {
					req := <-channel
					req.ResponseChan <- application_event_loop.ResponseMessage{
						RequestAccepted: acceptMessage,
					}

				}()

			}

			By("creating a channel which will send a 'accept' reply")
			activeChan := make(chan application_event_loop.RequestMessage)

			By("creating a channel which will send a 'reject' reply")
			inactiveChan := make(chan application_event_loop.RequestMessage)

			By("associating each function with the corresponding channel type")
			readChanAndReturnResponse(activeChan, true)
			readChanAndReturnResponse(inactiveChan, false)

			state := workspaceEventLoopInternalState{
				applicationMap: map[string]workspaceEventLoop_applicationEventLoopEntry{
					"active":   {input: activeChan},
					"inactive": {input: inactiveChan},
				},
			}

			handleStatusTickerMessage(state)

			Consistently(func() bool {
				_, exists := state.applicationMap["active"]
				return exists
			}).Should(BeTrue(), "the active application should have accepted the request, and thus should still be present")

			Eventually(func() bool {
				_, exists := state.applicationMap["inactive"]
				return exists
			}).Should(BeFalse(), "the inactive application should have rejected the request, and thus should have been removed from the map")

		})

	})
})

// testApplicationEventLoopFactory is a mock applicationEventQueueLoopFactory, used for unit tests above.
type testApplicationEventLoopFactory struct {
	mutex                     sync.Mutex
	outputChannel             chan application_event_loop.RequestMessage
	numberOfEventLoopsCreated int
}

var _ applicationEventQueueLoopFactory = &testApplicationEventLoopFactory{}

func (ta *testApplicationEventLoopFactory) waitForFirstInvocation() {

	Eventually(func() bool {
		ta.mutex.Lock()
		defer ta.mutex.Unlock()

		return ta.outputChannel != nil

	}).Should(BeTrue())

}

// Instead of starting a new application event queue loop (like the default implementation of applictionEventQueueLoopFactory)
// we instead just return a previously provided channel.
func (ta *testApplicationEventLoopFactory) startApplicationEventQueueLoop(ctx context.Context,
	aeqlParam application_event_loop.ApplicationEventQueueLoop) error {

	ta.mutex.Lock()
	defer ta.mutex.Unlock()

	ta.outputChannel = aeqlParam.InputChan

	// Increase count by 1 if new application event loop is created
	ta.numberOfEventLoopsCreated++

	return nil
}

// testApplicationEventLoopFactory is a mock applicationEventQueueLoopFactory, used for unit tests above.
type managedEnvironmentTestApplicationEventLoopFactory struct {
	outputChannelMap          map[string]chan application_event_loop.RequestMessage
	numberOfEventLoopsCreated int
	mutex                     sync.Mutex
}

var _ applicationEventQueueLoopFactory = &managedEnvironmentTestApplicationEventLoopFactory{}

// Instead of starting a new application event queue loop (like the default implementation of applictionEventQueueLoopFactory)
// we instead just return a previously provided channel.
func (ta *managedEnvironmentTestApplicationEventLoopFactory) startApplicationEventQueueLoop(ctx context.Context,
	aeqlParam application_event_loop.ApplicationEventQueueLoop) error {

	ta.mutex.Lock()
	defer ta.mutex.Unlock()

	ta.outputChannelMap[aeqlParam.GitopsDeploymentName] = aeqlParam.InputChan

	// Increase count by 1 if new application event loop is created
	ta.numberOfEventLoopsCreated++

	return nil
}

func (ta *managedEnvironmentTestApplicationEventLoopFactory) waitForFirstInvocation(gitopsDeploymentName string) {

	Eventually(func() bool {
		ta.mutex.Lock()
		defer ta.mutex.Unlock()

		return ta.outputChannelMap[gitopsDeploymentName] != nil

	}, "5m").Should(BeTrue())

}
