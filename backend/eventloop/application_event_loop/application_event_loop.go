package application_event_loop

import (
	"context"
	"math/rand"
	"time"

	"github.com/go-logr/logr"
	"github.com/kcp-dev/logicalcluster/v2"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

var (
	// deploymentStatusTickRate is the rate at which the application event runner will be sent a message
	// indicating that the GitOpsDeployment status field should be updated.
	deploymentStatusTickRate = 15 * time.Second
)

func StartApplicationEventQueueLoop(ctx context.Context, gitopsDeploymentName string, gitopsDeploymentNamespace string, workspaceID string,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop) chan eventlooptypes.EventLoopMessage {

	res := make(chan eventlooptypes.EventLoopMessage)

	go applicationEventQueueLoop(ctx, res, gitopsDeploymentName, gitopsDeploymentNamespace, workspaceID, sharedResourceEventLoop, defaultApplicationEventRunnerFactory{})

	return res
}

func startApplicationEventQueueLoopWithFactory(ctx context.Context, gitopsDeploymentName string, gitopsDeploymentNamespace string, workspaceID string,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop, aerFactory applicationEventRunnerFactory) chan eventlooptypes.EventLoopMessage {

	res := make(chan eventlooptypes.EventLoopMessage)

	go applicationEventQueueLoop(ctx, res, gitopsDeploymentName, gitopsDeploymentNamespace, workspaceID, sharedResourceEventLoop, aerFactory)

	return res
}

func applicationEventQueueLoop(ctx context.Context, input chan eventlooptypes.EventLoopMessage, gitopsDeploymentName string, gitopsDeploymentNamespace string,
	workspaceID string, sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop, aerFactory applicationEventRunnerFactory) {

	log := log.FromContext(ctx).WithValues("workspaceID", workspaceID, "gitOpsDeplName", gitopsDeploymentName,
		"gitopsDeplNamespace", gitopsDeploymentNamespace).WithName("application-event-loop")

	log.Info("applicationEventQueueLoop started.")
	defer log.Info("applicationEventQueueLoop ended.")

	// Only one deployment event is processed at a time
	// These are events that are waiting to be sent to application_event_runner_deployments runner
	var activeDeploymentEvent *eventlooptypes.EventLoopEvent
	waitingDeploymentEvents := []*eventlooptypes.EventLoopEvent{}

	// Only one sync operation event is processed at a time
	// For example: if the user created multiple GitOpsDeploymentSyncRun CRs, they will be processed to completion, one at a time.
	// These are events that are waiting to be sent to application_event_runner_syncruns runner
	var activeSyncOperationEvent *eventlooptypes.EventLoopEvent
	waitingSyncOperationEvents := []*eventlooptypes.EventLoopEvent{}

	deploymentEventRunner := aerFactory.createNewApplicationEventLoopRunner(input, sharedResourceEventLoop, gitopsDeploymentName,
		gitopsDeploymentNamespace, workspaceID, "deployment")
	deploymentEventRunnerShutdown := false

	syncOperationEventRunner := aerFactory.createNewApplicationEventLoopRunner(input, sharedResourceEventLoop, gitopsDeploymentName,
		gitopsDeploymentNamespace, workspaceID, "sync-operation")
	syncOperationEventRunnerShutdown := false

	// Start the ticker, which will -- every X seconds -- instruct the GitOpsDeployment CR fields to update
	startNewStatusUpdateTimer(ctx, input, log)

	for {

		// If both the runners signal that they have shutdown, then exit
		if deploymentEventRunnerShutdown && syncOperationEventRunnerShutdown {
			log.V(sharedutil.LogLevel_Debug).Info("orderly termination of deployment and sync runners.")
			break
		}

		// Block on waiting for more events for this application
		newEvent := <-input

		if newEvent.Event == nil {
			log.Error(nil, "SEVERE: applicationEventQueueLoop event was nil", "messageType", newEvent.MessageType)
			continue
		}

		// If we've received a new event from the workspace event loop
		if newEvent.MessageType == eventlooptypes.ApplicationEventLoopMessageType_Event {

			log := log.WithValues("event", eventlooptypes.StringEventLoopEvent(newEvent.Event))

			log.V(sharedutil.LogLevel_Debug).Info("applicationEventQueueLoop received event")

			if newEvent.Event.ReqResource == eventlooptypes.GitOpsDeploymentTypeName {

				if !deploymentEventRunnerShutdown {
					waitingDeploymentEvents = append(waitingDeploymentEvents, newEvent.Event)
				} else {
					log.V(sharedutil.LogLevel_Debug).Info("Ignoring post-shutdown deployment event")
				}

			} else if newEvent.Event.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {

				if !syncOperationEventRunnerShutdown {
					waitingSyncOperationEvents = append(waitingSyncOperationEvents, newEvent.Event)
				} else {
					log.V(sharedutil.LogLevel_Debug).Info("Ignoring post-shutdown sync operation event")
				}
			} else if newEvent.Event.ReqResource == eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName {

				if !deploymentEventRunnerShutdown {
					waitingDeploymentEvents = append(waitingDeploymentEvents, newEvent.Event)
				} else {
					log.V(sharedutil.LogLevel_Debug).Info("Ignoring post-shutdown managed environment event")
				}

			} else if newEvent.Event.EventType == eventlooptypes.UpdateDeploymentStatusTick {

				if !deploymentEventRunnerShutdown {
					waitingDeploymentEvents = append(waitingDeploymentEvents, newEvent.Event)
				} else {
					log.V(sharedutil.LogLevel_Debug).Info("Ignoring post-shutdown deployment event")
				}

			} else {
				log.Error(nil, "SEVERE: unexpected event resource type in applicationEventQueueLoop")
			}

			// If we've received a work complete notification...
		} else if newEvent.MessageType == eventlooptypes.ApplicationEventLoopMessageType_WorkComplete {

			log := log.WithValues("event", eventlooptypes.StringEventLoopEvent(newEvent.Event))

			log.V(sharedutil.LogLevel_Debug).Info("applicationEventQueueLoop received work complete event")

			if newEvent.Event.EventType == eventlooptypes.UpdateDeploymentStatusTick {
				// After we finish processing a previous status tick, start the timer to queue up a new one.
				// This ensures we are always reminded to do a status update.
				activeDeploymentEvent = nil
				startNewStatusUpdateTimer(ctx, input, log)

			} else if newEvent.Event.ReqResource == eventlooptypes.GitOpsDeploymentTypeName ||
				newEvent.Event.ReqResource == eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName {

				if activeDeploymentEvent != newEvent.Event {
					log.Error(nil, "SEVERE: unmatched deployment event work item", "activeDeploymentEvent", eventlooptypes.StringEventLoopEvent(activeDeploymentEvent))
				}
				activeDeploymentEvent = nil

				deploymentEventRunnerShutdown = newEvent.ShutdownSignalled
				if deploymentEventRunnerShutdown {
					log.Info("Deployment signalled shutdown")
				}

			} else if newEvent.Event.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {

				if newEvent.Event != activeSyncOperationEvent {
					log.Error(nil, "SEVERE: unmatched sync operation event work item", "activeSyncOperationEvent", eventlooptypes.StringEventLoopEvent(activeSyncOperationEvent))
				}
				activeSyncOperationEvent = nil
				syncOperationEventRunnerShutdown = newEvent.ShutdownSignalled
				if syncOperationEventRunnerShutdown {
					log.Info("Sync operation signalled shutdown")
				}

			} else {
				log.Error(nil, "SEVERE: unexpected event resource type in applicationEventQueueLoop")
			}

		} else {
			log.Error(nil, "SEVERE: Unrecognized workspace event type", "type", newEvent.MessageType)
		}

		// If we are not currently doing any deployment work, and there are events waiting, then send the next event to the runner
		if len(waitingDeploymentEvents) > 0 && activeDeploymentEvent == nil && !deploymentEventRunnerShutdown {

			activeDeploymentEvent = waitingDeploymentEvents[0]
			waitingDeploymentEvents = waitingDeploymentEvents[1:]

			// Send the work to the runner
			log.V(sharedutil.LogLevel_Debug).Info("About to send work to depl event runner", "event", eventlooptypes.StringEventLoopEvent(activeDeploymentEvent))
			deploymentEventRunner <- activeDeploymentEvent
			log.V(sharedutil.LogLevel_Debug).Info("Sent work to depl event runner", "event", eventlooptypes.StringEventLoopEvent(activeDeploymentEvent))

		}

		// If we are not currently doing any sync operation work, and there are events waiting, then send the next event to the runner
		if len(waitingSyncOperationEvents) > 0 && activeSyncOperationEvent == nil && !syncOperationEventRunnerShutdown {

			activeSyncOperationEvent = waitingSyncOperationEvents[0]
			waitingSyncOperationEvents = waitingSyncOperationEvents[1:]

			// Send the work to the runner
			syncOperationEventRunner <- activeSyncOperationEvent
			log.V(sharedutil.LogLevel_Debug).Info("Sent work to sync op runner", "event", eventlooptypes.StringEventLoopEvent(activeSyncOperationEvent))

		}

		// If the deployment runner has shutdown, and there are no active or waiting sync operation events,
		// then it is safe to shut down the sync runner too.
		if deploymentEventRunnerShutdown && len(waitingSyncOperationEvents) == 0 &&
			activeSyncOperationEvent == nil && !syncOperationEventRunnerShutdown {

			syncOperationEventRunnerShutdown = true
			if syncOperationEventRunnerShutdown {
				log.Info("Sync operation runner shutdown due to shutdown by deployment runner")
			}
		}
	}

}

// startNewStatusUpdateTimer will send a timer tick message to the application event loop in X seconds.
// This tick informs the runner that it needs to update the status field of the Deployment.
func startNewStatusUpdateTimer(ctx context.Context, input chan eventlooptypes.EventLoopMessage, log logr.Logger) {

	// Up to 1 second of jitter
	// #nosec
	jitter := time.Duration(int64(time.Millisecond) * int64(rand.Float64()*1000))

	statusUpdateTimer := time.NewTimer(deploymentStatusTickRate + jitter)

	go func() {
		var workspaceClient client.Client
		var err error

		// Keep trying to create k8s client, until we succeed
		backoff := sharedutil.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true}
		for {
			workspaceClient, err = getk8sClient()
			if err == nil {
				break
			} else {
				backoff.DelayOnFail(ctx)
			}

			// Exit if the context is cancelled
			select {
			case <-ctx.Done():
				log.V(sharedutil.LogLevel_Debug).Info("Deployment status ticker cancelled")
				return
			default:
			}
		}

		clusterName, _ := logicalcluster.ClusterFromContext(ctx)

		<-statusUpdateTimer.C
		tickMessage := eventlooptypes.EventLoopMessage{
			Event: &eventlooptypes.EventLoopEvent{
				EventType: eventlooptypes.UpdateDeploymentStatusTick,
				Request: reconcile.Request{
					ClusterName: clusterName.String(),
				},
				Client: workspaceClient,
			},
			MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
		}
		input <- tickMessage
	}()
}

func getk8sClient() (client.Client, error) {
	if sharedutil.IsRunningAgainstKCP() && !sharedutil.IsKCPVirtualWorkspaceDisabled() {
		return sharedutil.NewVirtualWorkspaceClient()
	}

	return getK8sClientForWorkspace()
}

// applicationEventRunnerFactory is used to start an application loop runner. It is a lightweight wrapper
// around the 'startNewApplicationEventLoopRunner' function.
//
// The defaultApplicationEventRunnerFactory should be used in all cases, except for when writing mocks for unit tests.
type applicationEventRunnerFactory interface {
	createNewApplicationEventLoopRunner(informWorkCompleteChan chan eventlooptypes.EventLoopMessage,
		sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop,
		gitopsDeplName string, gitopsDeplNamespace string, workspaceID string, debugContext string) chan *eventlooptypes.EventLoopEvent
}

type defaultApplicationEventRunnerFactory struct {
}

var _ applicationEventRunnerFactory = defaultApplicationEventRunnerFactory{}

// createNewApplicationEventLoopRunner is a simple wrapper around the default function.
func (defaultApplicationEventRunnerFactory) createNewApplicationEventLoopRunner(informWorkCompleteChan chan eventlooptypes.EventLoopMessage,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop,
	gitopsDeplName string, gitopsDeplNamespace string, workspaceID string, debugContext string) chan *eventlooptypes.EventLoopEvent {

	return startNewApplicationEventLoopRunner(informWorkCompleteChan, sharedResourceEventLoop, gitopsDeplName, gitopsDeplNamespace,
		workspaceID, debugContext)
}

func getK8sClientForWorkspace() (client.Client, error) {

	config, err := sharedutil.GetRESTConfig()
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	err = managedgitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = managedgitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return k8sClient, nil

}
