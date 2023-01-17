package eventloop

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	argosharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/utils"
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
//   - The user updated a field in a GitOpsDeployment in the user's namespace
//   - The managed-gitops backend updated corresponding field in the Application database row
//   - Next: an Operation was created to inform the cluster-agent component of the Application row changed
//   - We are here: now the Operation controller of cluster-agent has been informed of the Operation.
//   - We need to: Look at the Operation, determine what database entry changed, and ensure that Argo CD
//     is reconciled to the contents of the database entry.
//
// The overall workflow of Operations can be found in the architecture doc:
// https://docs.google.com/document/d/1e1UwCbwK-Ew5ODWedqp_jZmhiZzYWaxEvIL-tqebMzo/edit#heading=h.9vyguee8vhow
type OperationEventLoop struct {
	eventLoopInputChannel chan operationEventLoopEvent
}

// Functions that return a boolean indicating whether the request should be retried, should use these constants
// to improve code readability, rather than just using 'true'/'false'
const (
	shouldRetryTrue  = true
	shouldRetryFalse = false
)

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

	log := log.FromContext(ctx).WithName("operation-event-loop")

	taskRetryLoop := sharedutil.NewTaskRetryLoop("cluster-agent")

	log.Info("controllerEventLoopRouter started")

	dbQueries, err := db.NewSharedProductionPostgresDBQueries(true)
	if err != nil {
		log.Error(err, "SEVERE: Controller event loop exited before startup.")
		return
	}

	credentialService := utils.NewCredentialService(nil, false)

	for {
		newEvent := <-input

		// Generate the map key (which controls task concurrency) by retrieving the Operation from the database
		// that corresponds to the Operation custom resource from the event.
		var mapKey string
		_, err := sharedutil.CatchPanic(func() error {

			dbOperation, err := getDBOperationForEvent(ctx, newEvent, dbQueries, log)
			if err != nil {
				return err
			}

			if dbOperation == nil {
				// If information could not be retrieved from the database about the operation, then just skip processing the event
				return nil
			}

			// If multiple operations exist that target the same Application/SyncOperation, we should only process those
			// operations one at a time (i.e. non-concurrently)
			mapKey = dbOperation.Instance_id + "-" + string(dbOperation.Resource_type) + "-" + dbOperation.Resource_id

			return nil
		})

		if err != nil {
			log.Error(err, "unable to acquire information for operation from DB")
			continue
		}

		if mapKey == "" {
			log.Info("mapkey was nil for request, continuing.")
			continue
		}

		// Queue a new task in the task retry loop for our event.
		task := &processOperationEventTask{
			event: operationEventLoopEvent{
				request: newEvent.request,
				client:  newEvent.client,
			},
			log:               log,
			credentialService: credentialService,
			syncFuncs:         defaultSyncFuncs(),
		}
		taskRetryLoop.AddTaskIfNotPresent(mapKey, task, sharedutil.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true})

	}

}

func getDBOperationForEvent(ctx context.Context, newEvent operationEventLoopEvent, dbQueries db.DatabaseQueries, log logr.Logger) (*db.Operation, error) {

	backoff := &sharedutil.ExponentialBackoff{
		Factor: 2,
		Min:    time.Duration(time.Millisecond * 200),
		Max:    time.Duration(time.Second * 15),
		Jitter: true,
	}

	var res *db.Operation

	outerErr := sharedutil.RunTaskUntilTrue(ctx, backoff, "get DBOperation", log, func() (bool, error) {

		const (
			taskCompleteTrue  = true
			taskCompleteFalse = false
		)

		// 1) Retrieve an up-to-date copy of the Operation CR that we want to process.
		operationCR := &operation.Operation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      newEvent.request.Name,
				Namespace: newEvent.request.Namespace,
			},
		}

		if err := newEvent.client.Get(ctx, client.ObjectKeyFromObject(operationCR), operationCR); err != nil {
			if apierr.IsNotFound(err) {
				log.V(sharedutil.LogLevel_Debug).Info("Skipping a request for an operation DB entry that doesn't exist: " + operationCR.Namespace + "/" + operationCR.Name)
				return taskCompleteTrue, nil

			} else {
				// generic error
				log.Error(err, err.Error())
				return taskCompleteFalse, err
			}
		}

		// 2) Retrieve the corresponding database ID
		dbOperation := db.Operation{
			Operation_id: operationCR.Spec.OperationID,
		}
		if err := dbQueries.GetOperationById(ctx, &dbOperation); err != nil {

			if db.IsResultNotFoundError(err) {
				return taskCompleteTrue, nil
			} else {
				// some other generic error
				log.Error(err, "Unable to retrieve operation due to generic error: "+dbOperation.Operation_id)
				return taskCompleteFalse, err
			}
		}

		// Sanity test: all the fields should have non-empty values, at this step
		if dbOperation.Instance_id == "" && dbOperation.Resource_id == "" && string(dbOperation.Resource_type) == "" {

			log.Error(nil, "SEVERE: at least one of the expected operation's fields was empty, so could not process in cluster agent.")
			return taskCompleteTrue, nil
		}

		res = &dbOperation

		return taskCompleteTrue, nil
	})

	return res, outerErr
}

// processOperationEventTask takes as input an Operation resource event, and processes it based on the contents of that event.
type processOperationEventTask struct {
	event             operationEventLoopEvent
	log               logr.Logger
	credentialService *utils.CredentialService
	syncFuncs         *syncFuncs
}

// PerformTask takes as input an Operation resource event, and processes it based on the contents of that event.
//
// Returns bool (true if the task should be retried, for example because it failed, false otherwise),
// and error (an error to log on failure).
//
// NOTE: 'error' value does not affect whether the task will be retried, this error is only used for
// error reporting.
func (task *processOperationEventTask) PerformTask(taskContext context.Context) (bool, error) {
	dbQueries, err := db.NewSharedProductionPostgresDBQueries(false)
	if err != nil {
		return shouldRetryTrue, fmt.Errorf("unable to instantiate database in operation controller loop: %v", err)
	}
	defer dbQueries.CloseDatabase()

	// Process the event (this is where most of the work is done)
	dbOperation, shouldRetry, err := task.internalPerformTask(taskContext, dbQueries)

	if dbOperation != nil {

		// After the event is processed, update the status in the database

		// Don't update the status of operations that have previously completed.
		if dbOperation.State == db.OperationState_Completed || dbOperation.State == db.OperationState_Failed {
			return shouldRetryFalse, err
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
			dbOperation.Human_readable_state = db.TruncateVarchar(err.Error(), db.OperationHumanReadableStateLength)
		}

		// Update the Operation row of the database, based on the new state.
		if err := dbQueries.UpdateOperation(taskContext, dbOperation); err != nil {
			task.log.Error(err, "unable to update operation state", "operationID", dbOperation.Operation_id)
			return shouldRetryTrue, err
		}

		task.log.Info("Updated Operation state", "operationID", dbOperation.Operation_id, "new operationState", string(dbOperation.State))
	}

	return shouldRetry, err

}

func (task *processOperationEventTask) internalPerformTask(taskContext context.Context, dbQueries db.DatabaseQueries) (*db.Operation, bool, error) {

	eventClient := task.event.client

	log := task.log.WithValues("namespace", task.event.request.Namespace, "name", task.event.request.Name)

	// 1) Retrieve an up-to-date copy of the Operation CR that we want to process.
	operationCR := &operation.Operation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      task.event.request.Name,
			Namespace: task.event.request.Namespace,
		},
	}
	if err := eventClient.Get(taskContext, client.ObjectKeyFromObject(operationCR), operationCR); err != nil {
		if apierr.IsNotFound(err) {
			// If the resource doesn't exist, so our job is done.
			log.V(sharedutil.LogLevel_Debug).Info("Received a K8s request for an Operation CR that doesn't exist")

			return nil, shouldRetryFalse, nil

		} else {
			// generic error
			return nil, shouldRetryTrue, fmt.Errorf("unable to retrieve operation CR: %v", err)
		}
	}

	log = log.WithValues("operationID", operationCR.Spec.OperationID)

	// 2) Retrieve the database entry that corresponds to the Operation CR.
	dbOperation := db.Operation{
		Operation_id: operationCR.Spec.OperationID,
	}
	if err := dbQueries.GetOperationById(taskContext, &dbOperation); err != nil {

		if db.IsResultNotFoundError(err) {
			// no corresponding db operation, so no work to do
			log.V(sharedutil.LogLevel_Warn).Info("Received a K8 request for an Operation resource, but the referenced DB Operation Row doesn't exist")
			return nil, shouldRetryFalse, nil
		} else {
			// some other generic error
			log.Error(err, "Unable to retrieve operation due to generic error")
			return nil, shouldRetryTrue, err
		}
	}

	// If the operation has already completed (e.g. we previously ran it), then just ignore it and return
	if dbOperation.State == db.OperationState_Completed || dbOperation.State == db.OperationState_Failed {
		log.V(sharedutil.LogLevel_Debug).Info("Skipping Operation with state of Completed/Failed")
		return &dbOperation, shouldRetryFalse, nil
	}

	// If the operation is in waiting state, update it to in-progress before we start processing it.
	if dbOperation.State == db.OperationState_Waiting {
		dbOperation.State = db.OperationState_In_Progress

		if err := dbQueries.UpdateOperation(taskContext, &dbOperation); err != nil {
			log.Error(err, "Unable to update Operation state")
			return nil, shouldRetryTrue, fmt.Errorf("unable to update Operation, err: %v", err)
		}
		log.V(sharedutil.LogLevel_Debug).Info("Updated OperationState to InProgress")

	}

	// 3) Find the Argo CD instance that is targeted by this operation.
	dbGitopsEngineInstance := &db.GitopsEngineInstance{
		Gitopsengineinstance_id: dbOperation.Instance_id,
	}
	if err := dbQueries.GetGitopsEngineInstanceById(taskContext, dbGitopsEngineInstance); err != nil {

		if db.IsResultNotFoundError(err) {
			// log as warning
			log.Error(err, "Receive operation on gitops engine instance that doesn't exist")

			// no corresponding db operation, so no work to do
			return &dbOperation, shouldRetryFalse, nil
		} else {
			// some other generic error
			log.Error(err, "Unexpected error on retrieving GitOpsEngineInstance in internalPerformTask")
			return &dbOperation, shouldRetryTrue, err
		}
	}

	// Sanity test: find the gitops engine cluster, by kube-system, and ensure that the
	// gitopsengineinstance matches the gitops engine cluster we are running on.
	kubeSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	if err := eventClient.Get(taskContext, client.ObjectKeyFromObject(kubeSystemNamespace), kubeSystemNamespace); err != nil {
		log.Error(err, "Unable to retrieve kube-system namespace")
		return &dbOperation, shouldRetryTrue, fmt.Errorf("unable to retrieve kube-system namespace in internalPerformTask")
	}
	if thisCluster, err := dbutil.GetGitopsEngineClusterByKubeSystemNamespaceUID(taskContext, string(kubeSystemNamespace.UID), dbQueries, log); err != nil {
		log.Error(err, "Unable to retrieve GitOpsEngineCluster when processing Operation")
		return &dbOperation, shouldRetryTrue, err
	} else if thisCluster == nil {
		log.Error(err, "GitOpsEngineCluster could not be found when processing Operation")
		return &dbOperation, shouldRetryTrue, nil
	} else if thisCluster.Gitopsenginecluster_id != dbGitopsEngineInstance.EngineCluster_id {
		log.Error(nil, "SEVERE: The gitops engine cluster that the cluster-agent is running on did not match the operation's target argo cd instance id.")
		return &dbOperation, shouldRetryTrue, nil
	}

	// 4) Find the namespace for the targeted Argo CD instance
	argoCDNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: dbGitopsEngineInstance.Namespace_name,
		},
	}
	if err := eventClient.Get(taskContext, client.ObjectKeyFromObject(argoCDNamespace), argoCDNamespace); err != nil {
		if apierr.IsNotFound(err) {
			log.Error(err, "Argo CD namespace doesn't exist", "argoCDNamespaceName", argoCDNamespace.Name)
			// Return retry as true, as it's possible it may exist in the future.
			return &dbOperation, shouldRetryTrue, fmt.Errorf("argo CD namespace doesn't exist: %s", argoCDNamespace.Name)
		} else {
			log.Error(err, "unexpected error on retrieve Argo CD namespace")
			// some other generic error
			return &dbOperation, shouldRetryTrue, err
		}
	}
	if string(argoCDNamespace.UID) != dbGitopsEngineInstance.Namespace_uid {
		log.Error(nil, "SEVERE: Engine instance did not match Argo CD namespace uid, while processing operation")
		return &dbOperation, shouldRetryFalse, nil
	}

	operationConfigParams := operationConfig{
		dbQueries:         dbQueries,
		argoCDNamespace:   *argoCDNamespace,
		eventClient:       eventClient,
		credentialService: task.credentialService,
		log:               log,
		syncFuncs:         task.syncFuncs,
	}

	// 5) Finally, call the corresponding method for processing the particular type of Operation.

	if dbOperation.Resource_type == db.OperationResourceType_Application {
		shouldRetry, err := processOperation_Application(taskContext, dbOperation, *operationCR, operationConfigParams)

		if err != nil {
			log.Error(err, "error occurred on processing the application operation")
		}

		return &dbOperation, shouldRetry, err

	} else if dbOperation.Resource_type == db.OperationResourceType_ManagedEnvironment {
		shouldRetry, err := processOperation_ManagedEnvironment(taskContext, dbOperation, *operationCR, operationConfigParams)

		if err != nil {
			log.Error(err, "error occurred on processing the application operation")
		}

		return &dbOperation, shouldRetry, err

	} else if dbOperation.Resource_type == db.OperationResourceType_RepositoryCredentials {
		shouldRetry, err := processOperation_RepositoryCredentials(taskContext, dbOperation, *operationCR, operationConfigParams)

		if err != nil {
			log.Error(err, "error occurred on processing the repository credentials operation")
		}

		return &dbOperation, shouldRetry, err

	} else if dbOperation.Resource_type == db.OperationResourceType_SyncOperation {

		// Process a SyncOperation event
		shouldRetry, err := processOperation_SyncOperation(taskContext, dbOperation, *operationCR, operationConfigParams)

		if err != nil {
			log.Error(err, "error occurred on processing the sync application operation")
		}

		return &dbOperation, shouldRetry, err

	} else {
		log.Error(nil, "SEVERE: unrecognized resource type: "+string(dbOperation.Resource_type))
		return &dbOperation, shouldRetryFalse, nil
	}

}

// operationConfig contains generic parameters used by the operation event loop, but that are not specified to any
// particular operation type.
//
// Because operationConfig only provides general information, it is passed between many different operation event loop functions.
type operationConfig struct {
	// dbQueries is the DatabaseQuery object in use by the event loop
	dbQueries db.DatabaseQueries

	// argoCDNamespace is the namespace of the Argo CD instance we are targeting
	argoCDNamespace corev1.Namespace

	// eventClient is a K8s client object that can be used to interact with the cluster that Argo CD is on
	eventClient client.Client

	// credentialService is used to retrieve login credentials for Argo CD
	credentialService *utils.CredentialService

	// log is the current logger in used
	log logr.Logger

	// syncFuncs provide functions to sync/terminate an operation
	syncFuncs *syncFuncs
}

// Process a SyncOperation database entry, that was pointed to by an Operation CR.
// returns shouldRetry, error
func processOperation_SyncOperation(ctx context.Context, dbOperation db.Operation, crOperation operation.Operation,
	opConfig operationConfig) (bool, error) {

	log := opConfig.log
	dbQueries := opConfig.dbQueries

	// Sanity checks
	if dbOperation.Resource_id == "" {
		return shouldRetryFalse, fmt.Errorf("resource id was nil while processing operation: " + crOperation.Name)
	}

	// 1) Retrieve the SyncOperation DB entry pointed to by the Operation DB entry
	dbSyncOperation := &db.SyncOperation{
		SyncOperation_id: dbOperation.Resource_id,
	}
	if err := dbQueries.GetSyncOperationById(ctx, dbSyncOperation); err != nil {

		if !db.IsResultNotFoundError(err) {
			// On generic error, we should retry.
			log.Error(err, "DB error occurred on retriving SyncOperation: "+dbSyncOperation.SyncOperation_id)
			return shouldRetryTrue, err
		} else {
			// On db row not found, just ignore it and move on.
			log.V(sharedutil.LogLevel_Debug).Info("SyncOperation '" + dbSyncOperation.SyncOperation_id + "' DB entry was no longer available.")
			return shouldRetryFalse, err
		}
	}

	// 2) Retrieve the Application DB entry pointed to by the SyncOperation DB entry
	dbApplication := db.Application{
		Application_id: dbSyncOperation.Application_id,
	}
	if err := dbQueries.GetApplicationById(ctx, &dbApplication); err != nil {

		if db.IsResultNotFoundError(err) {
			log.Error(err, "Unable to retrieve application ID in SyncOperation table: "+dbApplication.Application_id)
			return shouldRetryFalse, err
		} else {
			// On generic error, return true so the operation is retried.
			log.Error(err, "Error occurred on retrieving application ID in SyncOperation table")
			return shouldRetryTrue, err
		}
	}

	// 3) Process the event, based on whether the SyncOperation is requesting an app sync, or a terminate.
	if dbSyncOperation.DesiredState == db.SyncOperation_DesiredState_Running {

		return runAppSync(ctx, dbOperation, *dbSyncOperation, &dbApplication, opConfig)

	} else if dbSyncOperation.DesiredState == db.SyncOperation_DesiredState_Terminated {

		return terminateExistingOperation(ctx, &dbApplication, opConfig)

	} else {
		log.Error(nil, "SEVERE: unexpected desired state in SyncOperation DB entry")
		return shouldRetryFalse, nil
	}
}

// returns shouldRetry, error
func terminateExistingOperation(ctx context.Context, dbApplication *db.Application, opConfig operationConfig) (bool, error) {

	isRunning, err := isOperationRunning(ctx, opConfig.eventClient, dbApplication.Name, opConfig.argoCDNamespace.Name)
	if err != nil {
		opConfig.log.Error(err, "unable to determine if an Operation is running for Application: "+dbApplication.Name)
		return shouldRetryTrue, err
	}

	// nothing to terminate if no sync operation is in progress
	if !isRunning {
		return shouldRetryFalse, nil
	}

	if err := opConfig.syncFuncs.terminateOperation(ctx, dbApplication.Name, opConfig.argoCDNamespace, opConfig.credentialService,
		opConfig.eventClient, time.Duration(5*time.Minute), opConfig.log); err != nil {

		opConfig.log.Error(err, "unable to terminate operation: "+dbApplication.Name)
		return shouldRetryTrue, err
	}

	opConfig.log.Info("Successfully terminated operation for application '" + dbApplication.Name + "'")

	return shouldRetryFalse, nil

}

func isOperationRunning(ctx context.Context, k8sClient client.Client, appName, appNS string) (bool, error) {
	app := &appv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: appNS,
		},
	}

	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {
		return false, err
	}

	if app.Operation == nil || app.Status.OperationState == nil {
		return false, nil
	}

	return true, nil
}

// syncFuncs is a wrapper over sync and terminate functions and is used in unit testing different sync scenarios
type syncFuncs struct {
	appSync            func(context.Context, string, string, string, client.Client, *utils.CredentialService, bool) error
	terminateOperation func(context.Context, string, corev1.Namespace, *utils.CredentialService, client.Client, time.Duration, logr.Logger) error
}

func defaultSyncFuncs() *syncFuncs {
	return &syncFuncs{
		appSync:            utils.AppSync,
		terminateOperation: utils.TerminateOperation,
	}
}

// returns shouldRetry, error
func runAppSync(ctx context.Context, dbOperation db.Operation, dbSyncOperation db.SyncOperation,
	dbApplication *db.Application, opConfig operationConfig) (bool, error) {

	log := opConfig.log

	completeChan := make(chan bool)

	var err error

	cancellableCtx, cancelFunc := context.WithCancel(ctx)

	defer cancelFunc()

	// Start the AppSync operation in a separate thread.
	go func() {
		err = opConfig.syncFuncs.appSync(cancellableCtx, dbApplication.Name, dbSyncOperation.Revision, opConfig.argoCDNamespace.Name, opConfig.eventClient,
			opConfig.credentialService, false)

		var failed bool
		if err != nil {
			log.Error(err, "app sync failed on application '"+dbApplication.Name+"'")
			failed = true
		}

		// signal the AppSync has exited
		completeChan <- failed
	}()

	var shouldRetry bool

	backoff := sharedutil.ExponentialBackoff{Factor: 1.5, Min: 2 * time.Second, Max: 15 * time.Second, Jitter: true}

outer:
	for {

		/// 1) Retrieve the database entry for the operation
		dbSyncOperation := &db.SyncOperation{
			SyncOperation_id: dbOperation.Resource_id,
		}
		if innerErr := opConfig.dbQueries.GetSyncOperationById(ctx, dbSyncOperation); innerErr != nil {

			// If the DB entry no longer exists, then no more work is to be done
			if db.IsResultNotFoundError(innerErr) {
				log.V(sharedutil.LogLevel_Debug).Info("SyncOperation '" + dbSyncOperation.SyncOperation_id + "' DB entry was no longer available, during AppSync.")
				shouldRetry = shouldRetryFalse
				err = nil
				break outer
			} else {
				log.Error(innerErr, "Unable to retrieve SyncOperation '"+dbSyncOperation.SyncOperation_id+"', during AppSync.")
			}
		}

		// 2) If the user cancelled the SyncOperation (by altering the desired state field), then we should stop waiting
		// for the appsync to complete, here.
		if dbSyncOperation.DesiredState != db.SyncOperation_DesiredState_Running {
			log.V(sharedutil.LogLevel_Debug).Info("SyncOperation '" + dbSyncOperation.SyncOperation_id + "' no longer had desired running state.")
			shouldRetry = shouldRetryFalse
			err = nil
			break outer
		}

		// 3) Otherwise, continue waiting the AppSync operation to complete.
		select {
		case shouldRetry = <-completeChan:
			break outer
		default:
			backoff.DelayOnFail(ctx)
		}
	}

	return shouldRetry, err
}

// processOperation_ManagedEnvironment handles an Operation that targets an Application.
// Returns true if the task should be retried (eg due to failure), false otherwise.
func processOperation_ManagedEnvironment(ctx context.Context, dbOperation db.Operation, crOperation operation.Operation,
	opConfig operationConfig) (bool, error) {

	// The only operation we currently support for managed environment is deletion (creation is handled by Application operations).
	// Thus, we expect the ManagedEnvironment database entry here to not be found.

	// 1) Make sure the managed environment db entry DOESN'T exist (see above)
	{
		managedEnv := &db.ManagedEnvironment{
			Managedenvironment_id: dbOperation.Resource_id, // managed env id referencing managed env row
		}
		if err := opConfig.dbQueries.GetManagedEnvironmentById(ctx, managedEnv); err != nil {
			if !db.IsResultNotFoundError(err) {
				return shouldRetryTrue, fmt.Errorf("an unexpected error occcurred on retrieving managed env: %v", err)
			}
		} else {
			// The database entry still exists, so return an error
			return shouldRetryFalse, fmt.Errorf("managed environment still exists in the database")
		}
	}

	// 2) Delete the Argo CD cluster secret corresponding to the managed environment.
	// - the cluster secret has a specific name format, so it is easy to locate by the name.

	// We have confirmed the database entry doesn't exist, now locate the Argo CD Cluster Secret that
	// corresponds to the managed environment.
	expectedSecretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: dbOperation.Resource_id})

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      expectedSecretName,
			Namespace: opConfig.argoCDNamespace.Name,
		},
	}
	if err := opConfig.eventClient.Delete(ctx, secret); err != nil {
		if apierr.IsNotFound(err) {
			// The cluster secret doesn't exist, so our work is done.
			return shouldRetryFalse, nil
		} else {
			return shouldRetryTrue, fmt.Errorf("unable to retrieve Argo CD Cluster Secret '%s' in '%s': %v", secret.Name, secret.Namespace, err)
		}
	}
	sharedutil.LogAPIResourceChangeEvent(secret.Namespace, secret.Name, secret, sharedutil.ResourceDeleted, opConfig.log)

	// Argo CD cluster secret corresponding to environment is successfully deleted.
	return shouldRetryFalse, nil
}

const (
	// ArgoCDDefaultDestinationInCluster is 'in-cluster' which is the spec destination value that Argo CD recognizes
	// as indicating that Argo CD should deploy to the local cluster (the cluster that Argo CD is installed on).
	ArgoCDDefaultDestinationInCluster = "in-cluster"
	// #nosec G101
	ArgoCDSecretTypeKey                 = "argocd.argoproj.io/secret-type"
	ArgoCDSecretTypeValue_ClusterSecret = "cluster"
)

// processOperation_Application handles an Operation that targets an Application.
// Returns true if the task should be retried (eg due to failure), false otherwise.
func processOperation_Application(ctx context.Context, dbOperation db.Operation, crOperation operation.Operation, opConfig operationConfig) (bool, error) {

	// Sanity check
	if dbOperation.Resource_id == "" {
		return shouldRetryTrue, fmt.Errorf("resource id was nil while processing operation: " + crOperation.Name)
	}

	dbApplication := &db.Application{
		Application_id: dbOperation.Resource_id,
	}

	log := opConfig.log.WithValues("applicationID", dbApplication.Application_id)

	if err := opConfig.dbQueries.GetApplicationById(ctx, dbApplication); err != nil {

		if db.IsResultNotFoundError(err) {
			// The application db entry no longer exists, so delete the corresponding Application CR

			// Find the Application that has the corresponding databaseID label
			list := appv1.ApplicationList{}
			labelSelector := labels.NewSelector()
			req, err := labels.NewRequirement(controllers.ArgoCDApplicationDatabaseIDLabel, selection.Equals, []string{dbApplication.Application_id})
			if err != nil {
				log.Error(err, "SEVERE: invalid label requirement")
				return shouldRetryFalse, err
			}
			labelSelector = labelSelector.Add(*req)
			if err := opConfig.eventClient.List(ctx, &list, &client.ListOptions{
				Namespace:     opConfig.argoCDNamespace.Name,
				LabelSelector: labelSelector,
			}); err != nil {
				log.Error(err, "unable to complete Argo CD Application list")
				return shouldRetryTrue, err
			}

			if len(list.Items) > 1 {
				// Sanity test: should really only ever be 0 or 1
				log.Error(nil, "SEVERE: unexpected number of items in list", "length", len(list.Items))
			}

			var firstDeletionErr error
			for _, item := range list.Items {

				log := log.WithValues("argoCDApplicationName", item.Name, "argoCDApplicationNamespace", item.Namespace)

				log.Info("Deleting Argo CD Application that is no longer (or not) defined in the Application table.")

				// Delete all Argo CD applications with the corresponding database label (but, there should be only one)
				err := controllers.DeleteArgoCDApplication(ctx, item, opConfig.eventClient, log)
				if err != nil {
					log.Error(err, "error on deleting Argo CD Application")

					if firstDeletionErr == nil {
						firstDeletionErr = err
					}
				}
			}

			if firstDeletionErr != nil {
				log.Error(firstDeletionErr, "Deletion of at least one Argo CD application failed.", "firstError", firstDeletionErr)
				return shouldRetryTrue, firstDeletionErr
			}

			// success
			return shouldRetryFalse, nil

		} else {
			log.Error(err, "Unable to retrieve database Application row from database")
			return shouldRetryTrue, err
		}
	}

	log = log.WithValues("argoCDApplicationName", dbApplication.Name)

	app := &appv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbApplication.Name,
			Namespace: opConfig.argoCDNamespace.Name,
		},
	}

	// Retrieve the Argo CD Application from the namespace
	if err := opConfig.eventClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {

		if apierr.IsNotFound(err) {
			// The Application CR doesn't exist, so we need to create it

			// Copy the contents of the Spec_field database column, into the Spec field of the Argo CD Application CR
			if err := yaml.Unmarshal([]byte(dbApplication.Spec_field), app); err != nil {
				log.Error(err, "SEVERE: unable to unmarshal application spec field on creating Application CR.")
				// We return nil here, because there's likely nothing else that can be done to fix this.
				// Thus there is no need to keep retrying.
				return shouldRetryFalse, nil
			}

			// Add databaseID label
			app.ObjectMeta.Labels = map[string]string{controllers.ArgoCDApplicationDatabaseIDLabel: dbApplication.Application_id}

			// Before we create the application, make sure that the managed environment exists that the application points to
			if app.Spec.Destination.Name != argosharedutil.ArgoCDDefaultDestinationInCluster {
				if err := ensureManagedEnvironmentExists(ctx, *dbApplication, opConfig); err != nil {
					log.Error(err, "unable to ensure that managed environment exists")
					return shouldRetryTrue, err
				}
			}
			if err := opConfig.eventClient.Create(ctx, app, &client.CreateOptions{}); err != nil {
				log.Error(err, "Unable to create Argo CD Application CR")
				// This may or may not be salvageable depending on the error; ultimately we should figure out which
				// error messages mean unsalvageable, and not wait for them.
				return shouldRetryTrue, err
			}
			sharedutil.LogAPIResourceChangeEvent(app.Namespace, app.Name, app, sharedutil.ResourceCreated, log)

			// Success
			return shouldRetryFalse, nil

		} else {
			// If another error occurred, retry.
			log.Error(err, "Unexpected error when attempting to retrieve Argo CD Application CR")
			return shouldRetryTrue, err
		}

	}

	// The application CR exists, and the database entry exists, so check if there is any
	// difference between them.

	// Before we create the application, make sure that the managed environment that the application points to exists

	specDiff, err := controllers.CompareApplication(*app, *dbApplication, log)
	if err != nil {
		log.Error(err, "unable to compare Argo CD Application with DB row")
		return shouldRetryFalse, err
	}

	if specDiff != "" {
		specFieldApp := &appv1.Application{}

		if err := yaml.Unmarshal([]byte(dbApplication.Spec_field), specFieldApp); err != nil {
			log.Error(err, "SEVERE: unable to unmarshal DB application spec field, on updating existing Application cR: "+app.Name)
			// We return nil here, with no retry, because there's likely nothing else that can be done to fix this.
			// Thus there is no need to keep retrying.
			return shouldRetryFalse, nil
		}

		app.Spec.Destination = specFieldApp.Spec.Destination
		app.Spec.Source = specFieldApp.Spec.Source
		app.Spec.Project = specFieldApp.Spec.Project
		app.Spec.SyncPolicy = specFieldApp.Spec.SyncPolicy
		if err := opConfig.eventClient.Update(ctx, app); err != nil {
			log.Error(err, "unable to update application after difference detected.")
			// Retry if we were unable to update the Application, for example due to a conflict
			return shouldRetryTrue, err
		}
		sharedutil.LogAPIResourceChangeEvent(app.Namespace, app.Name, app, sharedutil.ResourceModified, log)

		log.Info("Updated Argo CD Application CR", "specDiff", specDiff)

	} else {
		log.Info("no changes detected in application, so no update needed")
	}

	// Finally, ensure that the managed-environment secret is still up to date
	if app.Spec.Destination.Name != argosharedutil.ArgoCDDefaultDestinationInCluster {
		if err := ensureManagedEnvironmentExists(ctx, *dbApplication, opConfig); err != nil {
			log.Error(err, "unable to ensure that managed environment exists")
			return shouldRetryTrue, err
		}
	}

	return shouldRetryFalse, nil
}

// ensureManagedEnvironmentExists ensures that the managed environment described by 'application' is defined as an Argo CD
// cluster secret, in the Argo CD namespace.
func ensureManagedEnvironmentExists(ctx context.Context, application db.Application, opConfig operationConfig) error {

	if application.Managed_environment_id == "" {
		// No work to do
		return nil
	}

	expectedSecret, shouldDeleteSecret, err := generateExpectedClusterSecret(ctx, application, opConfig)
	if err != nil {
		return fmt.Errorf("unable to generate expected cluster secret: %v", err)
	}

	// If we detected that the managed environment row was deleted, ensure the secret is deleted.
	if shouldDeleteSecret {
		secretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: application.Managed_environment_id})
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: opConfig.argoCDNamespace.Name,
			},
		}

		if err := opConfig.eventClient.Delete(ctx, secret); err != nil {
			if apierr.IsNotFound(err) {
				// The secret doesn't exist, so no more work to do.
				return nil
			} else {
				return fmt.Errorf("unable to delete secret of deleted managed environment: %v", err)
			}
		}
		sharedutil.LogAPIResourceChangeEvent(secret.Namespace, secret.Name, secret, sharedutil.ResourceDeleted, opConfig.log)

		return nil
	}

	// If the secret is otherwise empty, no work is required
	if expectedSecret.Name == "" {
		return nil
	}

	managedEnv := &db.ManagedEnvironment{
		Managedenvironment_id: application.Managed_environment_id,
	}

	if err := opConfig.dbQueries.GetManagedEnvironmentById(ctx, managedEnv); err != nil {
		if db.IsResultNotFoundError(err) {
			// Application refers to a managed environment that doesn't exist: no more work to do.
			// Return true to indicate that the managed environment cluster secret should be deleted.
			return nil
		} else {
			return fmt.Errorf("unable to get managed environment '%s': %v", managedEnv.Managedenvironment_id, err)
		}
	}
	clusterCredentials := &db.ClusterCredentials{
		Clustercredentials_cred_id: managedEnv.Clustercredentials_id,
	}
	if err := opConfig.dbQueries.GetClusterCredentialsById(ctx, clusterCredentials); err != nil {
		if db.IsResultNotFoundError(err) {
			// Managed environment refers to cluster credentials which no longer exist: no more work to do.
			// Return true to indicate that the managed environment cluster secret should be deleted.
			return nil
		} else {
			return fmt.Errorf("unable to get cluster credentials '%s': %v", clusterCredentials.Clustercredentials_cred_id, err)
		}
	}

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      expectedSecret.Name,
			Namespace: expectedSecret.Namespace,
		},
	}
	if err := opConfig.eventClient.Get(ctx, client.ObjectKeyFromObject(existingSecret), existingSecret); err != nil {
		if !apierr.IsNotFound(err) {
			return fmt.Errorf("unable to retrieve existing Argo CD Cluster secret '%s' in '%s'", existingSecret.Name, existingSecret.Namespace)
		}

		// A) Secret doesn't exist, so create it
		if err := opConfig.eventClient.Create(ctx, &expectedSecret); err != nil {
			return fmt.Errorf("unable to create expected Argo CD Cluster secret '%s' in '%s'", expectedSecret.Name, expectedSecret.Namespace)
		}
		sharedutil.LogAPIResourceChangeEvent(expectedSecret.Namespace, expectedSecret.Name, expectedSecret, sharedutil.ResourceCreated, opConfig.log)

		return nil
	}

	// B) Secret already exists, so compare
	if reflect.DeepEqual(existingSecret.Data, expectedSecret.Data) {
		// No work required, so exit.
		return nil
	}
	existingSecret.Data = expectedSecret.Data

	// C) Secret exists, but is different from what is expected, so update it.
	if err := opConfig.eventClient.Update(ctx, existingSecret); err != nil {
		return fmt.Errorf("unable to update existing secret '%s' in '%s'", existingSecret.Name, existingSecret.Namespace)
	}
	sharedutil.LogAPIResourceChangeEvent(existingSecret.Namespace, existingSecret.Name, existingSecret, sharedutil.ResourceModified, opConfig.log)

	return nil

}

// generateExpectedClusterSecret generates (but does apply) an Argo CD cluster secret for the environment of the application.
// returns:
// - argo cd cluster secret based on managed environment
// - bool: true if secret should be deleted false otherwise
// - error
func generateExpectedClusterSecret(ctx context.Context, application db.Application, opConfig operationConfig) (corev1.Secret, bool, error) {

	const (
		deleteSecret_true  = true
		deleteSecret_false = false
	)

	managedEnv := &db.ManagedEnvironment{
		Managedenvironment_id: application.Managed_environment_id,
	}

	if err := opConfig.dbQueries.GetManagedEnvironmentById(ctx, managedEnv); err != nil {
		if db.IsResultNotFoundError(err) {
			// Application refers to a managed environment that doesn't exist: no more work to do.
			// Return true to indicate that the managed environment cluster secret should be deleted.
			return corev1.Secret{}, deleteSecret_true, nil
		} else {
			return corev1.Secret{}, deleteSecret_false,
				fmt.Errorf("unable to get managed environment '%s': %v", managedEnv.Managedenvironment_id, err)
		}
	}

	clusterCredentials := &db.ClusterCredentials{
		Clustercredentials_cred_id: managedEnv.Clustercredentials_id,
	}

	if err := opConfig.dbQueries.GetClusterCredentialsById(ctx, clusterCredentials); err != nil {
		if db.IsResultNotFoundError(err) {
			// Managed environment refers to cluster credentials which no longer exist: no more work to do.
			// Return true to indicate that the managed environment cluster secret should be deleted.
			return corev1.Secret{}, deleteSecret_true, nil
		} else {
			return corev1.Secret{}, deleteSecret_false,
				fmt.Errorf("unable to get cluster credentials '%s': %v", clusterCredentials.Clustercredentials_cred_id, err)
		}
	}

	bearerToken := clusterCredentials.Serviceaccount_bearer_token

	name := argosharedutil.GenerateArgoCDClusterSecretName(*managedEnv)
	insecureVerifyTLS := clusterCredentials.AllowInsecureSkipTLSVerify

	clusterSecretConfigJSON := ClusterSecretConfigJSON{
		BearerToken: bearerToken,
		TLSClientConfig: ClusterSecretTLSClientConfigJSON{
			Insecure: insecureVerifyTLS,
		},
	}

	jsonString, err := json.Marshal(clusterSecretConfigJSON)
	if err != nil {
		return corev1.Secret{}, deleteSecret_false, fmt.Errorf("SEVERE: unable to marshal JSON")
	}

	managedEnvironmentSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: opConfig.argoCDNamespace.Name,
			Labels: map[string]string{
				ArgoCDSecretTypeKey:                            ArgoCDSecretTypeValue_ClusterSecret,
				controllers.ArgoCDClusterSecretDatabaseIDLabel: managedEnv.Managedenvironment_id,
			},
		},
		Data: map[string][]byte{
			"name":   ([]byte)(name),
			"server": ([]byte)(clusterCredentials.Host),
			"config": ([]byte)(string(jsonString)),
		},
	}

	return managedEnvironmentSecret, deleteSecret_false, nil

}

type ClusterSecretConfigJSON struct {
	BearerToken     string                           `json:"bearerToken"`
	TLSClientConfig ClusterSecretTLSClientConfigJSON `json:"tlsClientConfig"`
}

type ClusterSecretTLSClientConfigJSON struct {
	Insecure bool `json:"insecure"`
}
