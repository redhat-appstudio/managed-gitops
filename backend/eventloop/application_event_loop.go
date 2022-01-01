package eventloop

import (
	"context"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func applicationEventQueueLoop(input chan applicationEventLoopMessage, gitopsDeplID string, workspaceID string, sharedResourceEventLoop *sharedResourceEventLoop) {

	log := log.FromContext(context.Background()).WithValues("workspaceID", workspaceID).WithValues("gitOpsDeplID", gitopsDeplID)

	log.Info("applicationEventQueueLoop started. gitopsDeplID: " + gitopsDeplID + ". workspaceID: " + workspaceID)
	defer log.Info("applicationEventQueueLoop ended. gitopsDeplID: " + gitopsDeplID + ". workspaceID: " + workspaceID)

	var activeDeploymentEvent *eventLoopEvent
	waitingDeploymentEvents := []*eventLoopEvent{}

	var activeSyncOperationEvent *eventLoopEvent
	waitingSyncOperationEvents := []*eventLoopEvent{}

	deploymentEventRunner := newApplicationEventLoopRunner(input, sharedResourceEventLoop, gitopsDeplID, workspaceID)
	deploymentEventRunnerShutdown := false

	syncOperationEventRunner := newApplicationEventLoopRunner(input, sharedResourceEventLoop, gitopsDeplID, workspaceID)
	syncOperationEventRunnerShutdown := false

	// TODO: GITOPS-1636 - Update GitOpsDeployment status field based on Application database entry's health/status

	for {

		// If both the runner signal that they have shutdown, then
		if deploymentEventRunnerShutdown && syncOperationEventRunnerShutdown {
			log.V(sharedutil.LogLevel_Debug).Info("orderly termination of deployment and sync runners.")
			break
		}

		// TODO: DEBT - Verify that everything we receive is directly or indirectly related to this gitopsDeplId

		// Block on waiting for more events for this application
		newEvent := <-input

		// Sanity checks
		if newEvent.event == nil {
			log.Error(nil, "SEVERE: applicationEventQueueLoop event was nil", "messageType", newEvent.messageType)
			continue
		}
		if newEvent.event.associatedGitopsDeplUID != gitopsDeplID {
			log.Error(nil, "SEVERE: gitopsdepluid associated with event had a different value than the application event queue loop gitopsdepl", "event-gitopsdepl-uid", newEvent.event.associatedGitopsDeplUID, "application-event-loop-gitopsdepl-", gitopsDeplID)
			continue
		}

		// If we've received a new event from the workspace event loop
		if newEvent.messageType == applicationEventLoopMessageType_Event {

			log := log.WithValues("event", stringEventLoopEvent(newEvent.event))

			log.V(sharedutil.LogLevel_Debug).Info("applicationEventQueueLoop received event")

			if newEvent.event.reqResource == v1alpha1.GitOpsDeploymentTypeName {

				if !deploymentEventRunnerShutdown {
					waitingDeploymentEvents = append(waitingDeploymentEvents, newEvent.event)
				} else {
					log.V(sharedutil.LogLevel_Debug).Info("Ignoring post-shutdown deployment event")
				}

			} else if newEvent.event.reqResource == v1alpha1.GitOpsDeploymentSyncRunTypeName {

				if !syncOperationEventRunnerShutdown {
					waitingSyncOperationEvents = append(waitingSyncOperationEvents, newEvent.event)
				} else {
					log.V(sharedutil.LogLevel_Debug).Info("Ignoring post-shutdown sync operation event")
				}

			} else {
				log.Error(nil, "SEVERE: unexpected event resource type in applicationEventQueueLoop")
				continue
			}

			// If we've received a work complete notification...
		} else if newEvent.messageType == applicationEventLoopMessageType_WorkComplete {

			log := log.WithValues("event", stringEventLoopEvent(newEvent.event))

			log.V(sharedutil.LogLevel_Debug).Info("applicationEventQueueLoop received work complete event")

			if newEvent.event.reqResource == v1alpha1.GitOpsDeploymentTypeName {

				if activeDeploymentEvent != newEvent.event {
					log.Error(nil, "SEVERE: unmatched deployment event work item", "activeDeploymentEvent", stringEventLoopEvent(activeDeploymentEvent))
				}
				activeDeploymentEvent = nil

				deploymentEventRunnerShutdown = newEvent.shutdownSignalled
				if deploymentEventRunnerShutdown {
					log.Info("Deployment signalled shutdown")
				}

			} else if newEvent.event.reqResource == v1alpha1.GitOpsDeploymentSyncRunTypeName {

				if newEvent.event != activeSyncOperationEvent {
					log.Error(nil, "SEVERE: unmatched sync operation event work item", "activeSyncOperationEvent", stringEventLoopEvent(activeSyncOperationEvent))
				}
				activeSyncOperationEvent = nil
				syncOperationEventRunnerShutdown = newEvent.shutdownSignalled
				if syncOperationEventRunnerShutdown {
					log.Info("Sync operation signalled shutdown")
				}

			} else {
				log.Error(nil, "SEVERE: unexpected event resource type in applicationEventQueueLoop")
				continue
			}

		} else {
			log.Error(nil, "SEVERE: Unrecognized workspace event type", "type", newEvent.messageType)
			continue
		}

		if len(waitingDeploymentEvents) > 0 && activeDeploymentEvent == nil && !deploymentEventRunnerShutdown {

			activeDeploymentEvent = waitingDeploymentEvents[0]
			waitingDeploymentEvents = waitingDeploymentEvents[1:]

			// Send the work to the runner
			deploymentEventRunner <- activeDeploymentEvent
			log.V(sharedutil.LogLevel_Debug).Info("Sent work to depl event runner", "event", stringEventLoopEvent(activeDeploymentEvent))

		}

		if len(waitingSyncOperationEvents) > 0 && activeSyncOperationEvent == nil && !syncOperationEventRunnerShutdown {

			activeSyncOperationEvent = waitingSyncOperationEvents[0]
			waitingSyncOperationEvents = waitingSyncOperationEvents[1:]

			// Send the work to the runner
			syncOperationEventRunner <- activeSyncOperationEvent
			log.V(sharedutil.LogLevel_Debug).Info("Sent work to sync op runner", "event", stringEventLoopEvent(activeSyncOperationEvent))

		}
	}

}
