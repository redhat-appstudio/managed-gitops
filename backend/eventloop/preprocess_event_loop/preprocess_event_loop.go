package preprocess_event_loop

import (
	"context"

	"github.com/go-logr/logr"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Preprocess Event Loop
//
// The pre-process event loop is responsible for:
// - receives all events all the API Resources controllers
// - pass the events to the next layer, which is controller_event_loop

// EventReceived is called by controllers to inform of it changes to API CRs
func (evl *PreprocessEventLoop) EventReceived(req ctrl.Request, reqResource eventlooptypes.GitOpsResourceType,
	client client.Client, eventType eventlooptypes.EventLoopEventType, namespaceID string) {

	event := eventlooptypes.EventLoopEvent{Request: req, EventType: eventType, WorkspaceID: namespaceID,
		Client: client, ReqResource: reqResource}

	evl.eventLoopInputChannel <- event
}

type PreprocessEventLoop struct {
	eventLoopInputChannel chan eventlooptypes.EventLoopEvent
	nextStep              *eventloop.ControllerEventLoop
}

func NewPreprocessEventLoop() *PreprocessEventLoop {
	channel := make(chan eventlooptypes.EventLoopEvent)

	res := &PreprocessEventLoop{}
	res.eventLoopInputChannel = channel
	res.nextStep = eventloop.NewControllerEventLoop()

	go preprocessEventLoopRouter(channel, res.nextStep)

	return res

}

func preprocessEventLoopRouter(input chan eventlooptypes.EventLoopEvent, nextStep *eventloop.ControllerEventLoop) {

	ctx := context.Background()
	log := log.FromContext(ctx)

	for {

		// Block on waiting for more events
		newEvent := <-input

		emitEvent(newEvent, nextStep, "bypass", log)

	}
}

// emitEvent passes the given event to the controller event loop
func emitEvent(event eventlooptypes.EventLoopEvent, nextStep *eventloop.ControllerEventLoop, debugStr string, log logr.Logger) {

	if nextStep == nil {
		log.Error(nil, "SEVERE: controllerEventLoop pointer should never be nil")
		return
	}

	log.V(sharedutil.LogLevel_Debug).Info("Emitting event to workspace event loop",
		"event", eventlooptypes.StringEventLoopEvent(&event), "debug-context", debugStr)

	nextStep.EventLoopInputChannel <- event

}
