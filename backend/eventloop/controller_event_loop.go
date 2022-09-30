package eventloop

import (
	"context"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// The controller event loop receives events from the pre-process event loop.

// The controller event loop maintains a list of active API namespaces, which contain GitOps Service API resources.

// The responsibility of the controller event loop is to:
// - Shard events between namespace goroutines:
//     - e.g. Events on namespace 'A' go to the Workspace Event Loop instance responsible for handling namespace 'A'.
//       Events on namespace 'B' go to the Workspace Event Loop instance responsible for handling namespace 'B'. Etc.
// - Ensure a new Workspace Event Loop goroutine is running for each active workspace. (When a new workspace is first
//   encountered, a new instance of the goroutine is started).

// Cardinality: A single instance of the controller event loop (e.g. a single goroutine) exists for the whole of
// the GitOps Service backend.

type ControllerEventLoop struct {
	EventLoopInputChannel chan eventlooptypes.EventLoopEvent
}

func NewControllerEventLoop() *ControllerEventLoop {

	channel := make(chan eventlooptypes.EventLoopEvent)
	go controllerEventLoopRouter(channel, defaultWorkspaceEventLoopRouterFactory{})

	res := &ControllerEventLoop{
		EventLoopInputChannel: channel,
	}

	return res
}

// newControllerEventLoopWithFactory is primarily for unit tests that want to mock a factory to catch
// events sent by the controller event loop.
//
// Note: All non-unit-test-based code should use 'newControllerEventLoop', defined above.
func newControllerEventLoopWithFactory(factory workspaceEventLoopRouterFactory) *ControllerEventLoop {

	channel := make(chan eventlooptypes.EventLoopEvent)
	go controllerEventLoopRouter(channel, factory)

	res := &ControllerEventLoop{
		EventLoopInputChannel: channel,
	}

	return res

}

// controllerEventLoop_workspaceEntry maintains individual state for each workspace
type controllerEventLoop_workspaceEntry struct {
	workspaceID        string
	workspaceEventLoop WorkspaceEventLoopRouterStruct
	// This struct should only be used from within the controller event loop
}

// controllerEventLoopRouter routes messages to the channel/go routine responsible for handling a particular workspace's events
// This channel is non-blocking.
func controllerEventLoopRouter(input chan eventlooptypes.EventLoopEvent, workspaceEventFactory workspaceEventLoopRouterFactory) {

	eventLoopRouterLog := log.FromContext(context.Background())

	eventLoopRouterLog.Info("controllerEventLoopRouter started.")
	defer eventLoopRouterLog.Error(nil, "SEVERE: controllerEventLoopRouter ended.")

	workspaceEntries := map[string] /* workspace id -> */ controllerEventLoop_workspaceEntry{}

	for {

		event := <-input

		eventLoopRouterLog.V(sharedutil.LogLevel_Debug).Info("eventLoop received event",
			"event", eventlooptypes.StringEventLoopEvent(&event), "workspace", event.WorkspaceID)

		workspaceEntryVal, ok := workspaceEntries[event.WorkspaceID]
		if !ok {

			workspaceEventLoop := workspaceEventFactory.startWorkspaceEventLoopRouter(event.WorkspaceID)

			// Start the workspace's event loop go-routine, if it's not already started.
			workspaceEntryVal = controllerEventLoop_workspaceEntry{
				workspaceID:        event.WorkspaceID,
				workspaceEventLoop: workspaceEventLoop,
			}
			workspaceEntries[event.WorkspaceID] = workspaceEntryVal

		}

		// Send the event to the channel/go routine that handles all events for this workspace (non-blocking)
		workspaceEntryVal.workspaceEventLoop.SendMessage(eventlooptypes.EventLoopMessage{
			MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
			Event:       &event,
		})
	}
}

// workspaceEventLoopRouterFactory implements a method that is responsible for starting a new workspace event loop
// router goroutine, and then returning a channel which can be used to send messages to that goroutine.
//
// defaultWorkspaceEventLoopRouterFactory should always be used, unless a mocked replacement is needed
// for a unit test.
type workspaceEventLoopRouterFactory interface {
	startWorkspaceEventLoopRouter(workspaceID string) WorkspaceEventLoopRouterStruct
}

type defaultWorkspaceEventLoopRouterFactory struct{}

var _ workspaceEventLoopRouterFactory = defaultWorkspaceEventLoopRouterFactory{}

func (defaultWorkspaceEventLoopRouterFactory) startWorkspaceEventLoopRouter(workspaceID string) WorkspaceEventLoopRouterStruct {

	return newWorkspaceEventLoopRouter(workspaceID)

}
