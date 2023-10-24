package application_event_loop

import (
	"context"
	"math/rand"
	"time"

	"github.com/go-logr/logr"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	logutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/log"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// All events that occur to a particular GitOpsDeployment CR, and any CRs (such as GitOpsDeploymentSyncRun) that reference
// GitOpsDeployment, are received by the Application Event Loop.

// The responsibility of the Application Event Loop is to:
// - Receive events for a single, specific application (a specific GitOpsDeployment, or a GitOpsDeploymentSyncRun that
//   is referencing that GitOpsDeployment)
// - Pass received events to the appropriate application event runner.
// - Ensure an application event runner exists for each resource type.
// - Within an Application Event Loop:
//     - 1 application event runner exists for handling GitOpsDeployment events
//     - 1 application event runner exists for handling GitOpsDeploymentSyncRun events

// Cardinality: 1 instance of the Application Event Loop exists per GitOpsDeployment CR
// (which also corresponds to 1 instance of the Application Event Loop per Argo CD Application CR,
// as there is also a 1:1 relationship between GitOpsDeployments CRs and Argo CD Application CRs).

const (
	// deploymentStatusTickRate is the rate at which the application event runner will be sent a message
	// indicating that the GitOpsDeployment status field should be updated.
	deploymentStatusTickRate = 15 * time.Second

	// disableDeploymentStatusTickLogging disables logging of events related to the deployment status.
	// - These events tend to be noisy, and this will help reduce this noise when debugging.
	disableDeploymentStatusTickLogging = false
)

// RequestMessage is a message sent to the Application Event Loop by the Workspace Event loop.
type RequestMessage struct {
	// Message is the primary message contents of the message sent to the Application Event loop
	Message eventlooptypes.EventLoopMessage

	// If the sender of the message would like a reply to the message, they may pass a channel as
	// part of the message. The receiver will reply on this channel.
	// - ResponseChan may be nil, if no response is needed.
	// - Not all message types support ResponseChan
	ResponseChan chan ResponseMessage
}

// ResponseMesage is a message sent back on the ResponseChan of the RequestMessage
type ResponseMessage struct {

	// RequestAccepted is true if the Application Event Loop is actively accepting message, and false
	// if the request was rejected (because the Application Event Loop has shutdown)
	RequestAccepted bool
}

// ApplicationEventQueueLoop contains the variables required to initialize an Application Event Loop. These refer
// to the particular GitOpsDeployment that a Application Event Loop is responsible for handling.
// - All fields are immutable: they should not be changed once first written.
type ApplicationEventQueueLoop struct {

	// GitopsDeploymentName is the GitOpsDeployment resource that this Appliction Event Loop is reponsible for handling
	GitopsDeploymentName string

	// GitopsDeploymentNamespace is the namespace of the above GitOpsDeployment
	GitopsDeploymentNamespace string

	// WorkspaceID is the UID of the namespace
	WorkspaceID string

	// SharedResourceEventLoop is a reference to the shared resource event loop
	SharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop

	// InputChan is the channel that this Application Event Loop will listen for mesages on.
	InputChan chan RequestMessage

	// Client is a K8s client for accessing GitOps service resources
	Client client.Client
}

// StartApplicationEventQueueLoop will start the Application Event Loop for the GitOpsDeployment referenced
// in the aeqlParam parameter.
func StartApplicationEventQueueLoop(ctx context.Context, aeqlParam ApplicationEventQueueLoop) {

	go applicationEventQueueLoop(ctx,
		aeqlParam.Client,
		aeqlParam.InputChan,
		aeqlParam.GitopsDeploymentName,
		aeqlParam.GitopsDeploymentNamespace,
		aeqlParam.WorkspaceID,
		aeqlParam.SharedResourceEventLoop,
		defaultApplicationEventRunnerFactory{}, // use the default factory
	)
}

// startApplicationEventQueueLoopWithFactory allows a custom applicationEventLoop factory to be passed, to replace
// the default factory. This function should only be called/useful for unit tests.
func startApplicationEventQueueLoopWithFactory(ctx context.Context, aeqlParam ApplicationEventQueueLoop, aerFactory applicationEventRunnerFactory) {

	go applicationEventQueueLoop(ctx,
		aeqlParam.Client,
		aeqlParam.InputChan,
		aeqlParam.GitopsDeploymentName,
		aeqlParam.GitopsDeploymentNamespace,
		aeqlParam.WorkspaceID,
		aeqlParam.SharedResourceEventLoop,
		aerFactory, // use parameter-provided factory
	)

}

type applicationEventQueueLoopState struct {

	// Only one deployment event is processed at a time
	// These are events that are waiting to be sent to application_event_runner_deployments runner
	activeDeploymentEvent   *RequestMessage
	waitingDeploymentEvents []*RequestMessage

	// Only one sync operation event is processed at a time
	// For example: if the user created multiple GitOpsDeploymentSyncRun CRs, they will be processed to completion, one at a time.
	// These are events that are waiting to be sent to application_event_runner_syncruns runner
	activeSyncOperationEvent   *RequestMessage
	waitingSyncOperationEvents []*RequestMessage

	// Channel which can be used to communicate with the active deployment event runner
	deploymentEventRunner chan eventlooptypes.EventLoopEvent

	// deploymentEventRunnerShutdown is true if the runner has shut down, false otherwise
	deploymentEventRunnerShutdown bool

	// Channel which can be used to communicate with the active sync event runner
	syncOperationEventRunner chan eventlooptypes.EventLoopEvent

	// syncOperationEventRunnerShutdown is true if the runner has shut down, false otherwise
	syncOperationEventRunnerShutdown bool
}

// applicationEventQueueLoop is the main function of the application event loop: it accepts messages from the
// workspace event loop, creates runners, and passes messages to runners.
func applicationEventQueueLoop(ctx context.Context, k8sClient client.Client,
	input chan RequestMessage,
	gitopsDeploymentName string, gitopsDeploymentNamespace string,
	workspaceID string,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop,
	aerFactory applicationEventRunnerFactory) {

	log := log.FromContext(ctx).
		WithValues(logutil.Log_K8s_Request_NamespaceID, workspaceID,
			logutil.Log_K8s_Request_Name, gitopsDeploymentName,
			logutil.Log_K8s_Request_Namespace, gitopsDeploymentNamespace).
		WithName(logutil.LogLogger_managed_gitops)

	log.V(logutil.LogLevel_Debug).Info("applicationEventQueueLoop started.")
	defer log.V(logutil.LogLevel_Debug).Info("applicationEventQueueLoop ended.")

	state := applicationEventQueueLoopState{
		activeDeploymentEvent:      nil,
		waitingDeploymentEvents:    []*RequestMessage{},
		activeSyncOperationEvent:   nil,
		waitingSyncOperationEvents: []*RequestMessage{},

		deploymentEventRunner: aerFactory.createNewApplicationEventLoopRunner(ctx, input, sharedResourceEventLoop, gitopsDeploymentName,
			gitopsDeploymentNamespace, workspaceID, "deployment", ExistingK8sClientFactory{existingK8sClient: k8sClient}),
		deploymentEventRunnerShutdown: false,

		syncOperationEventRunner: aerFactory.createNewApplicationEventLoopRunner(ctx, input, sharedResourceEventLoop, gitopsDeploymentName,
			gitopsDeploymentNamespace, workspaceID, "sync-operation", ExistingK8sClientFactory{existingK8sClient: k8sClient}),
		syncOperationEventRunnerShutdown: false,
	}

	// Start the ticker, which will -- every X seconds -- instruct the GitOpsDeployment CR fields to update
	startNewStatusUpdateTimer(ctx, k8sClient, input, log)

	for {
		// Block on waiting for more events for this application
		newEvent := <-input

		// The event loop will be terminated if the application no longer exists
		if terminateEventLoop := processApplicationEventQueueLoopMessage(ctx, newEvent, &state, input, k8sClient, log); terminateEventLoop {
			break
		}
	}
}

// returns true if the event loop should be terminated, false otherwise.
func processApplicationEventQueueLoopMessage(ctx context.Context, newEvent RequestMessage, state *applicationEventQueueLoopState, input chan RequestMessage, k8sClient client.Client, log logr.Logger) bool {

	const (
		terminateEventLoop_false = false
		terminateEventLoop_true  = true
	)

	eventLoopMessage := newEvent.Message.Event

	if eventLoopMessage != nil {
		if !(eventLoopMessage.EventType == eventlooptypes.UpdateDeploymentStatusTick && disableDeploymentStatusTickLogging) {
			log.V(logutil.LogLevel_Debug).Info("applicationEventQueueLoop received event",
				"event", eventlooptypes.StringEventLoopEvent(eventLoopMessage))
		}
	}

	// If we've received a new event from the workspace event loop
	if newEvent.Message.MessageType == eventlooptypes.ApplicationEventLoopMessageType_Event {

		if eventLoopMessage == nil {
			log.Error(nil, "SEVERE: applicationEventQueueLoop event was nil", "thing", newEvent.Message.MessageType)
			return terminateEventLoop_false
		}

		// First check if the event loop has shutdown, and therefore we should reject the request
		workRejected := state.deploymentEventRunnerShutdown && state.syncOperationEventRunnerShutdown

		// We reject the message from the workspace event loop if we have already shutdown
		// - the workspace event loop will receive the rejection, and start up a new event loop
		if newEvent.ResponseChan != nil {

			// Inform the event loop if we have accepted/rejected their message
			newEvent.ResponseChan <- ResponseMessage{
				RequestAccepted: !workRejected,
			}

			// Terminate this event loop once we inform the workspace event loop that we have shutdown.
			if workRejected {
				return terminateEventLoop_true
			}
		}

		// Otherwise, if the response chan is nil, but work is rejected, just continue
		if workRejected {
			return terminateEventLoop_false
		}

		// From this point on, the work has been accepted

		log := log.WithValues("event", eventlooptypes.StringEventLoopEvent(eventLoopMessage))

		if eventLoopMessage.ReqResource == eventlooptypes.GitOpsDeploymentTypeName {

			if !state.deploymentEventRunnerShutdown {
				state.waitingDeploymentEvents = append(state.waitingDeploymentEvents, &newEvent)
			} else {
				log.V(logutil.LogLevel_Debug).Info("Ignoring post-shutdown deployment event")
			}

		} else if eventLoopMessage.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {

			if !state.syncOperationEventRunnerShutdown {
				state.waitingSyncOperationEvents = append(state.waitingSyncOperationEvents, &newEvent)
			} else {
				log.V(logutil.LogLevel_Debug).Info("Ignoring post-shutdown sync operation event")
			}
		} else if eventLoopMessage.ReqResource == eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName {

			if !state.deploymentEventRunnerShutdown {
				state.waitingDeploymentEvents = append(state.waitingDeploymentEvents, &newEvent)
			} else {
				log.V(logutil.LogLevel_Debug).Info("Ignoring post-shutdown managed environment event")
			}

		} else if eventLoopMessage.EventType == eventlooptypes.UpdateDeploymentStatusTick {

			if !state.deploymentEventRunnerShutdown {
				state.waitingDeploymentEvents = append(state.waitingDeploymentEvents, &newEvent)
			} else {
				log.V(logutil.LogLevel_Debug).Info("Ignoring post-shutdown deployment event")
			}

		} else {
			log.Error(nil, "SEVERE: unexpected event resource type in applicationEventQueueLoop")
		}

		// If we've received a work complete notification...
	} else if newEvent.Message.MessageType == eventlooptypes.ApplicationEventLoopMessageType_WorkComplete {

		if eventLoopMessage == nil {
			log.Error(nil, "SEVERE: applicationEventQueueLoop event was nil", "thing", newEvent.Message.MessageType)
			return terminateEventLoop_false
		}

		if newEvent.ResponseChan != nil {
			log.Error(nil, "SEVERE: WorkComplete does not support non-nil ResponseChan")
		}

		log := log.WithValues("event", eventlooptypes.StringEventLoopEvent(eventLoopMessage))

		if !(eventLoopMessage.EventType == eventlooptypes.UpdateDeploymentStatusTick && disableDeploymentStatusTickLogging) {

			log.V(logutil.LogLevel_Debug).Info("applicationEventQueueLoop received work complete event")
		}

		if eventLoopMessage.EventType == eventlooptypes.UpdateDeploymentStatusTick {
			// After we finish processing a previous status tick, start the timer to queue up a new one.
			// This ensures we are always reminded to do a status update.
			state.activeDeploymentEvent = nil
			startNewStatusUpdateTimer(ctx, k8sClient, input, log)

		} else if eventLoopMessage.ReqResource == eventlooptypes.GitOpsDeploymentTypeName ||
			eventLoopMessage.ReqResource == eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName {

			if mismatchingField := eventlooptypes.EventsMatch(state.activeDeploymentEvent.Message.Event, newEvent.Message.Event); mismatchingField != "" {
				log.Error(nil, "SEVERE: unmatched deployment event work item",
					"activeDeploymentEvent", eventlooptypes.StringEventLoopEvent(state.activeDeploymentEvent.Message.Event),
					"newEvent", eventlooptypes.StringEventLoopEvent(newEvent.Message.Event),
					"mismatchingField", mismatchingField)
			}

			state.activeDeploymentEvent = nil

			state.deploymentEventRunnerShutdown = newEvent.Message.ShutdownSignalled
			if state.deploymentEventRunnerShutdown {
				log.Info("Deployment signalled shutdown")
			}

		} else if eventLoopMessage.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {

			if mismatchingField := eventlooptypes.EventsMatch(state.activeSyncOperationEvent.Message.Event, newEvent.Message.Event); mismatchingField != "" {
				log.Error(nil, "SEVERE: unmatched sync operation event work item",
					"activeSyncOperationEvent", eventlooptypes.StringEventLoopEvent(state.activeSyncOperationEvent.Message.Event),
					"newEvent", eventlooptypes.StringEventLoopEvent(newEvent.Message.Event),
					"mismatchingField", mismatchingField)
			}

			state.activeSyncOperationEvent = nil
			state.syncOperationEventRunnerShutdown = newEvent.Message.ShutdownSignalled
			if state.syncOperationEventRunnerShutdown {
				log.Info("Sync operation signalled shutdown")
			}

		} else {
			log.Error(nil, "SEVERE: unexpected event resource type in applicationEventQueueLoop")
		}

		// If the parent workspace event loop is requesting our status...
	} else if newEvent.Message.MessageType == eventlooptypes.ApplicationEventLoopMessageType_StatusCheck {

		if newEvent.ResponseChan == nil { // sanity check the response chan
			log.Error(nil, "SEVERE: StatusCheck does not support nil ResponseChan")
			return terminateEventLoop_false
		}

		workRejected := state.deploymentEventRunnerShutdown && state.syncOperationEventRunnerShutdown

		// Inform the event loop if we have accepted/rejected their message
		newEvent.ResponseChan <- ResponseMessage{
			RequestAccepted: !workRejected,
		}

		// Terminate the event loop once we inform the workspace event loop that we have shutdown.
		if workRejected {
			return terminateEventLoop_true
		}

	} else {
		log.Error(nil, "SEVERE: Unrecognized workspace event type", "type", newEvent.Message.MessageType)
	}

	// If we are not currently doing any deployment work, and there are events waiting, then send the next event to the runner
	if len(state.waitingDeploymentEvents) > 0 && state.activeDeploymentEvent == nil && !state.deploymentEventRunnerShutdown {

		state.activeDeploymentEvent = state.waitingDeploymentEvents[0]
		state.waitingDeploymentEvents = state.waitingDeploymentEvents[1:]

		// Send the work to the runner

		if !(state.activeDeploymentEvent.Message.Event.EventType == eventlooptypes.UpdateDeploymentStatusTick &&
			disableDeploymentStatusTickLogging == true) {
			log.V(logutil.LogLevel_Debug).Info("About to send work to depl event runner",
				"event", eventlooptypes.StringEventLoopEvent(state.activeDeploymentEvent.Message.Event))
		}

		state.deploymentEventRunner <- *state.activeDeploymentEvent.Message.Event

		if !(state.activeDeploymentEvent.Message.Event.EventType == eventlooptypes.UpdateDeploymentStatusTick &&
			disableDeploymentStatusTickLogging == true) {

			log.V(logutil.LogLevel_Debug).Info("Sent work to depl event runner",
				"event", eventlooptypes.StringEventLoopEvent(state.activeDeploymentEvent.Message.Event))

		}
	}

	// If we are not currently doing any sync operation work, and there are events waiting, then send the next event to the runner
	if len(state.waitingSyncOperationEvents) > 0 && state.activeSyncOperationEvent == nil && !state.syncOperationEventRunnerShutdown {

		state.activeSyncOperationEvent = state.waitingSyncOperationEvents[0]
		state.waitingSyncOperationEvents = state.waitingSyncOperationEvents[1:]

		// Send the work to the runner
		state.syncOperationEventRunner <- *state.activeSyncOperationEvent.Message.Event
		log.V(logutil.LogLevel_Debug).Info("Sent work to sync op runner",
			"event", eventlooptypes.StringEventLoopEvent(state.activeSyncOperationEvent.Message.Event))

	}

	// If the deployment runner has shutdown, and there are no active or waiting sync operation events,
	// then it is safe to shut down the sync runner too.
	if state.deploymentEventRunnerShutdown && len(state.waitingSyncOperationEvents) == 0 &&
		state.activeSyncOperationEvent == nil && !state.syncOperationEventRunnerShutdown {

		state.syncOperationEventRunnerShutdown = true
		if state.syncOperationEventRunnerShutdown {
			log.Info("Sync operation runner shutdown due to shutdown by deployment runner")
		}
	}

	return terminateEventLoop_false
}

// startNewStatusUpdateTimer will send a timer tick message to the application event loop in X seconds.
// This tick informs the runner that it needs to update the status field of the Deployment.
func startNewStatusUpdateTimer(ctx context.Context, k8sClient client.Client, input chan RequestMessage,
	log logr.Logger) {

	// Up to 1 second of jitter
	// #nosec
	jitter := time.Duration(int64(time.Millisecond) * int64(rand.Float64()*1000))

	statusUpdateTimer := time.NewTimer(deploymentStatusTickRate + jitter)

	go func() {

		<-statusUpdateTimer.C
		tickMessage := RequestMessage{
			Message: eventlooptypes.EventLoopMessage{
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.UpdateDeploymentStatusTick,
					Client:    k8sClient,
				},
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
			},
			ResponseChan: nil,
		}
		input <- tickMessage
	}()
}

// applicationEventRunnerFactory is used to start an application loop runner. It is a lightweight wrapper
// around the 'startNewApplicationEventLoopRunner' function.
//
// The defaultApplicationEventRunnerFactory should be used in all cases, except for when writing mocks for unit tests.
type applicationEventRunnerFactory interface {
	createNewApplicationEventLoopRunner(ctx context.Context, informWorkCompleteChan chan RequestMessage,
		sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop,
		gitopsDeplName string, gitopsDeplNamespace string, workspaceID string, debugContext string, k8sFactory shared_resource_loop.SRLK8sClientFactory) chan eventlooptypes.EventLoopEvent
}

type defaultApplicationEventRunnerFactory struct {
}

var _ applicationEventRunnerFactory = defaultApplicationEventRunnerFactory{}

// createNewApplicationEventLoopRunner is a simple wrapper around the default function.
func (defaultApplicationEventRunnerFactory) createNewApplicationEventLoopRunner(ctx context.Context, informWorkCompleteChan chan RequestMessage,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop,
	gitopsDeplName string, gitopsDeplNamespace string, workspaceID string, debugContext string, k8sFactory shared_resource_loop.SRLK8sClientFactory) chan eventlooptypes.EventLoopEvent {

	return startNewApplicationEventLoopRunner(ctx, informWorkCompleteChan, sharedResourceEventLoop, gitopsDeplName, gitopsDeplNamespace,
		workspaceID, debugContext, k8sFactory)
}

var _ shared_resource_loop.SRLK8sClientFactory = ExistingK8sClientFactory{}

// ExistingK8sClientFactory can be used when a caller already has a Service Workspace context they wish to use.
type ExistingK8sClientFactory struct {
	existingK8sClient client.Client
}

func (e ExistingK8sClientFactory) GetK8sClientForGitOpsEngineInstance(ctx context.Context, gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {
	return eventlooptypes.GetK8sClientForGitOpsEngineInstance(ctx, gitopsEngineInstance)
}

func (e ExistingK8sClientFactory) GetK8sClientForServiceWorkspace() (client.Client, error) {
	return e.existingK8sClient, nil
}

func (e ExistingK8sClientFactory) BuildK8sClient(restConfig *rest.Config) (client.Client, error) {
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	k8sClient = sharedutil.IfEnabledSimulateUnreliableClient(k8sClient)
	if err != nil {
		return nil, err
	}

	return k8sClient, err

}
