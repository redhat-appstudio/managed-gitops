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
	SyncRunModified            EventLoopEventType = "SyncRunModified"
	UpdateDeploymentStatusTick EventLoopEventType = "UpdateDeploymentStatusTick"
)

type eventLoopEvent struct {
	// eventType indicates the type of event, usually the modification of a resource
	eventType EventLoopEventType

	// request from the event context
	request ctrl.Request

	// client from the event context
	client client.Client

	// reqResource indicates whether the event is for a GitOpsDeployment, or DeploymentSyncRun (or other resources)
	reqResource managedgitopsv1alpha1.GitOpsResourceType

	// associatedGitopsDeplUID is the UID of the GitOpsDeployment resource that
	// - if 'request' is a GitOpsDeployment, then this field matches the UID of the resoruce
	// - if 'request' is a GitOpsDeploymentSyncRun, then this field matches the UID of the GitOpsDeployment referenced by the sync run's 'gitopsDeploymentName' field.
	associatedGitopsDeplUID string

	// workspaceID is the UID of the namespace that contains the request
	workspaceID string
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
