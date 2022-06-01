package eventloop

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// workspaceResourceEventLoop is responsible for handling workspace-scoped events, like events for RepositoryCredentials resources.
type workspaceResourceEventLoop struct {
	inputChannel chan workspaceResourceLoopMessage
}

type workspaceResourceLoopMessage struct {
	apiNamespaceClient client.Client

	messageType workspaceResourceLoopMessageType

	// optional payload
	payload interface{}
}

type workspaceResourceLoopMessageType string

const (
	workspaceResourceLoopMessageType_processRepositoryCredential workspaceResourceLoopMessageType = "processRepositoryCredential"
)

func (werl *workspaceResourceEventLoop) processRepositoryCredential(ctx context.Context, req ctrl.Request, apiNamespaceClient client.Client) error {

	msg := workspaceResourceLoopMessage{
		apiNamespaceClient: apiNamespaceClient,
		messageType:        workspaceResourceLoopMessageType_processRepositoryCredential,
		payload:            req,
	}

	werl.inputChannel <- msg

	return nil

}

func newWorkspaceResourceLoop() *workspaceResourceEventLoop {

	workspaceResourceEventLoop := &workspaceResourceEventLoop{
		inputChannel: make(chan workspaceResourceLoopMessage),
	}

	go internalWorkspaceResourceEventLoop(workspaceResourceEventLoop.inputChannel)

	return workspaceResourceEventLoop
}

func internalWorkspaceResourceEventLoop(inputChan chan workspaceResourceLoopMessage) {

	ctx := context.Background()
	log := log.FromContext(ctx).WithValues("component", "workspace_resource_event_loop")

	dbQueries, err := db.NewProductionPostgresDBQueries(false)
	if err != nil {
		log.Error(err, "SEVERE: internalSharedResourceEventLoop exiting before startup")
		return
	}

	taskRetryLoop := sharedutil.NewTaskRetryLoop("workspace-resource-event-retry-loop")

	for {
		msg := <-inputChan

		var mapKey string

		if msg.messageType == workspaceResourceLoopMessageType_processRepositoryCredential {

			repoCred, ok := (msg.payload).(managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential)
			if !ok {
				log.Error(nil, "SEVERE: Unexpected payload type in workspace resource event loop")
				continue
			}

			mapKey = "repo-cred-" + repoCred.Namespace + "-" + repoCred.Name

		} else {
			log.Error(nil, "SEVERE: Unexpected message type: "+string(msg.messageType))
			continue
		}

		// TODO: GITOPS-1702 - PERF - Use a more memory efficient key

		// Pass the event to the retry loop, for processing
		task := &workspaceResourceEventTask{
			msg:       msg,
			dbQueries: dbQueries,
			log:       log,
		}

		taskRetryLoop.AddTaskIfNotPresent(mapKey, task, sharedutil.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true})
	}
}

type workspaceResourceEventTask struct {
	msg       workspaceResourceLoopMessage
	dbQueries db.DatabaseQueries
	log       logr.Logger
}

// Returns true if the task should be retried, false otherwise
func (wert *workspaceResourceEventTask) PerformTask(taskContext context.Context) (bool, error) {

	retry, err := processWorkspaceResourceMessage(taskContext, wert.msg, wert.dbQueries, wert.log)

	return retry, err
}

func processWorkspaceResourceMessage(ctx context.Context, msg workspaceResourceLoopMessage, dbQueries db.DatabaseQueries, log logr.Logger) (bool, error) {

	log.V(sharedutil.LogLevel_Debug).Info("processWorkspaceResource received message: " + string(msg.messageType))

	if msg.apiNamespaceClient == nil {
		return false, fmt.Errorf("invalid namespace client")
	}

	if msg.messageType == workspaceResourceLoopMessageType_processRepositoryCredential {

		req, ok := (msg.payload).(ctrl.Request)
		if !ok {
			return false, fmt.Errorf("invalid payload in processWorkspaceResourceMessage")
		}

		// Retrieve the namespace that the repository credential is contained within
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Namespace,
				Namespace: req.Namespace,
			},
		}
		if err := msg.apiNamespaceClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {

			if !apierr.IsNotFound(err) {
				return true, fmt.Errorf("unexpected error in retrieving repo credentials: %v", err)
			}

			log.V(sharedutil.LogLevel_Warn).Info("Received a message for a repository credential in a namepace that doesn't exist", "namespace", namespace)
			return false, nil
		}

		repoCreds := &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{}

		if err := msg.apiNamespaceClient.Get(ctx, req.NamespacedName, repoCreds); err != nil {

			if !apierr.IsNotFound(err) {
				return true, fmt.Errorf("unexpected error in retrieving repo credentials: %v", err)
			}

			// The repository credentials necessarily don't exist

			// Find any existing database resources for repository credentials that previously existing with this namespace/name
			apiCRToDBMappingList := []db.APICRToDatabaseMapping{}
			if err := dbQueries.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx, db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
				req.Name, req.Namespace, string(namespace.GetUID()), db.APICRToDatabaseMapping_DBRelationType_RepositoryCredential, &apiCRToDBMappingList); err != nil {

				return true, fmt.Errorf("unable to list APICRs for repository credentials: %v", err)
			}

			for _, item := range apiCRToDBMappingList {

				fmt.Println("STUB:", item)

				// TODO: GITOPSRVCE-96: STUB: Next steps:
				// - Delete the repository credential from the database
				// - Create the operation row, pointing to the deleted repository credentials table
				// - Delete the APICRToDatabaseMapping referenced by 'item'

			}

			// TODO: GITOPSRVCE-96:  STUB - reconcile on the missing repository credentials

			return false, fmt.Errorf("STUB: reconcile on the missing repository credentials")

		}

		// TODO: GITOPSRVCE-96: STUB: If it exists, compare it with what's in the database
		// - If it doesn't exist in the database, create it
		// - If it does exist in the database, but the values are different, update it

	} else {
		log.Error(nil, "SEVERE: unrecognized sharedResourceLoopMessageType: "+string(msg.messageType))
	}

	return false, fmt.Errorf("unimplemented")
}
