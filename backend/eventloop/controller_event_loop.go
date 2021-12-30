package eventloop

import (
	"context"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type controllerEventLoop struct {
	eventLoopInputChannel chan eventLoopEvent
}

func newControllerEventLoop() *controllerEventLoop {

	channel := make(chan eventLoopEvent)
	go controllerEventLoopRouter(channel)

	res := &controllerEventLoop{}
	res.eventLoopInputChannel = channel

	return res

}

// EventReceived is called by controllers (after being handled by the preprocess event loop) to inform us of a new event (this call is non-blocking)
func (evl *controllerEventLoop) EventReceived(req ctrl.Request, reqResource managedgitopsv1alpha1.GitOpsResourceType, client client.Client, eventType EventLoopEventType, workspaceID string) {

	event := eventLoopEvent{request: req, eventType: eventType, workspaceID: workspaceID, client: client, reqResource: reqResource}
	evl.eventLoopInputChannel <- event
}

// workspaceEntry maintains individual state for each workspace
type workspaceEntry struct {
	workspaceID               string
	workspaceEventLoopChannel chan applicationEventLoopMessage
	// This struct should only be used from within the controller event loop
}

// controllerEventLoopRouter routes messages to the channel/go routine responsible for handling a particular workspace's events
// This channel is non-blocking.
func controllerEventLoopRouter(input chan eventLoopEvent) {

	eventLoopRouterLog := log.FromContext(context.Background())

	eventLoopRouterLog.Info("controllerEventLoopRouter started.")
	defer eventLoopRouterLog.Error(nil, "SEVERE: controllerEventLoopRouter ended.")

	workspaceEntries := map[string] /* workspace id -> */ workspaceEntry{}

	for {

		event := <-input

		eventLoopRouterLog.V(sharedutil.LogLevel_Debug).Info("eventLoop received event", "event", stringEventLoopEvent(&event), "workspace", event.workspaceID)

		workspaceEntryVal, ok := workspaceEntries[event.workspaceID]
		if !ok {
			// Start the workspace's event loop go-routine, if it's not already started.
			workspaceEntryVal = workspaceEntry{
				workspaceID:               event.workspaceID,
				workspaceEventLoopChannel: make(chan applicationEventLoopMessage),
			}
			workspaceEntries[event.workspaceID] = workspaceEntryVal

			startApplicationEventLoopRouter(workspaceEntryVal.workspaceEventLoopChannel, event.workspaceID)
		}

		// Send the event to the channel/go routine that handles all events for this workspace (non-blocking)
		workspaceEntryVal.workspaceEventLoopChannel <- applicationEventLoopMessage{
			messageType: applicationEventLoopMessageType_Event,
			event:       &event,
		}

	}

}
