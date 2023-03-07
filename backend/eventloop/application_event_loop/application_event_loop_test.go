package application_event_loop

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ApplicationEventLoop Tests", func() {

	Context("startApplicationEventQueueLoopWithFactory test", func() {

		It("should forward messages received on the input channel, to the runner, without a response channel", func() {

			mockApplicationEventLoopRunnerFactory := mockApplicationEventLoopRunnerFactory{
				mockChannel: make(chan *eventlooptypes.EventLoopEvent),
			}

			aeqlParam := ApplicationEventQueueLoop{
				GitopsDeploymentName:      "",
				GitopsDeploymentNamespace: "",
				WorkspaceID:               "",
				SharedResourceEventLoop:   nil,
				InputChan:                 make(chan RequestMessage),
			}

			startApplicationEventQueueLoopWithFactory(context.Background(), aeqlParam, &mockApplicationEventLoopRunnerFactory)

			inputEvent := &eventlooptypes.EventLoopEvent{
				EventType:   eventlooptypes.DeploymentModified,
				Request:     reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "", Name: ""}},
				Client:      nil,
				ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
				WorkspaceID: "",
			}

			aeqlParam.InputChan <- RequestMessage{
				Message: eventlooptypes.EventLoopMessage{
					MessageType:       eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event:             inputEvent,
					ShutdownSignalled: false,
				},
				ResponseChan: nil, // no response channel needed
			}

			outputEvent := <-mockApplicationEventLoopRunnerFactory.mockChannel
			Expect(outputEvent).NotTo(BeNil())
			Expect(outputEvent.EventType).To(Equal(inputEvent.EventType))
			Expect(outputEvent.Request).To(Equal(inputEvent.Request))
			Expect(outputEvent.ReqResource).To(Equal(inputEvent.ReqResource))
			Expect(outputEvent.WorkspaceID).To(Equal(inputEvent.WorkspaceID))

		})
	})

	Context("Test processApplicationEventQueueLoopMessage", func() {

		var ctx context.Context
		var state applicationEventQueueLoopState
		var klog logr.Logger
		var k8sClient client.WithWatch
		var workspaceUID string

		var input chan RequestMessage
		var responseChan chan ResponseMessage

		BeforeEach(OncePerOrdered, func() {
			scheme, argocdNamespace, kubesystemNamespace, workspace, err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			workspaceUID = string(workspace.UID)

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, argocdNamespace, kubesystemNamespace).Build()

			ctx = context.Background()

			klog = log.FromContext(ctx)

			state = applicationEventQueueLoopState{
				activeDeploymentEvent:      nil,
				waitingDeploymentEvents:    []*RequestMessage{},
				activeSyncOperationEvent:   nil,
				waitingSyncOperationEvents: []*RequestMessage{},

				deploymentEventRunner:         make(chan *eventlooptypes.EventLoopEvent, 5),
				deploymentEventRunnerShutdown: false,

				syncOperationEventRunner:         make(chan *eventlooptypes.EventLoopEvent, 5),
				syncOperationEventRunnerShutdown: false,
			}

			input = make(chan RequestMessage, 5)
			responseChan = make(chan ResponseMessage, 5)

		})

		Context("syncOperation event tests", func() {

			When("there are no active syncOperationEvents, and an event is sent to the queue", func() {

				It("should queue a new active event", func() {
					newEvent := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
							Event: &eventlooptypes.EventLoopEvent{
								EventType:   eventlooptypes.SyncRunModified,
								ReqResource: eventlooptypes.GitOpsDeploymentSyncRunTypeName,
								WorkspaceID: string(workspaceUID),
							},
							ShutdownSignalled: false,
						},
						ResponseChan: responseChan,
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, newEvent, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())

					event := <-state.syncOperationEventRunner
					Expect(event).To(Equal(newEvent.Message.Event))
					Expect(state.waitingSyncOperationEvents).To(HaveLen(0))
					Expect(state.activeSyncOperationEvent.Message).To(Equal(newEvent.Message))
				})

			})

			When("there are are already active syncOperationEvents, and an event is sent to the queue", Ordered, func() {

				var existingActiveMessage *RequestMessage
				var newEvent RequestMessage

				It("should add the new event to the waiting loop, but not queue it", func() {

					existingActiveMessage = &RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
							Event: &eventlooptypes.EventLoopEvent{
								ReqResource: eventlooptypes.GitOpsDeploymentSyncRunTypeName,
								WorkspaceID: "existing-event",
							},
						},
					}

					state.activeSyncOperationEvent = existingActiveMessage

					newEvent = RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
							Event: &eventlooptypes.EventLoopEvent{
								EventType:   eventlooptypes.SyncRunModified,
								ReqResource: eventlooptypes.GitOpsDeploymentSyncRunTypeName,
								WorkspaceID: string(workspaceUID),
							},
							ShutdownSignalled: false,
						},
						ResponseChan: responseChan,
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, newEvent, &state, input, k8sClient, klog)

					Expect(shouldTerminate).To(BeFalse())
					Consistently(state.syncOperationEventRunner).ShouldNot(Receive(), "it should not queue a new event, as an existing event is already active")
					Expect(state.waitingSyncOperationEvents).To(HaveLen(1))
					Expect(*state.waitingSyncOperationEvents[0]).To(Equal(newEvent), "our new event should be waiting to be processed")
					Expect(*state.activeSyncOperationEvent).To(Equal(*existingActiveMessage), "the old active event should still be active")

				})

				It("should queue the new event, on work complete of the old event", func() {
					workComplete := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType:       eventlooptypes.ApplicationEventLoopMessageType_WorkComplete,
							Event:             existingActiveMessage.Message.Event,
							ShutdownSignalled: false,
						},
						ResponseChan: nil,
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, workComplete, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())

					event := <-state.syncOperationEventRunner
					Expect(event).To(Equal(newEvent.Message.Event))
					Expect(state.waitingSyncOperationEvents).To(HaveLen(0))
					Expect(state.activeSyncOperationEvent.Message).To(Equal(newEvent.Message))

				})
				It("should have no active events once the new event completes, and should respect shutdown signalled", func() {
					workComplete := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType:       eventlooptypes.ApplicationEventLoopMessageType_WorkComplete,
							Event:             newEvent.Message.Event,
							ShutdownSignalled: true,
						},
						ResponseChan: nil,
					}
					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, workComplete, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())
					Expect(state.syncOperationEventRunnerShutdown).To(BeTrue())
					Expect(state.waitingSyncOperationEvents).To(HaveLen(0))
					Expect(state.activeSyncOperationEvent).To(BeNil())
				})
			})
		})

		Context("Deployment event tests", func() {

			When("there are no active deploy events, and an event is sent to the queue", func() {

				It("should set a new active event", func() {
					newEvent := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
							Event: &eventlooptypes.EventLoopEvent{
								EventType:   eventlooptypes.DeploymentModified,
								ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
								WorkspaceID: string(workspaceUID),
							},
							ShutdownSignalled: false,
						},
						ResponseChan: responseChan,
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, newEvent, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())

					event := <-state.deploymentEventRunner
					Expect(event).To(Equal(newEvent.Message.Event))
					Expect(state.waitingDeploymentEvents).To(HaveLen(0))
					Expect(state.activeDeploymentEvent.Message).To(Equal(newEvent.Message))
				})

			})

			When("there are are already active deploy events, and a new event is sent to the queue", Ordered, func() {

				var existingActiveMessage *RequestMessage
				var newEvent RequestMessage

				It("should add the new event to the waiting loop, but not queue it", func() {

					existingActiveMessage = &RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
							Event: &eventlooptypes.EventLoopEvent{
								ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
								WorkspaceID: "existing-event",
							},
						},
					}

					state.activeDeploymentEvent = existingActiveMessage

					newEvent = RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
							Event: &eventlooptypes.EventLoopEvent{
								EventType:   eventlooptypes.DeploymentModified,
								ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
								WorkspaceID: string(workspaceUID),
							},
							ShutdownSignalled: false,
						},
						ResponseChan: responseChan,
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, newEvent, &state, input, k8sClient, klog)

					Expect(shouldTerminate).To(BeFalse())
					Consistently(state.deploymentEventRunner).ShouldNot(Receive(), "it should not queue a new event, as an existing event is already active")
					Expect(state.waitingDeploymentEvents).To(HaveLen(1))
					Expect(*state.waitingDeploymentEvents[0]).To(Equal(newEvent), "our new event should be waiting to be processed")
					Expect(*state.activeDeploymentEvent).To(Equal(*existingActiveMessage), "the old active event should still be active")

				})

				It("should queue the new event, on work complete of the old event", func() {
					workComplete := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType:       eventlooptypes.ApplicationEventLoopMessageType_WorkComplete,
							Event:             existingActiveMessage.Message.Event,
							ShutdownSignalled: false,
						},
						ResponseChan: nil,
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, workComplete, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())

					event := <-state.deploymentEventRunner
					Expect(event).To(Equal(newEvent.Message.Event))
					Expect(state.waitingDeploymentEvents).To(HaveLen(0))
					Expect(state.activeDeploymentEvent.Message).To(Equal(newEvent.Message))

				})
				It("should have no active events once the new event completes, and should respect shutdown signalled", func() {
					workComplete := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType:       eventlooptypes.ApplicationEventLoopMessageType_WorkComplete,
							Event:             newEvent.Message.Event,
							ShutdownSignalled: true,
						},
						ResponseChan: nil,
					}
					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, workComplete, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())
					Expect(state.deploymentEventRunnerShutdown).To(BeTrue())
					Expect(state.waitingDeploymentEvents).To(HaveLen(0))
					Expect(state.activeDeploymentEvent).To(BeNil())
				})
			})
		})

		Context("ManagedEnvironment event tests", func() {

			When("A ManagedEnvironment event is sent to application event queue loop", Ordered, func() {

				var newEvent RequestMessage

				It("should queue the ManagedEnvironment event as a deployment, as this is where ManagedEnv events are processed", func() {
					newEvent = RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
							Event: &eventlooptypes.EventLoopEvent{
								EventType:   eventlooptypes.ManagedEnvironmentModified,
								ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
								WorkspaceID: string(workspaceUID),
							},
							ShutdownSignalled: false,
						},
						ResponseChan: responseChan,
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, newEvent, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())

					event := <-state.deploymentEventRunner
					Expect(event).To(Equal(newEvent.Message.Event))
					Expect(state.waitingDeploymentEvents).To(HaveLen(0))
					Expect(state.activeDeploymentEvent.Message).To(Equal(newEvent.Message))

				})

				It("should handle work completed on ManagedEnvironment event", func() {

					workComplete := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType:       eventlooptypes.ApplicationEventLoopMessageType_WorkComplete,
							Event:             newEvent.Message.Event,
							ShutdownSignalled: true,
						},
						ResponseChan: nil,
					}
					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, workComplete, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())
					Expect(state.deploymentEventRunnerShutdown).To(BeTrue())
					Expect(state.waitingDeploymentEvents).To(HaveLen(0))
					Expect(state.activeDeploymentEvent).To(BeNil())
				})
			})

		})

		Context("UpdateDeploymentStatusTick event tests", func() {

			When("an UpdateDeploymentStatusTick event is sent to event loop", Ordered, func() {

				It("it should set the event to active under the deployment runner", func() {
					newEvent := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
							Event: &eventlooptypes.EventLoopEvent{
								EventType:   eventlooptypes.UpdateDeploymentStatusTick,
								WorkspaceID: string(workspaceUID),
							},
							ShutdownSignalled: false,
						},
						ResponseChan: responseChan,
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, newEvent, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())

					event := <-state.deploymentEventRunner
					Expect(event).To(Equal(newEvent.Message.Event))
					Expect(state.waitingDeploymentEvents).To(HaveLen(0))
					Expect(state.activeDeploymentEvent.Message).To(Equal(newEvent.Message))

				})

				It("should start a new status update goroutine when work complete event is sent", func() {

					Expect(state.activeDeploymentEvent).ToNot(BeNil())

					workCompleteEvent := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType: eventlooptypes.ApplicationEventLoopMessageType_WorkComplete,
							Event: &eventlooptypes.EventLoopEvent{
								EventType:   eventlooptypes.UpdateDeploymentStatusTick,
								WorkspaceID: string(workspaceUID),
							},
							ShutdownSignalled: false,
						},
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, workCompleteEvent, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())
					Expect(state.activeDeploymentEvent).To(BeNil())

				})

			})
		})

		Context("StatusCheck event tests", func() {

			When("an application event loop has not shutdown", func() {
				It("should verify the statuscheck message indicates that the status check request was accepted", func() {
					newEvent := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType:       eventlooptypes.ApplicationEventLoopMessageType_StatusCheck,
							ShutdownSignalled: false,
						},
						ResponseChan: responseChan,
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, newEvent, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeFalse())

					event := <-newEvent.ResponseChan
					Expect(event.RequestAccepted).To(BeTrue())
				})

			})

			When("An application event loop has fully shutdown", func() {
				It("should verify the statuscheck message should indicate that the status check request was accepted", func() {

					state.deploymentEventRunnerShutdown = true
					state.syncOperationEventRunnerShutdown = true

					newEvent := RequestMessage{
						Message: eventlooptypes.EventLoopMessage{
							MessageType:       eventlooptypes.ApplicationEventLoopMessageType_StatusCheck,
							ShutdownSignalled: false,
						},
						ResponseChan: responseChan,
					}

					shouldTerminate := processApplicationEventQueueLoopMessage(ctx, newEvent, &state, input, k8sClient, klog)
					Expect(shouldTerminate).To(BeTrue())

					event := <-newEvent.ResponseChan
					Expect(event.RequestAccepted).To(BeFalse())
				})

			})

		})

	})

	Context("Simulate a deleted GitOpsDeployment", Ordered, func() {

		const (
			gitopsDeplName      = "my-gitops-depl"
			gitopsDeplNamespace = "my-namespace"
			namespaceUID        = "my-namespace-uid"
		)

		aeqlParam := ApplicationEventQueueLoop{
			GitopsDeploymentName:      gitopsDeplName,
			GitopsDeploymentNamespace: gitopsDeplNamespace,
			WorkspaceID:               namespaceUID,
			SharedResourceEventLoop:   nil,
			InputChan:                 make(chan RequestMessage),
		}

		event := &eventlooptypes.EventLoopEvent{
			EventType:   eventlooptypes.DeploymentModified,
			Request:     reconcile.Request{NamespacedName: types.NamespacedName{Namespace: gitopsDeplNamespace, Name: gitopsDeplName}},
			Client:      nil,
			ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
			WorkspaceID: namespaceUID,
		}

		It("should start a runner based on incoming GitOpsDeployment request ", func() {
			mockApplicationEventLoopRunnerFactory := mockApplicationEventLoopRunnerFactory{
				mockChannel: make(chan *eventlooptypes.EventLoopEvent),
			}

			startApplicationEventQueueLoopWithFactory(context.Background(), aeqlParam, &mockApplicationEventLoopRunnerFactory)

			By("sending message to the application event loop, informing it of a new GitOpsDeployment")
			responseChan := make(chan ResponseMessage)
			aeqlParam.InputChan <- RequestMessage{
				Message: eventlooptypes.EventLoopMessage{
					MessageType:       eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event:             event,
					ShutdownSignalled: false,
				},
				ResponseChan: responseChan,
			}

			By("By reading the response message from the application event loop")
			response := <-responseChan
			Expect(response.RequestAccepted).To(BeTrue())

			outputEvent := <-mockApplicationEventLoopRunnerFactory.mockChannel
			Expect(outputEvent).NotTo(BeNil(), "we should have received the event on a runner")
			Expect(outputEvent.EventType).To(Equal(event.EventType))
			Expect(outputEvent.Request).To(Equal(event.Request))
			Expect(outputEvent.ReqResource).To(Equal(event.ReqResource))
			Expect(outputEvent.WorkspaceID).To(Equal(event.WorkspaceID))

		})

		It("should shutdown the runner based on work complete, and then verify the event loop rejects additional work", func() {

			By("sending message to application event loop that the runner is complete")
			aeqlParam.InputChan <- RequestMessage{
				Message: eventlooptypes.EventLoopMessage{
					MessageType:       eventlooptypes.ApplicationEventLoopMessageType_WorkComplete,
					Event:             event,
					ShutdownSignalled: true,
				},
				ResponseChan: nil,
			}

			By("by attempting to send another request to the application event loop")
			responseChan := make(chan ResponseMessage)
			aeqlParam.InputChan <- RequestMessage{
				Message: eventlooptypes.EventLoopMessage{
					MessageType:       eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event:             event,
					ShutdownSignalled: false,
				},
				ResponseChan: responseChan,
			}

			response := <-responseChan
			Expect(response.RequestAccepted).To(BeFalse(), "the second request to the event loop should be rejected, as "+
				"the application event loop has terminate.")

			By("attempting to send another request to the application event loop")
			mutex := sync.Mutex{}
			responseReceived := false
			go func() {
				responseChan = make(chan ResponseMessage)
				aeqlParam.InputChan <- RequestMessage{
					Message: eventlooptypes.EventLoopMessage{
						MessageType:       eventlooptypes.ApplicationEventLoopMessageType_Event,
						Event:             event,
						ShutdownSignalled: false,
					},
					ResponseChan: nil,
				}
				mutex.Lock()
				defer mutex.Unlock()
				responseReceived = true

			}()

			Consistently(func() bool {
				mutex.Lock()
				defer mutex.Unlock()
				return responseReceived
			}).WithTimeout(time.Second).WithPolling(time.Millisecond*10).Should(BeFalse(),
				"we should not receive a response, because the event loop goroutine has terminated.")
		})
	})

})

// mockApplicationEventLoopRunnerFactory which returns a pre-provided channel, rather than starting a new goroutine.
type mockApplicationEventLoopRunnerFactory struct {
	mockChannel chan *eventlooptypes.EventLoopEvent
}

var _ applicationEventRunnerFactory = &mockApplicationEventLoopRunnerFactory{}

func (fact *mockApplicationEventLoopRunnerFactory) createNewApplicationEventLoopRunner(informWorkCompleteChan chan RequestMessage,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop, gitopsDeplName string, gitopsDeplNamespace string,
	workspaceID string, debugContext string) chan *eventlooptypes.EventLoopEvent {

	return fact.mockChannel

}
