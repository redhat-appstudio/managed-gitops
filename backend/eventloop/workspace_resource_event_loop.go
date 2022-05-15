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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type workspaceResourceEventLoop struct {
	inputChannel chan workspaceResourceLoopMessage
}

type workspaceResourceLoopMessage struct {
	apiNamespaceClient client.Client
	apiNamespace       corev1.Namespace

	messageType     workspaceResourceLoopMessageType
	responseChannel chan (interface{})

	// optional payload
	payload interface{}
}

type workspaceResourceLoopMessageType string

const (
	workspaceResourceLoopMessageType_processRepositoryCredential workspaceResourceLoopMessageType = "processRepositoryCredential"
)

func (werl *workspaceResourceEventLoop) processRepositoryCredential(ctx context.Context, repoCredResource managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential,
	apiNamespace corev1.Namespace, apiNamespaceClient client.Client) error {
	responseChannel := make(chan interface{})

	msg := workspaceResourceLoopMessage{
		apiNamespaceClient: apiNamespaceClient,
		apiNamespace:       apiNamespace,
		messageType:        workspaceResourceLoopMessageType_processRepositoryCredential,
		responseChannel:    responseChannel,
		payload:            repoCredResource,
	}

	werl.inputChannel <- msg

	select {
	case <-responseChannel:
	case <-ctx.Done():
		return fmt.Errorf("context cancelled in processRepositoryCredential")
	}

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
	log := log.FromContext(ctx)

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
		task := &WorkspaceResourceEventTask{
			msg:       msg,
			dbQueries: dbQueries,
			log:       log,
		}

		taskRetryLoop.AddTaskIfNotPresent(mapKey, task, sharedutil.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true})
	}
}

type WorkspaceResourceEventTask struct {
	msg       workspaceResourceLoopMessage
	dbQueries db.DatabaseQueries
	log       logr.Logger
}

// Returns true if the task should be retried, false otherwise
func (wert *WorkspaceResourceEventTask) PerformTask(taskContext context.Context) (bool, error) {

	retry, err := processWorkspaceResourceMessage(taskContext, wert.msg, wert.dbQueries, wert.log)

	return retry, err
}

func processWorkspaceResourceMessage(ctx context.Context, msg workspaceResourceLoopMessage, dbQueries db.DatabaseQueries, log logr.Logger) (bool, error) {

	log.V(sharedutil.LogLevel_Debug).Info("processWorkspaceResource received message: "+string(msg.messageType), "workspace",
		msg.apiNamespace.UID)

	if msg.messageType == workspaceResourceLoopMessageType_processRepositoryCredential {

		// repoCred, ok := (msg.payload).(managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential)
		// if !ok {
		// 	return false, fmt.Errorf("SEVERE: Unexpected payload type in workspace resource event loop")
		// }

		// Get or create repository credential in the database

		// Reply on a separate goroutine so cancelled callers don't block the event loop
		// go func() {
		// 	msg.responseChannel <- response
		// }()

	} else {
		log.Error(nil, "SEVERE: unrecognized sharedResourceLoopMessageType: "+string(msg.messageType))
	}

	return false, fmt.Errorf("unimplemented")
}
