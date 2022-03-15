package eventloop

import (
	"context"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// The controller event loop receives events from the pre-process event loop. Events received here will necessarily:
// - Have the 'associatedGitopsDeplUID' field set. This field references either a GitOpsDeployment, or
//   contains 'orphaned-resource' string (indicating that a corresponding GitOpsDeployment doesn't exist, usually
//   this is an error.)

// The controller event loop maintains a list of active API namespaces, which contain GitOps Service API resources.

// The responsibility of the controller event loop is to:
// - Shard events between namespace goroutines:
//     - e.g. Events on namespace 'A' go to the Workspace Event Loop instance responsible for handling namespace 'A'.
//       Events on namespace 'B' go to the Workspace Event Loop instance responsible for handling namespace 'B'. Etc.
// - Ensure a new Workspace Event Loop goroutine is running for each active workspace. (When a new workspace is first
//   encountered, a new instance of the goroutine is started).

// Cardinality: A single instance of the controller event loop (e.g. a single goroutine) exists for the whole of
// the GitOps Service backend.

type controllerEventLoop struct {
	eventLoopInputChannel chan eventlooptypes.EventLoopEvent
}

func newControllerEventLoop() *controllerEventLoop {

	channel := make(chan eventlooptypes.EventLoopEvent)
	go controllerEventLoopRouter(channel)

	res := &controllerEventLoop{}
	res.eventLoopInputChannel = channel

	return res

}

// workspaceEntry maintains individual state for each workspace
type workspaceEntry struct {
	workspaceID               string
	workspaceEventLoopChannel chan eventlooptypes.EventLoopMessage
	// This struct should only be used from within the controller event loop
}

// controllerEventLoopRouter routes messages to the channel/go routine responsible for handling a particular workspace's events
// This channel is non-blocking.
func controllerEventLoopRouter(input chan eventlooptypes.EventLoopEvent) {

	eventLoopRouterLog := log.FromContext(context.Background())

	eventLoopRouterLog.Info("controllerEventLoopRouter started.")
	defer eventLoopRouterLog.Error(nil, "SEVERE: controllerEventLoopRouter ended.")

	workspaceEntries := map[string] /* workspace id -> */ workspaceEntry{}

	for {

		event := <-input

		eventLoopRouterLog.V(sharedutil.LogLevel_Debug).Info("eventLoop received event", "event", eventlooptypes.StringEventLoopEvent(&event), "workspace", event.WorkspaceID)

		workspaceEntryVal, ok := workspaceEntries[event.WorkspaceID]
		if !ok {
			// Start the workspace's event loop go-routine, if it's not already started.
			workspaceEntryVal = workspaceEntry{
				workspaceID:               event.WorkspaceID,
				workspaceEventLoopChannel: make(chan eventlooptypes.EventLoopMessage),
			}
			workspaceEntries[event.WorkspaceID] = workspaceEntryVal

			startWorkspaceEventLoopRouter(workspaceEntryVal.workspaceEventLoopChannel, event.WorkspaceID)
		}

		// Send the event to the channel/go routine that handles all events for this workspace (non-blocking)
		workspaceEntryVal.workspaceEventLoopChannel <- eventlooptypes.EventLoopMessage{
			MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
			Event:       &event,
		}

	}

}
