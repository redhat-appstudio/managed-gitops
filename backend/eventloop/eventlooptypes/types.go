package eventlooptypes

import (
	"context"
	"fmt"

	gitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventLoopEventType string

const (
	DeploymentModified           EventLoopEventType = "DeploymentModified"
	RepositoryCredentialModified EventLoopEventType = "RepositoryCredentialModified"
	ManagedEnvironmentModified   EventLoopEventType = "ManagedEnvironmentModified"
	SyncRunModified              EventLoopEventType = "SyncRunModified"
	UpdateDeploymentStatusTick   EventLoopEventType = "UpdateDeploymentStatusTick"
)

const KubeSystemNamespace = "kube-system"

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
	ReqResource GitOpsResourceType

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

	// ApplicationEventLoopMessageType_StatusCheck is a periodic ticker that tells the application event loop to
	// check if it should terminate its goroutine
	ApplicationEventLoopMessageType_StatusCheck
)

// eventlooptypes.StringEventLoopEvent is a utility function for debug purposes.
func StringEventLoopEvent(obj *EventLoopEvent) string {
	if obj == nil {
		return "(nil)"
	}

	return fmt.Sprintf("[%s] %s/%s/%s, for workspace '%s'", obj.EventType, obj.Request.Namespace,
		obj.Request.Name, string(obj.ReqResource), obj.WorkspaceID)

}

type GitOpsResourceType string

const (
	GitOpsDeploymentTypeName                     GitOpsResourceType = "GitOpsDeployment"
	GitOpsDeploymentSyncRunTypeName              GitOpsResourceType = "GitOpsDeploymentSyncRun"
	GitOpsDeploymentRepositoryCredentialTypeName GitOpsResourceType = "GitOpsDeploymentRepositoryCredential"
	GitOpsDeploymentManagedEnvironmentTypeName   GitOpsResourceType = "GitOpsDeploymentManagedEnvironmentTypeName"
)

func GetWorkspaceIDFromNamespaceID(namespace corev1.Namespace) string {
	// Here we assume that the namespace UID is the same as the workspace UID. If/when that changes, this should be updated.
	return string(namespace.UID)
}

// GetK8sClientForGitOpsEngineInstance returns a client for accessing resources from a GitOpsEngine cluster based on the environment.
// Returns a normal client that targets the same cluster as backend.
func GetK8sClientForGitOpsEngineInstance(ctx context.Context, gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {

	// TODO: GITOPSRVCE-66: Update this once we support using Argo CD instances that are running on a separate cluster	serviceClient, err := GetK8sClientForServiceWorkspace()
	serviceClient, err := GetK8sClientForServiceWorkspace()
	if err != nil {
		return nil, err
	}

	return serviceClient, nil
}

// GetK8sClientForServiceWorkspace returns a client for service provider workspace
func GetK8sClientForServiceWorkspace() (client.Client, error) {
	config, err := sharedutil.GetRESTConfig()
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	err = gitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	k8sClient = sharedutil.IfEnabledSimulateUnreliableClient(k8sClient)
	if err != nil {
		return nil, err
	}

	return k8sClient, nil

}
