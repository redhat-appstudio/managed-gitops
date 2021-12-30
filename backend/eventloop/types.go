package eventloop

import (
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventLoopEventType string

const (
	DeploymentModified EventLoopEventType = "DeploymentModified"
	// WorkspaceModified
	// ApplicationModified
	// EnvironmentModified
	SyncRunModified EventLoopEventType = "SyncRunModified"
)

type eventLoopEvent struct {
	eventType   EventLoopEventType
	request     ctrl.Request
	reqResource managedgitopsv1alpha1.GitOpsResourceType

	// TODO: DEBT - depending on how controller-runtime caching works, including this in the struct might be a bad idea:
	client client.Client

	associatedGitopsDeplUID string
	workspaceID             string
}

type applicationEventLoopMessageType int

const (
	applicationEventLoopMessageType_WorkComplete applicationEventLoopMessageType = iota
	applicationEventLoopMessageType_Event
)

type applicationEventLoopMessage struct {
	messageType applicationEventLoopMessageType
	event       *eventLoopEvent

	// shutdownSignalled is included as part of workComplete message, to indicate that the goroutine has succesfully shut down.
	shutdownSignalled bool
}
