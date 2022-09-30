package eventloop

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/application_event_loop"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
func newWorkspaceEventLoopRouter(workspaceID string) WorkspaceEventLoopRouterStruct {

	res := WorkspaceEventLoopRouterStruct{
		channel: make(chan workspaceEventLoopMessage),
	}

	internalStartWorkspaceEventLoopRouter(res.channel, workspaceID, defaultApplicationEventLoopFactory{})

	return res
}

func newWorkspaceEventLoopRouterWithFactory(workspaceID string, applEventLoopFactory applicationEventQueueLoopFactory) WorkspaceEventLoopRouterStruct {

	res := WorkspaceEventLoopRouterStruct{
		channel: make(chan workspaceEventLoopMessage),
	}

	internalStartWorkspaceEventLoopRouter(res.channel, workspaceID, applEventLoopFactory)

	return res
}

func (welrs *WorkspaceEventLoopRouterStruct) SendMessage(msg eventlooptypes.EventLoopMessage) {

	welrs.channel <- workspaceEventLoopMessage{
		messageType: workspaceEventLoopMessageType_Event,
		payload:     msg,
	}
}

type WorkspaceEventLoopRouterStruct struct {
	// channel chan eventlooptypes.EventLoopMessage
	channel chan workspaceEventLoopMessage
}

type workspaceEventLoopMessageType string

const (
	workspaceEventLoopMessageType_Event workspaceEventLoopMessageType = "event"
	managedEnvProcessed_Event           workspaceEventLoopMessageType = "managedEnvProcessed"
)

type workspaceEventLoopMessage struct {
	messageType workspaceEventLoopMessageType
	payload     any
}

// internalStartWorkspaceEventLoopRouter has the primary goal of catching panics from the workspaceEventLoopRouter, and
// recovering from them.
func internalStartWorkspaceEventLoopRouter(input chan workspaceEventLoopMessage, workspaceID string,
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

// workspaceEventLoopRouter receives all events for the namespace, and passes them to specific goroutine responsible
// for handling events for individual applications.
func workspaceEventLoopRouter(input chan workspaceEventLoopMessage, namespaceID string,
	applEventLoopFactory applicationEventQueueLoopFactory) {

	ctx := context.Background()

	log := log.FromContext(ctx).WithValues("namespaceID", namespaceID).WithName("workspace-event-loop")

	log.Info("workspaceEventLoopRouter started")
	defer log.Info("workspaceEventLoopRouter ended.")

	sharedResourceEventLoop := shared_resource_loop.NewSharedResourceLoop()

	workspaceResourceLoop := newWorkspaceResourceLoop(sharedResourceEventLoop, input)

	// orphanedResources:
	// - key: gitops depl name
	// - value: map: name field of orphaned syncrun CR -> last event that referenced it
	orphanedResources := map[string]map[string]eventlooptypes.EventLoopEvent{}

	// applicationMap: maintains a list of which which GitOpsDeployment resources (1-1 relationship) are handled by which Go Routines
	// - key: string, of format "(namespace UID)-(namespace name)-(GitOpsDeployment name)"
	// - value:  channel for the go routine responsible for handling events for this GitOpsDeployment
	applicationMap := map[string]workspaceEventLoop_applicationEventLoopEntry{}
	for {
		wrapperEvent := <-input

		event := (wrapperEvent.payload).(eventlooptypes.EventLoopMessage)
		ctx = sharedutil.AddKCPClusterToContext(ctx, event.Event.Request.ClusterName)

		if wrapperEvent.messageType == workspaceEventLoopMessageType_Event {
			// First, sanity check the event
			if event.MessageType == eventlooptypes.ApplicationEventLoopMessageType_WorkComplete {
				log.Error(nil, "SEVERE: invalid message type received in applicationEventLooRouter")
				continue
			}
			if event.Event == nil {
				log.Error(nil, "SEVERE: event was nil in workspaceEventLooRouter")
				continue
			}

			log.V(sharedutil.LogLevel_Debug).Info("workspaceEventLoop received event",
				"event", eventlooptypes.StringEventLoopEvent(event.Event))

			// If it's a GitOpsDeploymentSyncRun:
			//   - If the SyncRun exists, use the name that was specified
			//   - If the Syncrun doesn't exist, retrieve from the database
			//     - If it's not found in the database, it's orphaned.

			// Check GitOpsDeployment resource events that come in: if we get an event for GitOpsDeployment that has an
			// orphaned child resource depending on it, then unorphan the child resource.
			if event.Event.ReqResource == eventlooptypes.GitOpsDeploymentTypeName {
				unorphanResourcesIfPossible(ctx, event, orphanedResources, input, log)
			}

			// Handle Repository Credentials
			if event.Event.ReqResource == eventlooptypes.GitOpsDeploymentRepositoryCredentialTypeName {
				workspaceResourceLoop.processRepositoryCredential(ctx, event.Event.Request, event.Event.Client)
				continue
			}

			// Handle Managed Environment
			if event.Event.ReqResource == eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName {

				// Reconcile to managed environment in the workspace resource loop, to ensure it is up-to-date (exists, or is deleted)
				workspaceResourceLoop.processManagedEnvironment(ctx, event, event.Event.Client)

				continue
			}

			var associatedGitOpsDeploymentName string // name of the GitOpsDeployment (in this namespace) that this resource is associated with

			if event.Event.ReqResource == eventlooptypes.GitOpsDeploymentTypeName {
				associatedGitOpsDeploymentName = event.Event.Request.Name

			} else if event.Event.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {

				associatedGitOpsDeploymentName = checkIfOrphanedGitOpsDeploymentSyncRun(ctx, event, orphanedResources, log)

				if associatedGitOpsDeploymentName == "" {
					// The SyncRun no longer exists, or an unrecoverable error occurred, so just continue
					continue
				}
			}

			if associatedGitOpsDeploymentName == "" {
				// Sanity check the name field is non-empty
				log.Error(nil, "SEVERE: received a resource event that we could not determine a GitOpsDeployment name for", "event", eventlooptypes.StringEventLoopEvent(event.Event))
				continue
			}

			mapKey := namespaceID + "-" + event.Event.Request.Namespace + "-" + associatedGitOpsDeploymentName

			applicationEntryVal, exists := applicationMap[mapKey]
			if !exists {
				// Start the application event queue go-routine, if it's not already started.

				// Start the application event loop's goroutine
				inputChan := applEventLoopFactory.startApplicationEventQueueLoop(ctx, associatedGitOpsDeploymentName, event.Event.Request.Namespace,
					event.Event.WorkspaceID, sharedResourceEventLoop)

				applicationEntryVal = workspaceEventLoop_applicationEventLoopEntry{
					input: inputChan,
				}
				applicationMap[mapKey] = applicationEntryVal
			}

			// Send the event to the channel/go routine that handles all events for this application/gitopsdepl (non-blocking)
			applicationEntryVal.input <- eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event:       event.Event,
			}

		} else if wrapperEvent.messageType == managedEnvProcessed_Event {

			log.V(sharedutil.LogLevel_Debug).Info(fmt.Sprintf("received ManagedEnvironment event, passed event to %d applications",
				len(applicationMap)))

			// Send a message about this ManagedEnvironment to all of the goroutines currently processing GitOpsDeployment/SyncRuns
			for key := range applicationMap {
				applicationEntryVal := applicationMap[key]

				go func() {
					applicationEntryVal.input <- eventlooptypes.EventLoopMessage{
						MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
						Event:       event.Event,
					}
				}()
			}

		} else {
			log.Error(nil, "SEVERE: unrecognized workspace event loop message type")
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
	startApplicationEventQueueLoop(ctx context.Context, gitopsDeplName string, gitopsDeplNamespace string, workspaceID string,
		sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop) chan eventlooptypes.EventLoopMessage
}

type defaultApplicationEventLoopFactory struct {
}

// The default implementation of startApplicationEventQueueLoop is just a simple wrapper around a call to
// StartApplicationEventQueueLoop
func (defaultApplicationEventLoopFactory) startApplicationEventQueueLoop(ctx context.Context, gitopsDeplName string, gitopsDeplNamespace string,
	workspaceID string, sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop) chan eventlooptypes.EventLoopMessage {

	res := application_event_loop.StartApplicationEventQueueLoop(ctx, gitopsDeplName, gitopsDeplNamespace, workspaceID, sharedResourceEventLoop)

	return res
}

var _ applicationEventQueueLoopFactory = defaultApplicationEventLoopFactory{}

// Processes events related to GitOpsDeploymentSyncRuns: determines whether the referenced GitOpsDeployment exists.
// - If the GitOpsDepl exists, return the name
// - Otherwise, add the SyncRun resource to orphaned list (why? because it is a gitopsdeplsyncrun that refers to a gitopsdepl that doesn't exist)
//
// If the resource is orphaned, or an error occurred, "" is returned.
//
//   See https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.8tiycl1h7rns for details.
func checkIfOrphanedGitOpsDeploymentSyncRun(ctx context.Context, event eventlooptypes.EventLoopMessage,
	orphanedResources map[string]map[string]eventlooptypes.EventLoopEvent, log logr.Logger) string {

	if event.Event.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {

		// 1) Retrieve the SyncRun
		syncRunCR := &v1alpha1.GitOpsDeploymentSyncRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      event.Event.Request.Name,
				Namespace: event.Event.Request.Namespace,
			},
		}
		if err := event.Event.Client.Get(ctx, client.ObjectKeyFromObject(syncRunCR), syncRunCR); err != nil {
			if apierr.IsNotFound(err) {
				log.V(sharedutil.LogLevel_Debug).Info("skipping potentially orphaned resource that could no longer be found:", "resource", syncRunCR.ObjectMeta)
				return ""
			} else {
				log.Error(err, "unexpected client error on retrieving syncrun object", "resource", syncRunCR.ObjectMeta)
				return ""
			}
		}

		// 2) Retrieve the GitOpsDeployment, referenced by the SyncRun, to see if the SyncRun is orphaned
		gitopsDeplCR := &v1alpha1.GitOpsDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      syncRunCR.Spec.GitopsDeploymentName,
				Namespace: event.Event.Request.Namespace,
			},
		}
		if err := event.Event.Client.Get(ctx, client.ObjectKeyFromObject(gitopsDeplCR), gitopsDeplCR); err != nil {

			if !apierr.IsNotFound(err) {
				log.Error(err, "unexpected client error on retrieving GitOpsDeployment references by GitOpsDeploymentSyncRun", "resource", syncRunCR.ObjectMeta)
				return ""
			}

			log.V(sharedutil.LogLevel_Debug).Info("was unable to locate GitOpsDeployment referenced by GitOpsDeploymentSyncRun, so the SyncRun is still orphaned.")

			// Otherwise, the GitOpsDeployment couldn't be found: therefore the SyncRun is orphaned, so continue.

		} else {
			// The resource is not orphaned, so our work is done: return the GitOpsDeployment referenced by the GitOpsDeploymentSyncRun
			return gitopsDeplCR.Name
		}

		// At this point, the SyncRun exists, but the GitOpsDeployment referenced by it doesn't

		// 3) Retrieve the list of orphaned resources that are depending on the GitOpsDeployment name referenced in the spec
		// field of the SyncRun.
		gitopsDeplMap, exists := orphanedResources[syncRunCR.Spec.GitopsDeploymentName]
		if !exists {
			gitopsDeplMap = map[string]eventlooptypes.EventLoopEvent{}
			orphanedResources[syncRunCR.Spec.GitopsDeploymentName] = gitopsDeplMap
		}

		// 4) Add this resource event to that list
		log.V(sharedutil.LogLevel_Debug).Info("Adding syncrun CR to orphaned resources list, name: " + syncRunCR.Name + ", missing gitopsdepl name: " + syncRunCR.Spec.GitopsDeploymentName)
		gitopsDeplMap[syncRunCR.Name] = *event.Event

		return ""

	} else {

		log.Error(nil, "SEVERE: unexpected event resource type in handleOrphaned")
		return ""
	}

}

func unorphanResourcesIfPossible(ctx context.Context, event eventlooptypes.EventLoopMessage,
	orphanedResources map[string]map[string]eventlooptypes.EventLoopEvent,
	input chan workspaceEventLoopMessage, log logr.Logger) {

	// If there exists an orphaned resource that is waiting for this gitopsdepl (by name)
	if gitopsDeplMap, exists := orphanedResources[event.Event.Request.Name]; exists {

		// Retrieve the gitopsdepl
		gitopsDeplCR := &v1alpha1.GitOpsDeployment{
			ObjectMeta: metav1.ObjectMeta{
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
					log.V(sharedutil.LogLevel_Debug).Info("requeueing orphaned resource: " + eventToRequeue.Event.Request.Name +
						", for parent: " + gitopsDeplCR.Name + " in namespace " + gitopsDeplCR.Namespace)
					input <- workspaceEventLoopMessage{
						messageType: workspaceEventLoopMessageType_Event,
						payload:     eventToRequeue,
					}
				}
			}()
		}
	}
}
