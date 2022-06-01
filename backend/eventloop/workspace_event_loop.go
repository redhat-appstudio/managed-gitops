package eventloop

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/application_event_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// All events that occur within an individual API namespace will be received by an instance of a Workspace Event loop
// (running with a Goroutine).

// The responsibility of the workspace event loop is to:
// - Receive events for a API namespace workspace, from the controller event loop
// - Pass received events to the appropriate Application Event Loop, for the particular GitOpsDeployment/GitOpsDeploymentSync.
// - Ensure a new Application Event Loop goroutine is running for each active application (see below).
// - Look for orphaned resources that become un-orphaned. (eg A GitOpsDeploymentSyncRun is orphaned because it
//   pointed to a non-existent GitOpsDeployment, but then the GitOpsDeployment it referenced was created.)
// - If this happens, the newly un-orphaned resource is requeued, so that it can finally be processed.

// Cardinality: 1 instance of the Workspace Event Loop goroutine exists for each workspace that contains
// GitOps service API resources. (For example, if there are 10 workspaces, there will be 10 Workspace
// Event Loop instances/goroutines servicing those).

// Start a workspace event loop router go routine, which is responsible for handling API namespace events and
// then passing them to the controller loop.
func startWorkspaceEventLoopRouter(workspaceID string) chan eventlooptypes.EventLoopMessage {

	res := make(chan eventlooptypes.EventLoopMessage)

	internalStartWorkspaceEventLoopRouter(res, workspaceID, defaultApplicationEventLoopFactory{})

	return res
}

// internalStartWorkspaceEventLoopRouter has the primary goal of catching panics from the workspaceEventLoopRouter, and recovering from them.
func internalStartWorkspaceEventLoopRouter(input chan eventlooptypes.EventLoopMessage, workspaceID string,
	applEventLoopFactory applicationEventQueueLoopFactory) {

	go func() {

		log := log.FromContext(context.Background())

		backoff := sharedutil.ExponentialBackoff{Min: time.Duration(500 * time.Millisecond), Max: time.Duration(15 * time.Second), Factor: 2, Jitter: true}

		lastFail := time.Now()

		for {
			isPanic, _ := sharedutil.CatchPanic(func() error {
				workspaceEventLoopRouter(input, workspaceID, applEventLoopFactory)
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
	// See https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.8tiycl1h7rns for details.
	orphanedResourceGitopsDeplUID = "orphaned"

	// noAssociatedGitOpsDeploymentUID: if a resource does not have an orphanedResourceDeplUID, this constant should be set.
	// For example: GitOpsDeploymentRepositoryCredentials might be associated with multiple (or zero) GitOpsDeployments.
	noAssociatedGitOpsDeploymentUID = "none"
)

// workspaceEventLoopRouter receives all events for the workspace, and passes them to specific goroutine responsible
// for handling events for individual applications.
func workspaceEventLoopRouter(input chan eventlooptypes.EventLoopMessage, workspaceID string,
	applEventLoopFactory applicationEventQueueLoopFactory) {

	workspaceResourceLoop := newWorkspaceResourceLoop()

	ctx := context.Background()

	log := log.FromContext(ctx).WithValues("workspaceID", workspaceID)

	log.Info("workspaceEventLoopRouter started")
	defer log.Info("workspaceEventLoopRouter ended.")

	sharedResourceEventLoop := shared_resource_loop.NewSharedResourceLoop()

	// orphanedResources: gitops depl name -> (name field of CR -> event depending on it)
	orphanedResources := map[string]map[string]eventlooptypes.EventLoopEvent{}

	// applicationMap: gitopsDepl UID -> channel for go routine responsible for handling it
	applicationMap := map[string]workspaceEventLoop_applicationEventLoopEntry{}
	for {
		event := <-input

		// First, sanity check the event
		if event.MessageType == eventlooptypes.ApplicationEventLoopMessageType_WorkComplete {
			log.Error(nil, "SEVERE: invalid message type received in applicationEventLooRouter")
			continue
		}
		if event.Event == nil {
			log.Error(nil, "SEVERE: event was nil in workspaceEventLooRouter")
			continue
		}
		if strings.TrimSpace(event.Event.AssociatedGitopsDeplUID) == "" {
			log.Error(nil, "SEVERE: event was nil in workspaceEventLoopRouter")
			continue
		}

		log.V(sharedutil.LogLevel_Debug).Info("workspaceEventLoop received event", "event", eventlooptypes.StringEventLoopEvent(event.Event))

		// If the event is orphaned (it refers to a gitopsdepl that doesn't exist, add it to our orphaned resources list)
		if event.Event.AssociatedGitopsDeplUID == orphanedResourceGitopsDeplUID {
			handleOrphaned(ctx, event, orphanedResources, log)
			continue
		}

		// Check GitOpsDeployment that come in: if we get an event for GitOpsDeployment that has an orphaned child resource
		// depending on it, then unorphan the child resource.
		if event.Event.ReqResource == v1alpha1.GitOpsDeploymentTypeName {
			unorphanResourcesIfPossible(ctx, event, orphanedResources, input, log)
		}

		// Handle Repository Credentials
		if event.Event.ReqResource == v1alpha1.GitOpsDeploymentRepositoryCredentialTypeName {

			workspaceResourceLoop.processRepositoryCredential(ctx, event.Event.Request, event.Event.Client)
			continue
		}

		applicationEntryVal, ok := applicationMap[event.Event.AssociatedGitopsDeplUID]
		if !ok {
			// Start the application event queue go-routine, if it's not already started.

			// Start the application event loop's goroutine
			inputChan := applEventLoopFactory.startApplicationEventQueueLoop(event.Event.AssociatedGitopsDeplUID,
				event.Event.WorkspaceID, sharedResourceEventLoop)

			applicationEntryVal = workspaceEventLoop_applicationEventLoopEntry{
				input: inputChan,
			}
			applicationMap[event.Event.AssociatedGitopsDeplUID] = applicationEntryVal
		}

		// Send the event to the channel/go routine that handles all events for this application/gitopsdepl (non-blocking)
		applicationEntryVal.input <- eventlooptypes.EventLoopMessage{
			MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
			Event:       event.Event,
		}
	}
}

type workspaceEventLoop_applicationEventLoopEntry struct {
	// input is the channel used to communicate with an application event loop goroutine.
	input chan eventlooptypes.EventLoopMessage
}

// applicationEventQueueLoopFactory is used to start the application event queue. It is a lightweight wrapper
// around the 'application_event_loop.StartApplicationEventQueueLoop' function.
//
// The defaultApplicationEventLoopFactory should be used in all cases, except for when writing mocks for unit tests.
type applicationEventQueueLoopFactory interface {
	startApplicationEventQueueLoop(gitopsDeplID string, workspaceID string,
		sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop) chan eventlooptypes.EventLoopMessage
}

type defaultApplicationEventLoopFactory struct {
}

// The default implementation of startApplicationEventQueueLoop is just a simple wrapper around a call to
// StartApplicationEventQueueLoop
func (defaultApplicationEventLoopFactory) startApplicationEventQueueLoop(gitopsDeplID string, workspaceID string,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop) chan eventlooptypes.EventLoopMessage {

	res := application_event_loop.StartApplicationEventQueueLoop(gitopsDeplID, workspaceID, sharedResourceEventLoop)

	return res
}

var _ applicationEventQueueLoopFactory = defaultApplicationEventLoopFactory{}

// Add a resource to orphaned list (why? because it is a gitopsdeplsyncrun that refers to a gitopsdepl that doesn't exist)
// See https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.8tiycl1h7rns for details.
func handleOrphaned(ctx context.Context, event eventlooptypes.EventLoopMessage, orphanedResources map[string]map[string]eventlooptypes.EventLoopEvent, log logr.Logger) {

	if event.Event.ReqResource == v1alpha1.GitOpsDeploymentSyncRunTypeName {

		syncRunCR := &v1alpha1.GitOpsDeploymentSyncRun{
			ObjectMeta: v1.ObjectMeta{
				Name:      event.Event.Request.Name,
				Namespace: event.Event.Request.Namespace,
			},
		}
		if err := event.Event.Client.Get(ctx, client.ObjectKeyFromObject(syncRunCR), syncRunCR); err != nil {
			if apierr.IsNotFound(err) {
				log.V(sharedutil.LogLevel_Debug).Info("skipping orphaned resource that could no longer be found:", "resource", syncRunCR.ObjectMeta)
				return
			} else {
				log.Error(err, "unexpected client error on retrieving orphaned syncrun object", "resource", syncRunCR.ObjectMeta)
				return
			}
		}

		// TODO: GITOPSRVCE-67 - DEBT - Make sure it's not still orphaned before adding it to orphanedResources

		gitopsDeplMap, exists := orphanedResources[syncRunCR.Spec.GitopsDeploymentName]
		if !exists {
			gitopsDeplMap = map[string]eventlooptypes.EventLoopEvent{}
			orphanedResources[syncRunCR.Spec.GitopsDeploymentName] = gitopsDeplMap
		}

		log.V(sharedutil.LogLevel_Debug).Info("Adding syncrun CR to orphaned resources list, name: " + syncRunCR.Name + ", missing gitopsdepl name: " + syncRunCR.Spec.GitopsDeploymentName)
		gitopsDeplMap[syncRunCR.Name] = *event.Event

	} else {
		log.Error(nil, "SEVERE: unexpected event resource type in handleOrphaned")
		return
	}

}

func unorphanResourcesIfPossible(ctx context.Context, event eventlooptypes.EventLoopMessage, orphanedResources map[string]map[string]eventlooptypes.EventLoopEvent,
	input chan eventlooptypes.EventLoopMessage, log logr.Logger) {

	// If there exists an orphaned resource that is waiting for this gitopsdepl (by name)
	if gitopsDeplMap, exists := orphanedResources[event.Event.Request.Name]; exists {

		// Retrieve the gitopsdepl
		gitopsDeplCR := &v1alpha1.GitOpsDeployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      event.Event.Request.Name,
				Namespace: event.Event.Request.Namespace,
			},
		}
		if err := event.Event.Client.Get(ctx, client.ObjectKeyFromObject(gitopsDeplCR), gitopsDeplCR); err != nil {
			// log as warning, but continue.
			log.V(sharedutil.LogLevel_Warn).Error(err, "unexpected client error on retrieving gitopsdepl")

		} else {
			// Copy the events to a new slice, and remove the events from the orphanedResourced map
			requeueEvents := []eventlooptypes.EventLoopMessage{}
			for index := range gitopsDeplMap {

				orphanedResourceEvent := gitopsDeplMap[index]

				// Unorphan the resource
				orphanedResourceEvent.AssociatedGitopsDeplUID = string(gitopsDeplCR.UID)

				requeueEvents = append(requeueEvents, eventlooptypes.EventLoopMessage{
					MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event:       &orphanedResourceEvent,
				})

				log.V(sharedutil.LogLevel_Debug).Info("found parent: " + gitopsDeplCR.Name + " (" + string(gitopsDeplCR.UID) + "), of orphaned resource: " + orphanedResourceEvent.Request.Name)
			}
			delete(orphanedResources, event.Event.Request.Name)

			// Requeue the orphaned events from a separate goroutine (to prevent us from blocking this goroutine).
			// The orphaned events will be reprocessed after the gitopsdepl is processed.
			go func() {
				for _, eventToRequeue := range requeueEvents {
					log.V(sharedutil.LogLevel_Debug).Info("requeueing orphaned resource: " + eventToRequeue.Event.Request.Name + ", for parent: " + eventToRequeue.Event.AssociatedGitopsDeplUID)
					input <- eventToRequeue
				}
			}()
		}
	}
}
