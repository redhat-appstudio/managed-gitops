package eventloop

import (
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventLoop struct {
	eventLoopInputChannel chan eventLoopEvent
}

type EventLoopEventType string

const (
	DeploymentModified EventLoopEventType = "DeploymentModified"
	// WorkspaceModified
	// ApplicationModified
	// EnvironmentModified
	SyncRunModified EventLoopEventType = "SyncRunModified"
)

func NewEventLoop() *EventLoop {

	channel := make(chan eventLoopEvent)
	go eventLoopRouter(channel)

	res := &EventLoop{}
	res.eventLoopInputChannel = channel

	return res

}

// EventReceived informs the event loop of a new event (this call is non-blocking)
func (evl *EventLoop) EventReceived(req ctrl.Request, reqResource managedgitopsv1alpha1.GitOpsResourceType, client client.Client, eventType EventLoopEventType, workspaceID string) {

	event := eventLoopEvent{request: req, eventType: eventType, workspaceID: workspaceID, client: client, reqResource: reqResource}
	evl.eventLoopInputChannel <- event
}
