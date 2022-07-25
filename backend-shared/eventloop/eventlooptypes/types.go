package eventlooptypes

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"k8s.io/apimachinery/pkg/runtime"
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
	ReqResource managedgitopsv1alpha1.GitOpsResourceType

	// AssociatedGitopsDeplUID is the UID of the GitOpsDeployment resource that
	// - if 'request' is a GitOpsDeployment, then this field matches the UID of the resoruce
	// - if 'request' is a GitOpsDeploymentSyncRun, then this field matches the UID of the GitOpsDeployment referenced by the sync run's 'gitopsDeploymentName' field.
	AssociatedGitopsDeplUID string

	// WorkspaceID is the UID of the namespace that contains the request
	WorkspaceID string
}

const (
	// orphanedResourceGitopsDeplUID indicates that a GitOpsDeploymentSyncRunCR is orphaned, which means
	// we do not know which GitOpsDeployment it should belong to. This is usually because the deployment name
	// field of the SyncRun refers to a K8s resource that doesn't (or no longer) exists.
	// See https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.8tiycl1h7rns for details.
	OrphanedResourceGitopsDeplUID = "orphaned"

	// noAssociatedGitOpsDeploymentUID: if a resource does not have an orphanedResourceDeplUID, this constant should be set.
	// For example: GitOpsDeploymentRepositoryCredentials might be associated with multiple (or zero) GitOpsDeployments.
	NoAssociatedGitOpsDeploymentUID = "none"
)

// GetReqResourceAsClientObject converts the resource into a simple client.Object: it will be of
// the expected type (GitOpsDeployment/SyncRun/etc), but only contain the name and namespace.
func (ele *EventLoopEvent) GetReqResourceAsSimpleClientObject() (client.Object, error) {

	var resource client.Object

	if ele.ReqResource == managedgitopsv1alpha1.GitOpsDeploymentTypeName {
		resource = &managedgitopsv1alpha1.GitOpsDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ele.Request.Name,
				Namespace: ele.Request.Namespace,
			},
		}
	} else if ele.ReqResource == managedgitopsv1alpha1.GitOpsDeploymentSyncRunTypeName {
		resource = &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ele.Request.Name,
				Namespace: ele.Request.Namespace,
			},
		}
	} else if ele.ReqResource == managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialTypeName {
		resource = &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ele.Request.Name,
				Namespace: ele.Request.Namespace,
			},
		}
	} else {
		return nil, fmt.Errorf("SEVERE - unexpected request resource type: %v", string(ele.ReqResource))
	}

	return resource, nil
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

// Global variables to identify resources created by Namespace Reconciler.
const (
	IdentifierKey   = "source"
	IdentifierValue = "periodic-cleanup"
)

func CreateOperation(ctx context.Context, waitForOperation bool, dbOperationParam db.Operation, clusterUserID string,
	operationNamespace string, dbQueries db.ApplicationScopedQueries, gitopsEngineClient client.Client,
	log logr.Logger) (*managedgitopsv1alpha1.Operation, *db.Operation, error) {

	var err error

	var dbOperationList []db.Operation
	if err = dbQueries.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, dbOperationParam.Resource_id, dbOperationParam.Resource_type, &dbOperationList, clusterUserID); err != nil {
		log.Error(err, "unable to fetch List of Operations for ResourceId: "+dbOperationParam.Resource_id+", Type: "+dbOperationParam.Resource_type+", OwnerId: "+clusterUserID)
	}

	// Iterate through existing DB entries for a given resource: look to see if there is already an Operation
	// in the waiting state.
	for idx := range dbOperationList {

		dbOperation := dbOperationList[idx]

		if dbOperation.State != db.OperationState_Waiting {
			continue
		}

		k8sOperation := operation.Operation{
			// TODO: GITOPSRVCE-195: Update this when standardizing operation CRs
			ObjectMeta: metav1.ObjectMeta{
				Name:      "operation-" + dbOperation.Operation_id,
				Namespace: operationNamespace,
			},
		}

		if err = gitopsEngineClient.Get(ctx, client.ObjectKeyFromObject(&k8sOperation), &k8sOperation); err != nil {
			log.Error(err, "unable to fetch existing Operation "+k8sOperation.Name+" from cluster.")
		} else {
			// An operation already exists in waiting state, and the Operation CR for it still exists, so we don't need to create
			// a new operation.
			log.Info("Skipping Operation creation, as it already exists for resource" + dbOperationParam.Resource_id + ", it is in " + string(dbOperation.State) + " state.")
			return &k8sOperation, &dbOperation, nil
		}
	}

	dbOperation := db.Operation{
		Instance_id:             dbOperationParam.Instance_id,
		Resource_id:             dbOperationParam.Resource_id,
		Resource_type:           dbOperationParam.Resource_type,
		Operation_owner_user_id: clusterUserID,
		Created_on:              time.Now(),
		Last_state_update:       time.Now(),
		State:                   db.OperationState_Waiting,
		Human_readable_state:    "",
	}

	if err := dbQueries.CreateOperation(ctx, &dbOperation, clusterUserID); err != nil {
		log.Error(err, "unable to create operation", "operation", dbOperation.LongString())
		return nil, nil, err
	}

	log.Info("Created database operation", "operation", dbOperation.ShortString())

	// Create K8s operation
	operation := managedgitopsv1alpha1.Operation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "operation-" + dbOperation.Operation_id,
			Namespace: operationNamespace,
		},
		Spec: managedgitopsv1alpha1.OperationSpec{
			OperationID: dbOperation.Operation_id,
		},
	}

	// Set annotation as an identifier for Operations created by NameSpace Reconciler.
	if clusterUserID == db.SpecialClusterUserName {
		operation.Annotations = map[string]string{IdentifierKey: IdentifierValue}
	}
	log.Info("Creating K8s Operation CR", "operation", fmt.Sprintf("%v", operation.Spec.OperationID))

	if err := gitopsEngineClient.Create(ctx, &operation, &client.CreateOptions{}); err != nil {
		log.Error(err, "unable to create K8s Operation in namespace", "operation", dbOperation.Operation_id, "namespace", operation.Namespace)
		return nil, nil, err
	}

	// Wait for operation to complete.
	if waitForOperation {
		log.Info("Waiting for Operation to complete", "operation", fmt.Sprintf("%v", operation.Spec.OperationID))

		if err = waitForOperationToComplete(ctx, &dbOperation, dbQueries, log); err != nil {
			log.Error(err, "operation did not complete", "operation", dbOperation.Operation_id, "namespace", operation.Namespace)
			return nil, nil, err
		}

		log.Info("Operation completed", "operation", fmt.Sprintf("%v", operation.Spec.OperationID))
	}

	return &operation, &dbOperation, nil

}

// waitForOperationToComplete waits for an Operation database entry to have 'Completed' or 'Failed' status.
//
func waitForOperationToComplete(ctx context.Context, dbOperation *db.Operation, dbQueries db.ApplicationScopedQueries, log logr.Logger) error {

	backoff := sharedutil.ExponentialBackoff{Factor: 2, Min: time.Duration(100 * time.Millisecond), Max: time.Duration(10 * time.Second), Jitter: true}

	for {

		err := dbQueries.GetOperationById(ctx, dbOperation)
		if err != nil {
			// Either the operation couldn't be found (which shouldn't happen here), or some other issue, so return it
			return err
		}

		if err == nil && (dbOperation.State == db.OperationState_Completed || dbOperation.State == db.OperationState_Failed) {
			break
		}

		backoff.DelayOnFail(ctx)

		// Break if the request is cancelled, or the timeout expires
		select {
		case <-ctx.Done():
			return fmt.Errorf("operation context is Done() in waitForOperationToComplete")
		default:
		}

	}

	return nil
}

func GetK8sClientForGitOpsEngineInstance(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {

	// TODO: GITOPSRVCE-73: When we support multiple Argo CD instances (and multiple instances on separate clusters), this logic should be updated.

	config, err := sharedutil.GetRESTConfig()
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	err = managedgitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = managedgitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return k8sClient, nil

}
