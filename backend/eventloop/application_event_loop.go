package eventloop

import (
	"context"
	"strings"
	"time"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/util"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type applicationEventLoop struct {
	input chan applicationEventLoopMessage
}

// TODO: DEBT - Set log to info, and make sure you can still figure out what's going on.

func startApplicationEventLoopRouter(input chan applicationEventLoopMessage, workspaceID string) {

	go func() {

		log := log.FromContext(context.Background())

		backoff := util.ExponentialBackoff{Min: time.Duration(500 * time.Millisecond), Max: time.Duration(15 * time.Second), Factor: 2, Jitter: true}

		lastFail := time.Now()

		for {
			isPanic, _ := sharedutil.CatchPanic(func() error {
				applicationEventLoopRouter(input, workspaceID)
				return nil
			})

			// This really shouldn't happen, so we log it as severe.
			log.Error(nil, "SEVERE: the applicationEventLoopRouter function exited unexpectedly.", "isPanic", isPanic)

			// If the applicationEventLoopRouter somehow stops (for example, due to panic), we should restart it.
			// - if we don't restart it, we will miss any events that occur.
			//
			// However, we don't want to KEEP restarting it if it will just keep panicing over and over again,
			// so we use an expotenial backoff to ensure that failures do not consume all our CPU resources.

			// If the last failure was > 2 minutes ago, then reset the backoff back to Min.
			if time.Now().After(lastFail.Add(5 * time.Minute)) {
				backoff.Reset()
			}

			lastFail = time.Now()

			// Wait a small amount of time before we restart the application event loop
			backoff.DelayOnFail(context.Background())
		}
	}()

}

const (
	// orphanedResourceGitopsDeplUID indicates that a GitOpsDeploymentSyncRunCR is orphaned, which means
	// we do not know which GitOpsDeployment it should belong to. This is usually because the deployment name
	// field of the SyncRun refers to a K8s resource that doesn't (or no longer) exists.
	orphanedResourceGitopsDeplUID = "orphaned"
)

// applicationEventLoopRouter receives all events for the workspace, and passes them to specific goroutine responsible
// for handling events for individual applications.
func applicationEventLoopRouter(input chan applicationEventLoopMessage, workspaceID string) {

	ctx := context.Background()

	log := log.FromContext(ctx).WithValues("workspaceID", workspaceID)

	log.Info("applicationEventLoopRouter started")
	defer log.Info("applicationEventLoopRouter ended.")

	sharedResourceEventLoop := newSharedResourceLoop()

	// orphanedResources: gitops depl name -> name field of CR -> event depending on it
	orphanedResources := map[string]map[string]eventLoopEvent{}

	// applicationMap: gitopsDepl UID -> channel for go routine responsible for handling it
	applicationMap := map[string]applicationEventLoop{}
	for {
		event := <-input

		// First, sanity check the event

		if event.messageType == applicationEventLoopMessageType_WorkComplete {
			log.Error(nil, "SEVERE: invalid message type received in applicationEventLooRouter")
			continue
		}

		if event.event == nil {
			log.Error(nil, "SEVERE: event was nil applicationEventLooRouter")
			continue
		}

		if strings.TrimSpace(event.event.associatedGitopsDeplUID) == "" {
			log.Error(nil, "SEVERE: event was nil applicationEventLooRouter")
			continue
		}

		log.V(sharedutil.LogLevel_Debug).Info("applicationEventLoop received event", "event", stringEventLoopEvent(event.event))

		// If the event is orphaned (it refers to a gitopsdepl that doesn't exist, add it to our orphaned resources list)
		if event.event.associatedGitopsDeplUID == orphanedResourceGitopsDeplUID {

			if event.event.reqResource == v1alpha1.GitOpsDeploymentSyncRunTypeName {
				syncRunCR := &v1alpha1.GitOpsDeploymentSyncRun{
					ObjectMeta: v1.ObjectMeta{
						Name:      event.event.request.Name,
						Namespace: event.event.request.Namespace,
					},
				}
				if err := event.event.client.Get(ctx, client.ObjectKeyFromObject(syncRunCR), syncRunCR); err != nil {

					if apierr.IsNotFound(err) {
						log.V(sharedutil.LogLevel_Debug).Info("skipping orphaned resource that could no longer be found:", "resource", syncRunCR.ObjectMeta)
						continue
					} else {
						log.Error(err, "unexpected client error on retrieving orphaned syncrun object", "resource", syncRunCR.ObjectMeta)
						continue
					}
				}

				// TODO: DEBT - Make sure it's not still orphaned before adding it to orphanedResources

				gitopsDeplMap, exists := orphanedResources[syncRunCR.Spec.GitopsDeploymentName]
				if !exists {
					gitopsDeplMap = map[string]eventLoopEvent{}
					orphanedResources[syncRunCR.Spec.GitopsDeploymentName] = gitopsDeplMap
				}

				log.V(sharedutil.LogLevel_Debug).Info("Adding syncrun CR to orphaned resources list, name: " + syncRunCR.Name + ", missing gitopsdepl name: " + syncRunCR.Spec.GitopsDeploymentName)
				gitopsDeplMap[syncRunCR.Name] = *event.event
			} else {
				log.Error(nil, "SEVERE: unexpected event resource type in applicationEventLoopRouter")
				continue
			}

			continue
		}

		if event.event.reqResource == v1alpha1.GitOpsDeploymentTypeName {

			// If there exists an orphaned resource that is waiting for this gitopsdepl
			if gitopsDeplMap, exists := orphanedResources[event.event.request.Name]; exists {

				// Retrieve the gitopsdepl
				gitopsDeplCR := &v1alpha1.GitOpsDeployment{
					ObjectMeta: v1.ObjectMeta{
						Name:      event.event.request.Name,
						Namespace: event.event.request.Namespace,
					},
				}
				if err := event.event.client.Get(ctx, client.ObjectKeyFromObject(gitopsDeplCR), gitopsDeplCR); err != nil {
					// log as warning, but continue.
					log.V(sharedutil.LogLevel_Warn).Error(err, "unexpected client error on retrieving gitopsdepl")

				} else {
					// Copy the events to a new slice, and remove the events from the orphanedResourced map
					requeueEvents := []applicationEventLoopMessage{}
					for _, orphanedResourceEvent := range gitopsDeplMap {

						// Unorphan the resource
						orphanedResourceEvent.associatedGitopsDeplUID = string(gitopsDeplCR.UID)

						requeueEvents = append(requeueEvents, applicationEventLoopMessage{
							messageType: applicationEventLoopMessageType_Event,
							event:       &orphanedResourceEvent,
						})

						log.V(sharedutil.LogLevel_Debug).Info("found parent: " + gitopsDeplCR.Name + " (" + string(gitopsDeplCR.UID) + "), of orphaned resource: " + orphanedResourceEvent.request.Name)
					}
					delete(orphanedResources, event.event.request.Name)

					// Requeue the orphaned events from a separate goroutine (to prevent us from blocking this goroutine).
					// The orphaned events will be reprocessed after the gitopsdepl is processed.
					go func() {
						for _, eventToRequeue := range requeueEvents {
							log.V(sharedutil.LogLevel_Debug).Info("requeueing orphaned resource: " + eventToRequeue.event.request.Name + ", for parent: " + eventToRequeue.event.associatedGitopsDeplUID)
							input <- eventToRequeue
						}
					}()
				}
			}
		}

		applicationEntryVal, ok := applicationMap[event.event.associatedGitopsDeplUID]
		if !ok {
			// Start the application event queue go-routine, if it's not already started.
			applicationEntryVal = applicationEventLoop{
				input: make(chan applicationEventLoopMessage),
			}
			applicationMap[event.event.associatedGitopsDeplUID] = applicationEntryVal

			go applicationEventQueueLoop(applicationEntryVal.input, event.event.associatedGitopsDeplUID, event.event.workspaceID, sharedResourceEventLoop)
		}

		// Send the event to the channel/go routine that handles all events for this application/gitopsdepl (non-blocking)
		applicationEntryVal.input <- applicationEventLoopMessage{
			messageType: applicationEventLoopMessageType_Event,
			event:       event.event,
		}
	}
}

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

	// Maintain a list of workspace events we have received from Reconcile(), but not yet processed.
	for {

		// If both the runner signal that they have shutdown, then
		if deploymentEventRunnerShutdown && syncOperationEventRunnerShutdown {
			log.V(sharedutil.LogLevel_Debug).Info("orderly termination of deployment and sync runners.")
			break
		}

		// TODO: DEBT - Verify that everything we receive is directly or indirectly related to this gitopsDeplId

		// Block on waiting for more events for this workspace
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

		// If we've received a new event from the application event loop
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
