package application_event_loop

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ApplicationEventLoop Test", func() {

	Context("Application Event Loop test", func() {

		It("should forward messages received on the input channel, to the runner, without a response channel", func() {

			mockApplicationEventLoopRunnerFactory := mockApplicationEventLoopRunnerFactory{
				mockChannel: make(chan *eventlooptypes.EventLoopEvent),
			}

			aeqlParam := ApplicationEventQueueLoop{
				GitopsDeploymentName:      "",
				GitopsDeploymentNamespace: "",
				WorkspaceID:               "",
				SharedResourceEventLoop:   nil,
				VwsAPIExportName:          "gitops-api",
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
			VwsAPIExportName:          "gitops-api",
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
