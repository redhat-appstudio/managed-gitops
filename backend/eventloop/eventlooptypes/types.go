package eventlooptypes

import (
	"fmt"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventLoopEventType string

const (
	// WorkspaceModified
	// ApplicationModified
	// EnvironmentModified
	DeploymentModified         EventLoopEventType = "DeploymentModified"
	CredentialModified         EventLoopEventType = "CredentialModified"
	SyncRunModified            EventLoopEventType = "SyncRunModified"
	UpdateDeploymentStatusTick EventLoopEventType = "UpdateDeploymentStatusTick"
)

// EventLoopEvent tracks an event received from the controllers in the apis/managed-gitops/v1alpha1 package.
// For example, when a GitOpsDeployment is created/modified/deleted, an EventLoopEvent is created and
// is then processed by the event loops.
type EventLoopEvent struct {

	// EventType indicates the type of event, usually the modification of a resource
	EventType EventLoopEventType

	// Request from the event context
	Request ctrl.Request

	// Client from the event context
	Client client.Client

	// ReqResource indicates whether the event is for a GitOpsDeployment, or DeploymentSyncRun (or other resources)
	ReqResource managedgitopsv1alpha1.GitOpsResourceType

	// AssociatedGitopsDeplUID is the UID of the GitOpsDeployment resource that
	// - if 'request' is a GitOpsDeployment, then this field matches the UID of the resoruce
	// - if 'request' is a GitOpsDeploymentSyncRun, then this field matches the UID of the GitOpsDeployment referenced by the sync run's 'gitopsDeploymentName' field.
	AssociatedGitopsDeplUID string

	// WorkspaceID is the UID of the namespace that contains the request
	WorkspaceID string
}

// Packages an EventLoopEvent as a message between event loop channels.
// - The type of messages depends on the MessageType
type EventLoopMessage struct {
	MessageType EventLoopMessageType
	Event       *EventLoopEvent

	// ShutdownSignalled is included as part of workComplete message, to indicate that the goroutine has succesfully shut down.
	ShutdownSignalled bool
}

type EventLoopMessageType int

const (
	// ApplicationEventLoopMessageType_WorkComplete indicates the message indicates that a particular task has completed.
	// For example:
	ApplicationEventLoopMessageType_WorkComplete EventLoopMessageType = iota

	// ApplicationEventLoopMessageType_Event indicates that the message contains an event
	ApplicationEventLoopMessageType_Event
)

// eventlooptypes.StringEventLoopEvent is a utility function for debug purposes.
func StringEventLoopEvent(obj *EventLoopEvent) string {
	if obj == nil {
		return "(nil)"
	}

	return fmt.Sprintf("[%s] %s/%s/%s, for workspace '%s', gitopsdepluid: '%s'", obj.EventType, obj.Request.Namespace,
		obj.Request.Name, string(obj.ReqResource), obj.WorkspaceID, obj.AssociatedGitopsDeplUID)

}

func GetWorkspaceIDFromNamespaceID(namespace corev1.Namespace) string {
	// Here we assume that the namespace UID is the same as the workspace UID. If/when that changes, this should be updated.
	return string(namespace.UID)
}
