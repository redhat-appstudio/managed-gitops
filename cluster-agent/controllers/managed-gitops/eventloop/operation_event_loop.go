package eventloop

import (
	"context"
	"fmt"
	"reflect"
	"time"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

// OperationEventLoop is informed of Operation resource changes (creations/modifications/deletions) by
// the Operation controller (operation_controller.go).
//
// Next, these resource changes (events) are then processed within a task retry loop.
// - Operations are how the backend informs the cluster-agent of database changes.
// - For example:
//     - The user updated a field in a GitOpsDeployment in the user's namespace
//     - The managed-gitops backend updated corresponding field in the Application database row
//     - Next: an Operation was created to inform the cluster-agent component of the Application row changed
//     - We are here: now the Operation controller of cluster-agent has been informed of the Operation.
//     - We need to: Look at the Operation, determine what database entry changed, and ensure that Argo CD
//                   is reconciled to the contents of the database entry.
//
// The overall workflow of Operations can be found in the architecture doc:
// https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.9vyguee8vhow
//
type OperationEventLoop struct {
	eventLoopInputChannel chan operationEventLoopEvent
}

func NewOperationEventLoop() *OperationEventLoop {
	channel := make(chan operationEventLoopEvent)

	res := &OperationEventLoop{}
	res.eventLoopInputChannel = channel

	go operationEventLoopRouter(channel)

	return res

}

type operationEventLoopEvent struct {
	request ctrl.Request
	client  client.Client
}

func (evl *OperationEventLoop) EventReceived(req ctrl.Request, client client.Client) {

	event := operationEventLoopEvent{request: req, client: client}
	evl.eventLoopInputChannel <- event
}

func operationEventLoopRouter(input chan operationEventLoopEvent) {

	ctx := context.Background()

	log := log.FromContext(ctx)

	taskRetryLoop := sharedutil.NewTaskRetryLoop("cluster-agent")

	log.Info("controllerEventLoopRouter started")

	for {
		newEvent := <-input

		mapKey := newEvent.request.Name + "-" + newEvent.request.Namespace

		// Queue a new task in the task retry loop for our event.
		task := &processOperationEventTask{
			event: operationEventLoopEvent{
				request: newEvent.request,
				client:  newEvent.client,
			},
			log: log,
		}
		taskRetryLoop.AddTaskIfNotPresent(mapKey, task, sharedutil.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true})

	}

}

// processOperationEventTask takes as input an Operation resource event, and processes it based on the contents of that event.
type processOperationEventTask struct {
	event operationEventLoopEvent
	log   logr.Logger
}

// PerformTask takes as input an Operation resource event, and processes it based on the contents of that event.
//
// Returns bool (true if the task should be retried, for example because it failed, false otherwise),
// and error (an error to log on failure).
//
// NOTE: 'error' value does not affect whether the task will be retried, this error is only used for
// error reporting.
func (task *processOperationEventTask) PerformTask(taskContext context.Context) (bool, error) {
	dbQueries, err := db.NewProductionPostgresDBQueries(true)
	if err != nil {
		return true, fmt.Errorf("unable to instantiate database in operation controller loop: %v", err)
	}
	defer dbQueries.CloseDatabase()

	// Process the event (this is where most of the work is done)
	dbOperation, shouldRetry, err := task.internalPerformTask(taskContext, dbQueries)

	if dbOperation != nil {

		// After the event is processed, update the status in the database

		// Don't update the status of operations that have previously completed.
		if dbOperation.State == db.OperationState_Completed || dbOperation.State == db.OperationState_Failed {
			return false, err
		}

		// If the task failed and thus should be retried...
		if shouldRetry {
			// Not complete, still (re)trying.
			dbOperation.State = db.OperationState_In_Progress
		} else {

			// Complete (but complete doesn't mean successful: it could be complete due to a fatal error)
			if err == nil {
				dbOperation.State = db.OperationState_Completed
			} else {
				dbOperation.State = db.OperationState_Failed
			}
		}
		dbOperation.Last_state_update = time.Now()

		if err != nil {
			// TODO: GITOPSRVCE-77 - SECURITY - At some point, we will likely want to sanitize the error value for users
			dbOperation.Human_readable_state = err.Error()
		}

		// Update the Operation row of the database, based on the new state.
		if err := dbQueries.UpdateOperation(taskContext, dbOperation); err != nil {
			task.log.Error(err, "unable to update operation state", "operation", dbOperation.Operation_id)
			return true, err
		}
		task.log.Info("Updated Operation state for '" + dbOperation.Operation_id + "', state is: " + string(dbOperation.State))
	}

	return shouldRetry, err

}

func (task *processOperationEventTask) internalPerformTask(taskContext context.Context, dbQueries db.DatabaseQueries) (*db.Operation, bool, error) {
	log := log.FromContext(taskContext)

	eventClient := task.event.client

	// 1) Retrieve an up-to-date copy of the Operation CR that we want to process.
	operationCR := &operation.Operation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      task.event.request.Name,
			Namespace: task.event.request.Namespace,
		},
	}
	if err := eventClient.Get(taskContext, client.ObjectKeyFromObject(operationCR), operationCR); err != nil {
		if apierr.IsNotFound(err) {
			// If the resource doesn't exist, our job is done.
			log.V(sharedutil.LogLevel_Debug).Info("Received a request for an operation CR that doesn't exist: " + operationCR.Namespace + "/" + operationCR.Name)
			return nil, false, nil

		} else {
			// generic error
			return nil, true, fmt.Errorf("unable to retrieve operation CR: %v", err)
		}
	}

	// 2) Retrieve the database entry that corresponds to the Operation CR.
	dbOperation := db.Operation{
		Operation_id: operationCR.Spec.OperationID,
	}
	if err := dbQueries.GetOperationById(taskContext, &dbOperation); err != nil {

		if db.IsResultNotFoundError(err) {
			// no corresponding db operation, so no work to do
			log.V(sharedutil.LogLevel_Warn).Info("Received operation requested for operation DB entry that doesn't exist: " + dbOperation.Operation_id)
			return nil, false, nil
		} else {
			// some other generic error
			log.Error(err, "Unable to retrieve operation due to generic error: "+dbOperation.Operation_id)
			return nil, true, err
		}
	}

	// If the operation has already completed (e.g. we previously ran it), then just ignore it and return
	if dbOperation.State == db.OperationState_Completed || dbOperation.State == db.OperationState_Failed {
		log.V(sharedutil.LogLevel_Debug).Info("Skipping Operation with state of completed/failed", "operationId", dbOperation.Operation_id)
		return &dbOperation, false, nil
	}

	// 3) Find the Argo CD instance that is targeted by this operation.
	dbGitopsEngineInstance := &db.GitopsEngineInstance{
		Gitopsengineinstance_id: dbOperation.Instance_id,
	}
	if err := dbQueries.GetGitopsEngineInstanceById(taskContext, dbGitopsEngineInstance); err != nil {

		if db.IsResultNotFoundError(err) {
			// log as warning
			log.Error(err, "Receive operation on gitops engine instance that doesn't exist: "+dbOperation.Operation_id)

			// no corresponding db operation, so no work to do
			return &dbOperation, false, nil
		} else {
			// some other generic error
			log.Error(err, "Unexpected error on retrieving GitOpsEngineInstance in internalPerformTask")
			return &dbOperation, true, err
		}
	}

	// Sanity test: find the gitops engine cluster, by kube-system, and ensure that the
	// gitopsengineinstance matches the gitops engine cluster we are running on.
	kubeSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", Namespace: "kube-system"}}
	if err := eventClient.Get(taskContext, client.ObjectKeyFromObject(kubeSystemNamespace), kubeSystemNamespace); err != nil {
		log.Error(err, "SEVERE: Unable to retrieve kube-system namespace")
		return &dbOperation, true, fmt.Errorf("unable to retrieve kube-system namespace in internalPerformTask")
	}
	if thisCluster, err := dbutil.GetGitopsEngineClusterByKubeSystemNamespaceUID(taskContext, string(kubeSystemNamespace.UID), dbQueries, log); err != nil {
		log.Error(err, "Unable to retrieve GitOpsEngineCluster when processing Operation")
		return &dbOperation, true, err
	} else if thisCluster == nil {
		log.Error(err, "GitOpsEngineCluster could not be found when processing Operation")
		return &dbOperation, true, nil
	} else if thisCluster.Gitopsenginecluster_id != dbGitopsEngineInstance.EngineCluster_id {
		log.Error(nil, "SEVERE: The gitops engine cluster that the cluster-agent is running on did not match the operation's target argo cd instance id.")
		return &dbOperation, true, nil
	}

	// 4) Find the namespace for the targeted Argo CD instance
	argoCDNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbGitopsEngineInstance.Namespace_name,
			Namespace: dbGitopsEngineInstance.Namespace_name,
		},
	}
	if err := eventClient.Get(taskContext, client.ObjectKeyFromObject(argoCDNamespace), argoCDNamespace); err != nil {
		if db.IsResultNotFoundError(err) {
			log.Error(err, "Argo CD namespace doesn't exist")
			// no corresponding db operation, so no work to do
			return &dbOperation, false, nil
		} else {
			log.Error(err, "unexpected error on retrieve Argo CD namespace")
			// some other generic error
			return &dbOperation, true, err
		}
	}
	if string(argoCDNamespace.UID) != dbGitopsEngineInstance.Namespace_uid {
		log.Error(nil, "SEVERE: Engine instance did not match Argo CD namespace uid, while processing operation")
		return &dbOperation, false, nil
	}

	// Finally, call the corresponding method for processing the particular type of Operation.

	if dbOperation.Resource_type == db.OperationResourceType_Application {
		shouldRetry, err := processOperation_Application(taskContext, dbOperation, *operationCR, dbQueries, *argoCDNamespace, eventClient, log)

		if err != nil {
			log.Error(err, "error occurred on processing the application operation")
		}

		return &dbOperation, shouldRetry, err

	} else {
		log.Error(nil, "SEVERE: unrecognized resource type: "+dbOperation.Resource_type)
		return &dbOperation, false, nil
	}

}

// processOperation_Application handles an Operation that targets an Application. Returns true if the task should be retried (eg due to failure).
func processOperation_Application(ctx context.Context, dbOperation db.Operation, crOperation operation.Operation, dbQueries db.DatabaseQueries,
	argoCDNamespace corev1.Namespace, eventClient client.Client, log logr.Logger) (bool, error) {

	// Sanity check
	if dbOperation.Resource_id == "" {
		return true, fmt.Errorf("resource id was nil while processing operation: " + crOperation.Name)
	}

	dbApplication := &db.Application{
		Application_id: dbOperation.Resource_id,
	}

	if err := dbQueries.GetApplicationById(ctx, dbApplication); err != nil {

		if db.IsResultNotFoundError(err) {
			// The application db entry no longer exists, so delete the corresponding Application CR

			// Find the Application that has the corresponding databaseID label
			list := appv1.ApplicationList{}
			labelSelector := labels.NewSelector()
			req, err := labels.NewRequirement("databaseID", selection.Equals, []string{dbApplication.Application_id})
			if err != nil {
				log.Error(err, "invalid label requirement")
				return true, err
			}
			labelSelector = labelSelector.Add(*req)
			if err := eventClient.List(ctx, &list, &client.ListOptions{
				Namespace:     argoCDNamespace.Name,
				LabelSelector: labelSelector,
			}); err != nil {
				log.Error(err, "unable to complete Argo CD Application list")
				return true, err
			}

			if len(list.Items) > 1 {
				// Sanity test: should really only ever be 0 or 1
				log.Error(nil, "unexpected number of items in list", "length", len(list.Items))
			}

			var firstDeletionErr error
			for _, item := range list.Items {
				// Delete all Argo CD applications with the corresponding database label (but, there should be only one)
				err := controllers.DeleteArgoCDApplication(ctx, item, eventClient, log)
				if err != nil {
					log.Error(err, "error on deleting Argo CD Application: %v", item.Name)

					if firstDeletionErr == nil {
						firstDeletionErr = err
					}
				}
			}

			if firstDeletionErr != nil {
				log.Error(firstDeletionErr, "Deletion of at least one Argo CD application failed. First error was: %v", firstDeletionErr)
				return true, firstDeletionErr
			}

			return false, nil

		} else {
			log.Error(err, "An error occurred while attempting to retrieve Argo CD Application CR")
			return true, err
		}
	}

	app := &appv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbApplication.Name,
			Namespace: argoCDNamespace.Name,
		},
	}

	log = log.WithValues("app.Name", app.Name)

	if err := eventClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {

		if apierr.IsNotFound(err) {
			// The Application CR doesn't exist, so we need to create it

			if err := yaml.Unmarshal([]byte(dbApplication.Spec_field), app); err != nil {
				log.Error(err, "SEVERE: unable to unmarshal application spec field on creating Application CR: "+app.Name)
				// We return nil here, because there's likely nothing else that can be done to fix this.
				// Thus there is no need to keep retrying.
				return false, nil
			}

			app.ObjectMeta.Labels = map[string]string{"databaseID": dbApplication.Application_id}

			if err := eventClient.Create(ctx, app, &client.CreateOptions{}); err != nil {
				log.Error(err, "unable to create Argo CD Application CR: "+app.Name)
				// This may or may not be salvageable depending on the error; ultimately we should figure out which
				// error messages mean unsalvageable, and not wait for them.
				return true, err
			}

			log.Info("Created Argo CD Application CR: " + app.Name)

			return false, nil

		} else {
			log.Error(err, "Unexpected error when attempting to retrieve Argo CD Application CR")
			return false, err
		}

	}

	// The application CR exists, and the database entry exists, so check if there is any
	// difference between them.

	specFieldApp := &appv1.Application{}

	if err := yaml.Unmarshal([]byte(dbApplication.Spec_field), specFieldApp); err != nil {
		log.Error(err, "SEVERE: unable to unmarshal DB application spec field, on updating existing Application cR: "+app.Name)
		// We return nil here, because there's likely nothing else that can be done to fix this.
		// Thus there is no need to keep retrying.
		return false, nil
	}

	var specDiff string
	if !reflect.DeepEqual(specFieldApp.Spec.Source, app.Spec.Source) {
		specDiff = "spec.source fields differ"
	} else if !reflect.DeepEqual(specFieldApp.Spec.Destination, app.Spec.Destination) {
		specDiff = "spec.destination fields differ"
	} else if specFieldApp.Spec.Project != app.Spec.Project {
		specDiff = "spec project fields differ"
	} else if specFieldApp.Spec.SyncPolicy != app.Spec.SyncPolicy {
		specDiff = "sync policy fields differ"
	}

	if specDiff != "" {
		app.Spec.Destination = specFieldApp.Spec.Destination
		app.Spec.Source = specFieldApp.Spec.Source
		app.Spec.Project = specFieldApp.Spec.Project

		if err := eventClient.Update(ctx, app); err != nil {
			log.Error(err, "unable to update application after difference detected: "+app.Name)
			return false, err
		}

		log.Info("Updated Argo CD Application CR: " + app.Name + ", diff was: " + specDiff)

	} else {
		log.Info("no changes detected in application, so no update needed")
	}

	return false, nil
}
