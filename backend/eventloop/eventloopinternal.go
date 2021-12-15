package eventloop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/util"
	v1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// For more information on how events are distributed between goroutines by event loop, see:
// https://miro.com/app/board/o9J_lgiqJAs=/?moveToWidget=3458764514216218600&cot=14

type eventLoopEvent struct {
	eventType   EventLoopEventType
	request     ctrl.Request
	reqResource managedgitopsv1alpha1.GitOpsResourceType
	client      client.Client
	workspaceID string
}

const (
	// RequestTimeoutDuration is the default length of time a workspace event goroutine should process an event before
	// giving up (cancelling the context)
	RequestTimeoutDuration = time.Minute * time.Duration(5)
)

// workspaceEntry maintains individual state for each workspace
type workspaceEntry struct {
	workspaceID               string
	workspaceEventLoopChannel chan workspaceEventLoopMessage
	// This struct should only be used from within the event loop
}

// eventLoopRouter routes messages to the channel/go routine responsible for handling a particular workspace's events
// This channel is non-blocking.
func eventLoopRouter(input chan eventLoopEvent) {

	eventLoopRouterLog := log.FromContext(context.Background())

	eventLoopRouterLog.Info("eventLoopRouter started.")
	defer eventLoopRouterLog.Error(nil, "SEVERE: eventLoopRouter ended.")

	workspaceEntries := map[string] /* workspace id -> */ workspaceEntry{}

	for {

		event := <-input

		eventLoopRouterLog.V(sharedutil.LogLevel_Debug).Info("eventLoop received event", "event", stringEventLoopEvent(&event), "workspace", event.workspaceID)

		workspaceEntryVal, ok := workspaceEntries[event.workspaceID]
		if !ok {
			// Start the workspace's event loop go-routine, if it's not already started.
			workspaceEntryVal = workspaceEntry{
				workspaceID:               event.workspaceID,
				workspaceEventLoopChannel: make(chan workspaceEventLoopMessage),
			}
			workspaceEntries[event.workspaceID] = workspaceEntryVal

			go workspaceEventLoop(workspaceEntryVal.workspaceEventLoopChannel, event.workspaceID)
		}

		// Send the event to the channel/go routine that handles all events for this workspace (non-blocking)
		workspaceEntryVal.workspaceEventLoopChannel <- workspaceEventLoopMessage{
			messageType: WorkspaceEventLoopMessageType_Event,
			event:       &event,
		}

	}

}

type WorkspaceEventLoopMessageType int

const (
	WorkspaceEventLoopMessageType_WorkComplete WorkspaceEventLoopMessageType = iota
	WorkspaceEventLoopMessageType_Event
)

type workspaceEventLoopMessage struct {
	messageType WorkspaceEventLoopMessageType
	event       *eventLoopEvent
}

// workspaceEventLoop receives (and acts on) all events on a workspace
func workspaceEventLoop(input chan workspaceEventLoopMessage, workspaceID string) {

	workspaceEventLoopLog := log.FromContext(context.Background()).WithValues("workspaceID", workspaceID)

	workspaceEventLoopLog.Info("workspaceEventLoop started")
	defer workspaceEventLoopLog.Info("eventLoopRouter ended.")

	// Whether the runner is currently busy
	runnerProcessingEvent := false

	runnerChannelOutput := make(chan eventLoopEvent)

	// Start go routine for processing workspace events
	workspaceEventLoopRunner(runnerChannelOutput, input)

	// TODO: GITOPS-1636 - Update GitOpsDeployment status field based on Application database entry's health/status

	// Maintain a list of workspace events we have received from Reconcile(), but not yet processed.
	unprocessedEvents := []eventLoopEvent{}
	// TODO: PERF - workspaceEventLoop - We should remove duplicates to save space and cycles.
	// TODO: PERF - workspaceEventLoop - At some point, we need to start dropping events to prevent OOM. (we need to make sure we can handle dropping events). OTOH, perhaps duplicate removal means that's not necessary?
	for {

		// Block on waiting for more events for this workspace
		newEvent := <-input

		if newEvent.messageType == WorkspaceEventLoopMessageType_Event {
			if newEvent.event == nil {
				workspaceEventLoopLog.Error(nil, "SEVERE: workspaceEventLoop event was nil")
				continue
			}

			workspaceEventLoopLog.V(sharedutil.LogLevel_Debug).Info("workspaceEventLoop received event", "event", stringEventLoopEvent(newEvent.event))

			unprocessedEvents = append(unprocessedEvents, *newEvent.event)

		} else if newEvent.messageType == WorkspaceEventLoopMessageType_WorkComplete {
			workspaceEventLoopLog.V(sharedutil.LogLevel_Debug).Info("workspaceEventLoop received work complete event")

			runnerProcessingEvent = false

		} else {
			workspaceEventLoopLog.Error(nil, "SEVERE: Unrecognized workspace event type", "type", newEvent.messageType)
			continue
		}

		if len(unprocessedEvents) > 0 && !runnerProcessingEvent {
			runnerProcessingEvent = true
			nextEvent := unprocessedEvents[0]
			unprocessedEvents = unprocessedEvents[1:]

			// Send the work to the runner
			runnerChannelOutput <- nextEvent
			workspaceEventLoopLog.V(sharedutil.LogLevel_Debug).Info("Sent work to runner", "event", stringEventLoopEvent(&nextEvent))
		}
	}
}

// TODO: What if you create a GitOpsSyncRun, then create a GitOpsSyncDeploymentRun... the sync run SHOULD work (and sync run should not block on waiting for other to exist)
// actually this will work, because sync run setups up the deployment if it doesn't exist.

func workspaceEventLoopRunner(inputChannel chan eventLoopEvent, informWorkCompleteChan chan workspaceEventLoopMessage) {

	go func() {

		outerContext := context.Background()
		actionLog := log.FromContext(outerContext)

		for {
			// Read from input channel: wait for an event on this workspace
			newEvent := <-inputChannel

			ctx, cancel := context.WithDeadline(outerContext, time.Now().Add(RequestTimeoutDuration))

			defer cancel()

			// Process the event

			actionLog.V(sharedutil.LogLevel_Debug).Info("workspaceEventLoopRunner - event received", "event", stringEventLoopEvent(&newEvent))

			backoff := util.ExponentialBackoff{Min: time.Duration(100 * time.Millisecond), Max: time.Duration(15 * time.Second), Factor: 2, Jitter: true}
		inner_for:
			for {

				// Break if the request is cancelled, or the timeout expires
				select {
				case <-ctx.Done():
					break inner_for
				default:
				}

				_, err := sharedutil.CatchPanic(func() error {

					action := workspaceEventLoopRunner_Action{
						getK8sClientForGitOpsEngineInstance: actionGetK8sClientForGitOpsEngineInstance,
						eventResourceName:                   newEvent.request.Name,
						eventResourceNamespace:              newEvent.request.Namespace,
						workspaceClient:                     newEvent.client,
						log:                                 actionLog,
					}

					var err error

					dbQueries, err := db.NewProductionPostgresDBQueries(true)
					if err != nil {
						actionLog.Error(err, "unable to access database in workspaceEventLoopRunner")
						return err
					}

					if newEvent.eventType == DeploymentModified {
						_, _, err = action.workspaceEventLoopRunner_handleDeploymentModified(ctx, dbQueries)

					} else if newEvent.eventType == SyncRunModified {
						err = action.workspaceEventLoopRunner_handleSyncRunModified(ctx, dbQueries)

					} else {
						actionLog.Error(nil, "SEVERE: Unrecognized event type", "event type", newEvent.eventType)
					}

					// TODO: GITOPS-1582: Implement detection of workspace/api proxy delete, here, and handle cleanup

					return err

				})

				if err == nil {
					break inner_for
				} else {
					actionLog.Error(err, "error from inner event handler")
					backoff.DelayOnFail(ctx)
				}
			}

			// Inform the caller that we have completed a single unit of work
			informWorkCompleteChan <- workspaceEventLoopMessage{messageType: WorkspaceEventLoopMessageType_WorkComplete}

		}

	}()

	// return outputChannel
}

func (a *workspaceEventLoopRunner_Action) workspaceEventLoopRunner_handleSyncRunModified(ctx context.Context, dbQueries db.DatabaseQueries) error {
	// TODO: Hmm: no support for multiple simultaneous operations on a workspace.

	// TODO: The ability for a newer sync operation to cancel an older sync operation.
	// Note: it's not just a sync event, it's an actual new sync object.

	actionLog := a.log

	namespace := v1.Namespace{}
	if err := a.workspaceClient.Get(ctx, types.NamespacedName{Namespace: a.eventResourceNamespace, Name: a.eventResourceNamespace}, &namespace); err != nil {
		return fmt.Errorf("unable to retrieve namespace '%s': %v", a.eventResourceNamespace, err)
	}

	clusterUser, err := getOrCreateClusterUserByNamespaceUID(ctx, string(namespace.UID), dbQueries)
	if err != nil {
		return fmt.Errorf("unable to retrieve cluster user in handleDeploymentModified, '%s': %v", string(namespace.UID), err)
	}

	// Retrieve the GitOpsDeploymentSyncRun from the namespace
	syncRunCRExists := true // True if the GitOpsDeployment resource exists in the namespace, false otherwise
	syncRunCR := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{}
	{
		syncRunKey := client.ObjectKey{Namespace: a.eventResourceNamespace, Name: a.eventResourceName}

		if err := a.workspaceClient.Get(ctx, syncRunKey, syncRunCR); err != nil {

			if apierr.IsNotFound(err) {
				syncRunCRExists = false
			} else {
				actionLog.Error(err, "unable to locate object in handleSyncRunModified", "request", syncRunKey)
				return err
			}
		}
	}

	var apiCRToDBList []db.APICRToDatabaseMapping
	dbEntryExists := false
	if syncRunCRExists {
		mapping := db.APICRToDatabaseMapping{
			APIResourceType: "GitOpsDeploymentSyncRun",
			APIResourceUID:  string(syncRunCR.UID),
			DBRelationType:  "SyncOperation",
		}

		if err := dbQueries.GetDatabaseMappingForAPICR(ctx, &mapping); err != nil {
			if db.IsResultNotFoundError(err) {
				// No corresponding entry
				dbEntryExists = false
			} else {
				// A generic error occured, so just return
				actionLog.Error(err, "unable to resoure APICRToDatabaseMapping", "uid", string(syncRunCR.UID))
				return err
			}
		} else {
			// Match found in database
			apiCRToDBList = append(apiCRToDBList, mapping)
		}
	} else {
		// The CR no longer exists (it was likely deleted), so instead we retrieve the UID of the SyncRun from
		// the APICRToDatabaseMapping table, by combination of (name/namespace/workspace).

		if err := dbQueries.UncheckedListAPICRToDatabaseMappingByAPINamespaceAndName(ctx, "GitOpsDeploymentSyncRun", a.eventResourceName, a.eventResourceNamespace,
			getWorkspaceIDFromNamespaceID(namespace), "Operation", &apiCRToDBList); err != nil {

			actionLog.Error(err, "unable to find API CR to DB Mapping, by API name/namespace/uid",
				"name", a.eventResourceName, "namespace", a.eventResourceNamespace, "UID", string(namespace.UID))
			return err
		}

		if len(apiCRToDBList) == 0 {
			// Not found: the database does not contain an entry for the GitOpsDeploymentSyncRun, and the CR doesn't exist,
			// so there is no more work for us to do.
			dbEntryExists = false
		} else {
			// Found: we were able to locate the DeplToAppMap resource for the deleted GitOpsDeployment,
			dbEntryExists = true
		}
	}

	actionLog.Info("workspacerEventLoopRunner_handleSyncRunModified", "syncRunCRExists", syncRunCRExists, "dbEntryExists", dbEntryExists)

	if !syncRunCRExists && !dbEntryExists {
		actionLog.Info("neither sync run CR exists, nor db entry, so our work is done.")
		// if neither exists, our work is done
		return nil
	}

	// The applications and gitopsengineinstance pointed to by the gitopsdeployment (if they are non-nil)
	var application *db.Application
	var gitopsEngineInstance *db.GitopsEngineInstance

	var gitopsDepl *managedgitopsv1alpha1.GitOpsDeployment

	if syncRunCRExists {
		// Location the Gitops

		gitopsDepl = &managedgitopsv1alpha1.GitOpsDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      syncRunCR.Spec.GitopsDeploymentName,
				Namespace: syncRunCR.Namespace,
			},
		}
		// Retrieve the GitOpsDeployment, and ensure it is processed, if necessary
		{
			if err := a.workspaceClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl); err != nil {

				if apierr.IsNotFound(err) {

					err := fmt.Errorf("unable to retrieve gitopsdeployment referenced in syncrun: %v", err)
					// TODO: If the gitopsDepl isn't referenced, update the status of the GitOpsDeplomentSyncRun condition as an error and return
					// TODO: ENHANCEMENT - implement status conditions on GitOpsDeploymentSyncRun
					actionLog.Error(err, "handleSyncRunModified error")
					return err

				}

				// If there was a generic error in retrieving the key, return it
				actionLog.Error(err, "unable to retrieve gitopsdeployment referenced in syncrun")
				return err
			}

			// The GitOpsDeployment exists, so ensure that it is processed (database entries exist for it, it is configured on gitops engine, etc.)
			// before we use it.

			actionLog.Info("handleSyncRunModified calling handleDeploymentModified, to ensure Deployment is fully setup")

			newAction := a.shallowClone()
			newAction.eventResourceName = gitopsDepl.Name
			newAction.eventResourceNamespace = gitopsDepl.Namespace

			application, gitopsEngineInstance, err = newAction.workspaceEventLoopRunner_handleDeploymentModified(ctx, dbQueries)
			if err != nil {
				actionLog.Error(err, "Error in handleDeploymentModified, when called from handleSyncRunModified", "gitopsdeplname", gitopsDepl.Name, "gitopsdeplns", gitopsDepl.Namespace)
				return err
			}
		}

	}

	if syncRunCRExists && !dbEntryExists {
		// Handle create:
		// If the gitopsdeplsyncrun CR exists, but the database entry doesn't, then this is the first time we
		// have seen the GitOpsDeplSyncRun CR.
		// Create it in the DB and create the operation.

		if application == nil || gitopsEngineInstance == nil {
			err := fmt.Errorf("app or engine instance were nil in handleSyncRunModified app: %v, instance: %v", application, gitopsEngineInstance)
			actionLog.Error(err, "unexpected nil value of required objects")
			return err
		}

		// createdResources is a list of database entries created in this function; if an error occurs, we delete them
		// in reverse order.
		var createdResources []db.DisposableResource

		// Create sync operation
		syncOperation := &db.SyncOperation{
			Application_id:      application.Application_id,
			Operation_id:        "delme", // TODO: This field can probably be removed from the database
			DeploymentNameField: syncRunCR.Spec.GitopsDeploymentName,
			Revision:            syncRunCR.Spec.RevisionId,
		}
		if err := dbQueries.UncheckedCreateSyncOperation(ctx, syncOperation); err != nil {
			actionLog.Error(err, "unable to create sync operation in database")
			return err
		}
		createdResources = append(createdResources, syncOperation)

		newApiCRToDBMapping := db.APICRToDatabaseMapping{
			APIResourceType: "GitOpsDeploymentSyncRun", // TODO: DEBT - add these strings to constants
			APIResourceUID:  string(syncRunCR.UID),
			DBRelationType:  "SyncOperation",
			DBRelationKey:   syncOperation.SyncOperation_id,

			APIResourceName:      syncRunCR.Name,
			APIResourceNamespace: syncRunCR.Namespace,
			WorkspaceUID:         getWorkspaceIDFromNamespaceID(namespace),
		}
		if err := dbQueries.CreateAPICRToDatabaseMapping(ctx, &newApiCRToDBMapping); err != nil {
			actionLog.Error(err, "unable to create api to db mapping in database")

			// If we were unable to retrieve the client, delete the resources we created in the previous steps
			sharedutil.DisposeResources(ctx, createdResources, dbQueries, actionLog)

			return err
		}
		createdResources = append(createdResources, &newApiCRToDBMapping)

		operationClient, err := a.getK8sClientForGitOpsEngineInstance(gitopsEngineInstance)
		if err != nil {
			actionLog.Error(err, "unable to retrieve gitopsengine instance from handleSyncRunModified")

			// If we were unable to retrieve the client, delete the resources we created in the previous steps
			sharedutil.DisposeResources(ctx, createdResources, dbQueries, actionLog)

			// Return the original error
			return err
		}

		dbOperationInput := db.Operation{
			Instance_id:   gitopsEngineInstance.Gitopsengineinstance_id,
			Resource_id:   syncOperation.SyncOperation_id,
			Resource_type: "SyncOperation", // TODO: DEBT - ADd this to contants
		}

		k8sOperation, dbOperation, err := uncheckedCreateOperation(ctx, false, dbOperationInput, clusterUser.Clusteruser_id, sharedutil.GitOpsEngineSingleInstanceNamespace, dbQueries, operationClient, actionLog)
		if err != nil {
			actionLog.Error(err, "could not create operation", "namespace", sharedutil.GitOpsEngineSingleInstanceNamespace)

			// If we were unable to create the operation, delete the resources we created in the previous steps
			sharedutil.DisposeResources(ctx, createdResources, dbQueries, actionLog)

			return err
		}

		// TODO: GITOPS-1467 - STUB - Remove the 'false' in createOperation above, once cluster agent handling of operation is implemented.
		actionLog.Info("STUB: Not waiting for create Sync Run operation to complete, in handleNewSyncRunModified")

		if err := cleanupOperation(ctx, *dbOperation, *k8sOperation, sharedutil.GitOpsEngineSingleInstanceNamespace, dbQueries, operationClient, actionLog); err != nil {
			sharedutil.DisposeResources(ctx, createdResources, dbQueries, actionLog)
			return err
		}

		sharedutil.DisposeResources(ctx, createdResources, dbQueries, actionLog)
		return nil
	}

	if !syncRunCRExists && dbEntryExists {
		// Handle delete:
		// If the gitopsdeplsyncrun CR doesn't exist, but database row does, then the CR has been deleted, so handle it.

		// TODO: DEBT - Log stub: need to implement sync on cluster side
		actionLog.Info("STUB: cluster-agent")

		// Remove the mappings
		for idx := range apiCRToDBList {
			apiCRToDB := &apiCRToDBList[idx]

			if apiCRToDB.DBRelationKey != "SyncOperation" { // TODO: DEBT: Add this to constants
				err = fmt.Errorf("SEVERE: unexpected DBRelationKey, should be SyncOperation")
				actionLog.Error(err, "")
				return err
			}

			rowsDeleted, err := dbQueries.UncheckedDeleteSyncOperationById(ctx, apiCRToDB.DBRelationKey)
			if err != nil {
				actionLog.V(sharedutil.LogLevel_Warn).Error(err, "unable to delete sync operation db entry on sync operation delete", "key", apiCRToDB.DBRelationKey)
			} else if rowsDeleted == 0 {
				actionLog.V(sharedutil.LogLevel_Warn).Error(err, "unexpected number of rows deleted on sync db entry delete", "key", apiCRToDB.DBRelationKey)
			}

			var operations []db.Operation
			if err := dbQueries.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, apiCRToDB.DBRelationKey, apiCRToDB.APIResourceType, &operations, clusterUser.Clusteruser_id); err != nil {
				actionLog.V(sharedutil.LogLevel_Warn).Error(err, "unable to retrieve operations pointing to sync operation", "key", apiCRToDB.DBRelationKey)
			} else {
				// Delete the operations that reference this SyncOperation
				for idx := range operations {
					operationId := operations[idx].Operation_id

					rowsDeleted, err := dbQueries.DeleteOperationById(ctx, operationId, clusterUser.Clusteruser_id)
					if err != nil {
						actionLog.V(sharedutil.LogLevel_Warn).Error(err, "unable to delete old operation", "operationId", operationId)
					} else if rowsDeleted == 0 {
						actionLog.V(sharedutil.LogLevel_Warn).Error(err, "unexpected number of deleted rows when deleting old operation", "operationId", operationId)
					}
				}
			}

			rowsDeleted, err = dbQueries.UncheckedDeleteAPICRToDatabaseMapping(ctx, apiCRToDB)
			if err != nil {
				actionLog.V(sharedutil.LogLevel_Warn).Error(err, "unable to delete apiCRToDBmapping", "mapping", apiCRToDBList[idx].APIResourceUID)
			} else if rowsDeleted == 0 {
				actionLog.V(sharedutil.LogLevel_Warn).Error(err, "unexpected number of rows deleted of apiCRToDBmapping", "mapping", apiCRToDBList[idx].APIResourceUID)
			}
		}

		return nil
	}

	if syncRunCRExists && dbEntryExists {

		if syncRunCR == nil {
			err := fmt.Errorf("SEVERE - vsync run cr is nil")
			actionLog.Error(err, "This shouldn't happen")
			return err
		}

		if len(apiCRToDBList) != 1 {
			err := fmt.Errorf("SEVERE - Update only supports one operation parameter")
			actionLog.Error(err, "This shouldn't happen")
			return err
		}

		// Get the SyncOperation table entry pointed to by the resource
		apiCRToDBMapping := apiCRToDBList[0]

		if apiCRToDBMapping.DBRelationType != "SyncOperation" {
			err := fmt.Errorf("SEVERE - db relation type should be syncoperation")
			actionLog.Error(err, "This shouldn't happen")
			return err
		}

		syncOperation := db.SyncOperation{SyncOperation_id: apiCRToDBMapping.DBRelationKey}

		if err := dbQueries.UncheckedGetSyncOperationById(ctx, &syncOperation); err != nil {
			actionLog.Error(err, "unable to retrieve sync operation by id on modified", "operationID", syncOperation.SyncOperation_id)
			return err
		}

		if syncOperation.DeploymentNameField != syncRunCR.Spec.GitopsDeploymentName {
			err := fmt.Errorf("deployment name field is immutable: changing it from its initial value is not supported")
			actionLog.Error(err, "deployment name field change is not supported")
			return err
		}

		if syncOperation.Revision != syncRunCR.Spec.RevisionId {
			err := fmt.Errorf("revision change is not supported: changing it from its initial value is not supported")
			actionLog.Error(err, "revision field change is not supported")
			return err
		}

		// TODO: Is there logic that should occur in sync run handler if gitops depl is deleted?
		// - Any active sync operations should be cancelled. (no such thing)
		// - won't the syncrun db database be pointing to an application that doesn't exist?
		// 		- delete syncrunoperations (and apicrtothing) that point to applications that no longer exist

		// TODO: Add UID of the gitopsdeplsync to gitopsdepl operation?
		//		- this would allow us to detect is the gitosp depl had magically changed. (but irrelevant if above)

		// TODO: Make sure we are checking whether the SyncOperation exists

		return nil
	}

	return nil

}

// func (a *workspaceEventLoopRunner_Action) processDeploymentObjectIfNeeded(ctx context.Context, actionLog logr) error {
// 	if !gitopsDeploymentCRExists && !deplToAppMapExistsInDB {
// 		// if neither exists, our work is done
// 		return nil
// 	}

// 	if gitopsDeploymentCRExists && !deplToAppMapExistsInDB {
// 		// If the gitopsdepl CR exists, but the database entry doesn't, then this is the first time we
// 		// have seen the GitOpsDepl CR.
// 		// Create it in the DB and create the operation.
// 		return a.handleNewGitOpsDeplEvent(ctx, gitopsDeployment, clusterUser, sharedutil.GitOpsEngineSingleInstanceNamespace, dbQueries, actionLog)

// 	}

// 	if !gitopsDeploymentCRExists && deplToAppMapExistsInDB {
// 		// If the gitopsdepl CR doesn't exist, but database row does, then the CR has been deleted, so handle it.
// 		return a.handleDeleteGitOpsDeplEvent(ctx, clusterUser, sharedutil.GitOpsEngineSingleInstanceNamespace,
// 			&deplToAppMappingList, dbQueries, actionLog)
// 	}

// 	if gitopsDeploymentCRExists && deplToAppMapExistsInDB {

// 		if len(deplToAppMappingList) != 1 {
// 			err := fmt.Errorf("SEVERE - Update only supports one operation parameter")
// 			actionLog.Error(err, "This shouldn't happen")
// 			return err
// 		}

// 		// if both exist: it's an update (or a no-op)
// 		return a.handleUpdatedGitOpsDeplEvent(ctx, &deplToAppMappingList[0], gitopsDeployment, clusterUser,
// 			sharedutil.GitOpsEngineSingleInstanceNamespace, dbQueries, actionLog)
// 	}

// 	return nil

// }

func (a *workspaceEventLoopRunner_Action) workspaceEventLoopRunner_handleDeploymentModified(ctx context.Context, dbQueries db.DatabaseQueries) (*db.Application, *db.GitopsEngineInstance, error) {

	deplName := a.eventResourceName
	deplNamespace := a.eventResourceNamespace
	workspaceClient := a.workspaceClient

	actionLog := log.FromContext(ctx)

	namespace := v1.Namespace{}
	if err := workspaceClient.Get(ctx, types.NamespacedName{Namespace: deplNamespace, Name: deplNamespace}, &namespace); err != nil {
		return nil, nil, fmt.Errorf("unable to retrieve namespace '%s': %v", deplNamespace, err)
	}

	clusterUser, err := getOrCreateClusterUserByNamespaceUID(ctx, string(namespace.UID), dbQueries)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to retrieve cluster user in handleDeploymentModified, '%s': %v", string(namespace.UID), err)
	}

	// Retrieve the GitOpsDeployment from the namespace
	gitopsDeploymentCRExists := true // True if the GitOpsDeployment resource exists in the namespace, false otherwise
	gitopsDeployment := &managedgitopsv1alpha1.GitOpsDeployment{}
	{
		gitopsDeploymentKey := client.ObjectKey{Namespace: deplNamespace, Name: deplName}

		if err := workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment); err != nil {

			if apierr.IsNotFound(err) {
				gitopsDeploymentCRExists = false
			} else {
				actionLog.Error(err, "unable to locate object in handleDeploymentModified", "request", gitopsDeploymentKey)
				return nil, nil, err
			}
		}
	}

	// Next, retrieve the corresponding database entry, if applicable
	deplToAppMapExistsInDB := false

	var deplToAppMappingList []db.DeploymentToApplicationMapping
	if gitopsDeploymentCRExists {
		// The CR exists, so use the UID of the CR to retrieve the database entry, if possible
		deplToAppMapping := &db.DeploymentToApplicationMapping{Deploymenttoapplicationmapping_uid_id: string(gitopsDeployment.UID)}

		if err = dbQueries.UncheckedGetDeploymentToApplicationMappingByDeplId(ctx, deplToAppMapping); err != nil {

			if db.IsResultNotFoundError(err) {
				deplToAppMapExistsInDB = false
			} else {
				actionLog.Error(err, "unable to retrieve deployment to application mapping", "uid", string(gitopsDeployment.UID))
				return nil, nil, err
			}

		} else {
			deplToAppMappingList = append(deplToAppMappingList, *deplToAppMapping)
			deplToAppMapExistsInDB = true
		}
	} else {
		// The CR no longer exists (it was likely deleted), so instead we retrieve the UID of the GitOpsDeployment from
		// the DeploymentToApplicatinMapping table, by combination of (name/namespace/workspace).

		if err := dbQueries.UncheckedListDeploymentToApplicationMappingByNamespaceAndName(ctx, deplName, deplNamespace,
			getWorkspaceIDFromNamespaceID(namespace), &deplToAppMappingList); err != nil {

			actionLog.Error(err, "unable to retrieve deployment to application mapping by name/namespace/uid",
				"name", deplName, "namespace", deplNamespace, "UID", string(namespace.UID))
			return nil, nil, err
		}

		if len(deplToAppMappingList) == 0 {
			// Not found: the database does not contain an entry for the GitOpsDeployment, and the CR doesn't exist,
			// so there is no more work for us to do.
			deplToAppMapExistsInDB = false
		} else {
			// Found: we were able to locate the DeplToAppMap resource for the deleted GitOpsDeployment,
			deplToAppMapExistsInDB = true
		}

	}

	actionLog.V(sharedutil.LogLevel_Debug).Info("workspacerEventLoopRunner_handleDeploymentModified processing event", "gitopsDeploymentExists", gitopsDeploymentCRExists, "deplToAppMapExists", deplToAppMapExistsInDB)

	if !gitopsDeploymentCRExists && !deplToAppMapExistsInDB {
		// if neither exists, our work is done
		return nil, nil, nil
	}

	if gitopsDeploymentCRExists && !deplToAppMapExistsInDB {
		// If the gitopsdepl CR exists, but the database entry doesn't, then this is the first time we
		// have seen the GitOpsDepl CR.
		// Create it in the DB and create the operation.
		return a.handleNewGitOpsDeplEvent(ctx, gitopsDeployment, clusterUser, sharedutil.GitOpsEngineSingleInstanceNamespace, dbQueries)

	}

	if !gitopsDeploymentCRExists && deplToAppMapExistsInDB {
		// If the gitopsdepl CR doesn't exist, but database row does, then the CR has been deleted, so handle it.
		err := a.handleDeleteGitOpsDeplEvent(ctx, clusterUser, sharedutil.GitOpsEngineSingleInstanceNamespace,
			&deplToAppMappingList, dbQueries)

		return nil, nil, err
	}

	if gitopsDeploymentCRExists && deplToAppMapExistsInDB {

		if len(deplToAppMappingList) != 1 {
			err := fmt.Errorf("SEVERE - Update only supports one operation parameter")
			actionLog.Error(err, "This shouldn't happen")
			return nil, nil, err
		}

		// if both exist: it's an update (or a no-op)
		return a.handleUpdatedGitOpsDeplEvent(ctx, &deplToAppMappingList[0], gitopsDeployment, clusterUser,
			sharedutil.GitOpsEngineSingleInstanceNamespace, dbQueries)
	}

	return nil, nil, fmt.Errorf("SEVERE - All cases should be handled by above if statements")
}

func (a workspaceEventLoopRunner_Action) handleDeleteGitOpsDeplEvent(ctx context.Context, clusterUser *db.ClusterUser,
	operationNamespace string, deplToAppMappingList *[]db.DeploymentToApplicationMapping, dbQueries db.DatabaseQueries) error {

	if deplToAppMappingList == nil || clusterUser == nil {
		return fmt.Errorf("required parameter should not be nil in handleDelete: %v %v", deplToAppMappingList, clusterUser)
	}

	for idx := range *deplToAppMappingList {
		deplToAppMapping := (*deplToAppMappingList)[idx]

		dbApplicationFound := true

		// If the gitopsdepl CR doesn't exist, but database row does, then the CR has been deleted, so handle it.
		dbApplication := db.Application{
			Application_id: deplToAppMapping.Application_id,
		}
		if err := dbQueries.UncheckedGetApplicationById(ctx, &dbApplication); err != nil {
			dbApplicationFound = false
			a.log.Error(err, "unable to get application by id", "id", deplToAppMapping.Application_id)
			// Log the error, but continue.
		}

		actionLog := a.log.WithValues("id", dbApplication.Application_id)

		// Remove the ApplicationState from the database
		rowsDeleted, err := dbQueries.UncheckedDeleteApplicationStateById(ctx, deplToAppMapping.Application_id)
		if err != nil {
			actionLog.Error(err, "unable to delete application state by id")
			// Log the error, but continue.
		}
		if rowsDeleted == 0 {
			actionLog.Info("unexpected number of rows deleted for application state", "rowsDeleted", rowsDeleted)
			// but: don't return an error, just keep going
		}

		// Remove DeplToAppMapping
		rowsDeleted, err = dbQueries.UncheckedDeleteDeploymentToApplicationMappingByDeplId(ctx, deplToAppMapping.Deploymenttoapplicationmapping_uid_id)
		if err != nil {
			actionLog.Error(err, "unable to deplToAppMapping by id")
			// Log the error, but continue
		} else if rowsDeleted == 0 {
			actionLog.V(sharedutil.LogLevel_Warn).Error(nil, "unexpected number of rows deleted for deplToAppMapping", "rowsDeleted", rowsDeleted)
			// but: don't return an error, just keep going
		}

		// If the Application table entry still exists...
		if dbApplicationFound {

			// Remove the Application from the database
			rowsDeleted, err = dbQueries.UncheckedDeleteApplicationById(ctx, deplToAppMapping.Application_id)
			if err != nil {
				actionLog.Error(err, "unable to delete application by id")
			} else if rowsDeleted == 0 {
				actionLog.V(sharedutil.LogLevel_Warn).Error(nil, "unexpected number of rows deleted for application", "rowsDeleted", rowsDeleted)
				// but: don't return an error, just keep going
			}

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: dbApplication.Engine_instance_inst_id,
			}
			if err := dbQueries.UncheckedGetGitopsEngineInstanceById(ctx, &gitopsEngineInstance); err != nil {
				actionLog.V(sharedutil.LogLevel_Warn).Error(err, "GitOpsEngineInstance could not be retrieved during gitopsdepl deletion handling")
				continue
			}

			// Create the operation
			gitopsEngineClient, err := a.getK8sClientForGitOpsEngineInstance(&gitopsEngineInstance)
			if err != nil {
				actionLog.Error(err, "could not retrieve client for gitops engine instance", "instance", gitopsEngineInstance.Gitopsengineinstance_id)
				continue
			}
			dbOperationInput := db.Operation{
				Instance_id:   dbApplication.Engine_instance_inst_id,
				Resource_id:   deplToAppMapping.Application_id,
				Resource_type: "Application",
			}

			k8sOperation, dbOperation, err := uncheckedCreateOperation(ctx, false, dbOperationInput, clusterUser.Clusteruser_id, operationNamespace, dbQueries, gitopsEngineClient, actionLog)
			if err != nil {
				actionLog.Error(err, "unable to create operation", "operation", dbOperationInput.ShortString())
				continue
			}

			// TODO: GITOPS-1467 - STUB - Skipping waiting on operation to complete 'handleDeleteGitOpsDeplEvent'
			actionLog.Info("STUB - Skipping waiting on operation to complete 'handleDeleteGitOpsDeplEvent'")

			// TODO: GITOPS-1467 - Do something with the operation status

			if err := cleanupOperation(ctx, *dbOperation, *k8sOperation, operationNamespace, dbQueries, gitopsEngineClient, actionLog); err != nil {
				actionLog.Error(err, "unable to cleanup operation", "operation", dbOperationInput.ShortString())
				continue
			}
		}
	}

	return nil
}

func (a workspaceEventLoopRunner_Action) handleUpdatedGitOpsDeplEvent(ctx context.Context, deplToAppMapping *db.DeploymentToApplicationMapping,
	gitopsDeployment *managedgitopsv1alpha1.GitOpsDeployment, clusterUser *db.ClusterUser, operationNamespace string,
	dbQueries db.DatabaseQueries) (*db.Application, *db.GitopsEngineInstance, error) {

	if deplToAppMapping == nil || gitopsDeployment == nil || clusterUser == nil {
		return nil, nil, fmt.Errorf("unexpected nil param in handleUpdatedGitOpsDeplEvent: %v %v %v", deplToAppMapping, gitopsDeployment, clusterUser)
	}

	actionLog := a.log.WithValues("applicationId", deplToAppMapping.Application_id, "gitopsDeplUID", gitopsDeployment.UID)

	application := &db.Application{Application_id: deplToAppMapping.Application_id}
	if err := dbQueries.UncheckedGetApplicationById(ctx, application); err != nil {
		if !db.IsResultNotFoundError(err) {
			actionLog.Error(err, "unable to retrieve Application DB entry in handleUpdatedGitOpsDeplEvent")
			return nil, nil, err
		} else {

			// The application pointed to by the deplToAppMapping doesn't exist; this shouldn't happen.
			actionLog.Error(err, "SEVERE: Application pointed to by deplToAppMapping doesn't exist, in handleUpdatedGitOpsDeplEvent")

			// Delete the deplToAppMapping, since the app doesn't exist. This should cause the gitopsdepl to be reconciled by the event loop.
			if _, err := dbQueries.UncheckedDeleteDeploymentToApplicationMappingByDeplId(ctx, deplToAppMapping.Deploymenttoapplicationmapping_uid_id); err != nil {
				actionLog.Error(err, "Unable to delete deplToAppMapping which pointed to non-existent Application, in handleUpdatedGitOpsDeplEvent")
				return nil, nil, err
			}
			return nil, nil, err
		}
	}

	engineInstanceParam := &db.GitopsEngineInstance{
		Gitopsengineinstance_id: application.Engine_instance_inst_id,
	}
	if err := dbQueries.UncheckedGetGitopsEngineInstanceById(ctx, engineInstanceParam); err != nil {
		actionLog.Error(err, "SEVERE: GitOps engine instance pointed to by Application doesn't exist, in handleUpdatedGitOpsDeplEvent", "engine-instance-id", engineInstanceParam.Gitopsengineinstance_id)
		return nil, nil, err
	}

	// TODO: ENHANCEMENT - Ensure that the backend code gracefully handles database values that are too large for the DB field (eg too many VARCHARs)

	// TODO: DEBT - Sanity check that the application.name matches the expected value set in handleCreateGitOpsEvent

	specFieldInput := argoCDSpecInput{
		crName:               application.Name,
		crNamespace:          engineInstanceParam.Namespace_name,
		destinationNamespace: gitopsDeployment.Spec.Destination.Namespace,
		// TODO: After GITOPS 1564 - Fill this in with cluster credentials
		destinationName:      "in-cluster",
		sourceRepoURL:        gitopsDeployment.Spec.Source.RepoURL,
		sourcePath:           gitopsDeployment.Spec.Source.Path,
		sourceTargetRevision: gitopsDeployment.Spec.Source.TargetRevision,
	}

	specFieldResult := createSpecField(specFieldInput)
	if specFieldResult == application.Spec_field {
		// No change required: the application database entry is consistent with the gitopsdepl CR
		return application, engineInstanceParam, nil
	}

	actionLog.Info("Spec change detected between Application DB entry and GitOpsDeployment CR")

	application.Spec_field = specFieldResult

	if err := dbQueries.UncheckedUpdateApplication(ctx, application); err != nil {
		actionLog.Error(err, "Unable to update application, after mismatch detected")
		return nil, nil, err
	}
	// Create the operation
	gitopsEngineClient, err := a.getK8sClientForGitOpsEngineInstance(engineInstanceParam)
	if err != nil {
		actionLog.Error(err, "unable to retrieve gitopsengineinstance for updated gitopsdepl", "gitopsEngineIstance", engineInstanceParam.EngineCluster_id)
		return nil, nil, err
	}

	dbOperationInput := db.Operation{
		Instance_id:   engineInstanceParam.Gitopsengineinstance_id,
		Resource_id:   application.Application_id,
		Resource_type: "Application",
	}

	k8sOperation, dbOperation, err := uncheckedCreateOperation(ctx, false, dbOperationInput, clusterUser.Clusteruser_id, operationNamespace, dbQueries, gitopsEngineClient, actionLog)
	if err != nil {
		actionLog.Error(err, "could not create operation", "operation", dbOperation.Operation_id, "namespace", operationNamespace)
		return nil, nil, err
	}

	// TODO: GITOPS-1467 - STUB - Remove the 'false' in createOperation above, once cluster agent handling of operation is implemented.
	actionLog.Info("STUB: Not waiting for create Application operation to complete, in handleNewGitOpsDeplEvent")

	// TODO: GITOPS-1467 - Do something with the operation status

	if err := cleanupOperation(ctx, *dbOperation, *k8sOperation, operationNamespace, dbQueries, gitopsEngineClient, actionLog); err != nil {
		return nil, nil, err
	}

	return application, engineInstanceParam, nil

}

func ensureRequiredDBEntriesExist(ctx context.Context, workspaceClient client.Client, workspaceNamespace v1.Namespace, clusterUser *db.ClusterUser, operationNamespace string, dbQueries db.DatabaseQueries, actionLog logr.Logger) (*db.ManagedEnvironment, *db.GitopsEngineInstance, *db.ClusterAccess, error) {

	managedEnv, err := sharedutil.UncheckedGetOrCreateManagedEnvironmentByNamespaceUID(ctx, workspaceNamespace, dbQueries, actionLog)
	if err != nil {
		actionLog.Error(err, "unable to get or created managed env on deployment modified event")
		return nil, nil, nil, err
	}

	// TODO: After GITOPS-1564 - Once cluster credential creation is implemented, we need to create an Operation to create the Argo CD cluster secret.
	actionLog.Info("STUB: handleNewGitOpsDeplEvent after getOrCreateManagedEnvironmentByNamespaceUID, skipping creating the Argo CD Cluster Secret via a new Operation.")

	engineInstance, err := uncheckedDetermineGitOpsEngineInstanceForNewApplication(ctx, *clusterUser, *managedEnv, workspaceClient, dbQueries, actionLog)
	if err != nil {
		actionLog.Error(err, "unable to determine gitops engine instance")
		return nil, nil, nil, err
	}

	// Create the cluster access object, to allow us to interact with the GitOpsEngine and ManagedEnvironment on the user's behalf
	ca := db.ClusterAccess{
		Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    managedEnv.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: engineInstance.Gitopsengineinstance_id,
	}

	if err := getOrCreateClusterAccess(ctx, &ca, dbQueries); err != nil {
		actionLog.Error(err, "unable to create cluster access")
		return nil, nil, nil, err
	}

	return managedEnv, engineInstance, &ca, nil

}

// Don't call this directly: call it via workspaceEventLoopRunner_Action
func actionGetK8sClientForGitOpsEngineInstance(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {

	// TODO: GITOPS-1455: When we support multiple Argo CD instances (and multiple instances on separate clusters), this logic should be updated.

	// TODO: Post GITOPS-1564: At the moment, we're not using the managed env param, but it should be tied to cluster credentials of argo cd instance.

	config, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}

	return k8sClient, nil

}

func (a workspaceEventLoopRunner_Action) handleNewGitOpsDeplEvent(ctx context.Context, gitopsDeployment *managedgitopsv1alpha1.GitOpsDeployment,
	clusterUser *db.ClusterUser, operationNamespace string, dbQueries db.DatabaseQueries) (*db.Application, *db.GitopsEngineInstance, error) {

	gitopsDeplNamespace := v1.Namespace{}
	if err := a.workspaceClient.Get(ctx, types.NamespacedName{Namespace: gitopsDeployment.ObjectMeta.Namespace, Name: gitopsDeployment.ObjectMeta.Namespace}, &gitopsDeplNamespace); err != nil {
		return nil, nil, fmt.Errorf("unable to retrieve namespace for managed env, '%s': %v", gitopsDeployment.ObjectMeta.Namespace, err)
	}

	managedEnv, engineInstance, _, err := ensureRequiredDBEntriesExist(ctx, a.workspaceClient, gitopsDeplNamespace, clusterUser, operationNamespace, dbQueries, a.log)
	if err != nil {
		a.log.Error(err, "unable to get or create required db entries on deployment modified event")
		return nil, nil, err
	}

	// managedEnv, err := sharedutil.UncheckedGetOrCreateManagedEnvironmentByNamespaceUID(ctx, gitopsDeplNamespace, dbQueries, actionLog)
	// if err != nil {
	// 	actionLog.Error(err, "unable to get or created managed env on deployment modified event")
	// 	return err
	// }

	// // TODO: After GITOPS-1564 - Once cluster credential creation is implemented, we need to create an Operation to create the Argo CD cluster secret.
	// actionLog.Info("STUB: handleNewGitOpsDeplEvent after getOrCreateManagedEnvironmentByNamespaceUID, skipping creating the Argo CD Cluster Secret via a new Operation.")

	// engineInstance, err := uncheckedDetermineGitOpsEngineInstanceForNewApplication(ctx, *clusterUser, *managedEnv, event.client, dbQueries, actionLog)
	// if err != nil {
	// 	actionLog.Error(err, "unable to determine gitops engine instance")
	// 	return err
	// }

	// // Create the cluster access object, to allow us to interact with the GitOpsEngine and ManagedEnvironment on the user's behalf
	// ca := db.ClusterAccess{
	// 	Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
	// 	Clusteraccess_managed_environment_id:    managedEnv.Managedenvironment_id,
	// 	Clusteraccess_gitops_engine_instance_id: engineInstance.Gitopsengineinstance_id,
	// }

	// if err := getOrCreateClusterAccess(ctx, &ca, dbQueries); err != nil {
	// 	actionLog.Error(err, "unable to create cluster access")
	// 	return err
	// }

	appName := "gitopsdepl-" + string(gitopsDeployment.UID)

	specFieldInput := argoCDSpecInput{
		crName:               appName,
		crNamespace:          engineInstance.Namespace_name,
		destinationNamespace: gitopsDeployment.Spec.Destination.Namespace,
		// TODO: After GITOPS 1564 - Fill this in with cluster credentials
		destinationName:      "in-cluster",
		sourceRepoURL:        gitopsDeployment.Spec.Source.RepoURL,
		sourcePath:           gitopsDeployment.Spec.Source.Path,
		sourceTargetRevision: gitopsDeployment.Spec.Source.TargetRevision,
	}

	application := db.Application{
		Name:                    appName,
		Engine_instance_inst_id: engineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnv.Managedenvironment_id,
		Spec_field:              createSpecField(specFieldInput),
	}

	if err := dbQueries.CreateApplication(ctx, &application, clusterUser.Clusteruser_id); err != nil {
		a.log.Error(err, "unable to create application", "application", application, "ownerId", clusterUser.Clusteruser_id)
		return nil, nil, err
	}

	requiredDeplToAppMapping := &db.DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: string(gitopsDeployment.UID),
		Application_id:                        application.Application_id,
		DeploymentName:                        gitopsDeployment.Name,
		DeploymentNamespace:                   gitopsDeployment.Namespace,
		WorkspaceUID:                          getWorkspaceIDFromNamespaceID(gitopsDeplNamespace),
	}

	if err := sharedutil.UncheckedGetOrCreateDeploymentToApplicationMapping(ctx, requiredDeplToAppMapping, dbQueries, a.log); err != nil {
		a.log.Error(err, "unable to create deplToApp mapping", "deplToAppMapping", requiredDeplToAppMapping)
		return nil, nil, err
	}

	dbOperationInput := db.Operation{
		Instance_id:   engineInstance.Gitopsengineinstance_id,
		Resource_id:   application.Application_id,
		Resource_type: "Application",
	}

	gitopsEngineClient, err := a.getK8sClientForGitOpsEngineInstance(engineInstance)
	if err != nil {
		return nil, nil, err
	}

	k8sOperation, dbOperation, err := uncheckedCreateOperation(ctx, false, dbOperationInput, clusterUser.Clusteruser_id, operationNamespace, dbQueries, gitopsEngineClient, a.log)
	if err != nil {
		a.log.Error(err, "could not create operation", "namespace", operationNamespace)
		return nil, nil, err
	}

	// TODO: GITOPS-1467 - STUB - Remove the 'false' in createOperation above, once cluster agent handling of operation is implemented.
	a.log.Info("STUB: Not waiting for create Application operation to complete, in handleNewGitOpsDeplEvent")

	if err := cleanupOperation(ctx, *dbOperation, *k8sOperation, operationNamespace, dbQueries, gitopsEngineClient, a.log); err != nil {
		return nil, nil, err
	}

	return &application, engineInstance, nil
}

// workspaceEventLoopRunner_Action is a short-lived struct containing data required to perform an action
// on the database, and/or on gitops engine cluster.
type workspaceEventLoopRunner_Action struct {

	// logger to use
	log logr.Logger

	// The Name field of the K8s object (for example, a GitOpsDeploymentRun with a name of 'my-app-deployment')
	eventResourceName string

	// The namespace (for example, a GitOpsDeploymentRun in namespace 'my-deployments')
	eventResourceNamespace string

	// The K8s client that can be used to read/write objects on the workspace cluster
	workspaceClient client.Client

	// getK8sClientForGitOpsEngineInstance returns the K8s client that corresponds to the gitops engine instance.
	// As of this writing, only one Argo CD instance is supported, so this is trivial, but should have
	// more complex logic in the future.
	getK8sClientForGitOpsEngineInstance func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error)
}

func (a *workspaceEventLoopRunner_Action) shallowClone() workspaceEventLoopRunner_Action {
	return *a
}

// cleanupOperation cleans up the database entry and (optionally) the CR, once an operation has concluded.
func cleanupOperation(ctx context.Context, dbOperation db.Operation, k8sOperation operation.Operation, operationNamespace string,
	dbQueries db.DatabaseQueries, gitopsEngineClient client.Client, actionLog logr.Logger) error {

	// Delete the database entry
	rowsDeleted, err := dbQueries.UncheckedDeleteOperationById(ctx, dbOperation.Operation_id)
	if err != nil {
		return err
	}
	if rowsDeleted != 1 {
		actionLog.V(sharedutil.LogLevel_Warn).Error(err, "unexpected number of operation rows deleted", "operation-id", dbOperation.Operation_id, "rows", rowsDeleted)
	}

	// Optional: Delete the Operation CR
	if err := gitopsEngineClient.Delete(ctx, &k8sOperation); err != nil {
		if !apierr.IsNotFound(err) {
			// Log the error, but don't return it: it's the responsibility of the cluster agent to delete the operation cr
			actionLog.Error(err, "unable to delete operation", "operation", dbOperation.Operation_id, "namespace", operationNamespace)
		}
	}

	return nil

}

func uncheckedCreateOperation(ctx context.Context, waitForOperation bool, dbOperationParam db.Operation, clusterUserID string,
	operationNamespace string, dbQueries db.DatabaseQueries, gitopsEngineClient client.Client, actionLog logr.Logger) (*operation.Operation, *db.Operation, error) {

	var err error
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

	actionLog.Info("Creating database operation", "operation", dbOperation.ShortString())

	if err := dbQueries.CreateOperation(ctx, &dbOperation, clusterUserID); err != nil {
		actionLog.Error(err, "unable to create operation", "operation", dbOperation.LongString())
		return nil, nil, err
	}

	// Create K8s operation
	operation := operation.Operation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "operation-" + dbOperation.Operation_id,
			Namespace: operationNamespace,
		},
		Spec: operation.OperationSpec{
			OperationID: dbOperation.Operation_id,
		},
	}

	actionLog.Info("Creating K8s Operation", "operation", fmt.Sprintf("%v", operation.Spec.OperationID))

	if err := gitopsEngineClient.Create(ctx, &operation, &client.CreateOptions{}); err != nil {
		actionLog.Error(err, "unable to create K8s Operation in namespace", "operation", dbOperation.Operation_id, "namespace", operation.Namespace)
		return nil, nil, err
	}

	// Wait for operation to complete.

	if waitForOperation {
		actionLog.Info("Waiting for Operation to complete", "operation", fmt.Sprintf("%v", operation.Spec.OperationID))

		if err = uncheckedWaitForOperationToComplete(ctx, &dbOperation, dbQueries, actionLog); err != nil {
			actionLog.Error(err, "operation did not complete", "operation", dbOperation.Operation_id, "namespace", operation.Namespace)
			return nil, nil, err
		}

		actionLog.Info("Operation completed", "operation", fmt.Sprintf("%v", operation.Spec.OperationID))
	}

	return &operation, &dbOperation, nil

}

func uncheckedWaitForOperationToComplete(ctx context.Context, dbOperation *db.Operation, dbQueries db.DatabaseQueries, actionLog logr.Logger) error {

	backoff := util.ExponentialBackoff{Factor: 2, Min: time.Duration(100 * time.Millisecond), Max: time.Duration(10 * time.Second), Jitter: true}

	for {

		err := dbQueries.UncheckedGetOperationById(ctx, dbOperation)
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
			return fmt.Errorf("operation context is Done()")
		default:
		}

	}

	return nil
}

// Whenever a new Argo CD Application needs to be created, we need to find an Argo CD instance
// that is available to use it. In the future, when we have multiple instances, there would
// be an algorithm that intelligently places applications -> instances, to ensure that
// there are no Argo CD instances that are overloaded (have too many users).
//
// However, at the moment we are using a single shared Argo CD instnace, so we will
// just return that.
//
// This logic would be improved by https://issues.redhat.com/browse/GITOPS-1455 (and others)
func uncheckedDetermineGitOpsEngineInstanceForNewApplication(ctx context.Context, user db.ClusterUser, managedEnv db.ManagedEnvironment, k8sClient client.Client, dbq db.DatabaseQueries, actionLog logr.Logger) (*db.GitopsEngineInstance, error) {

	namespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: sharedutil.GitOpsEngineSingleInstanceNamespace, Namespace: sharedutil.GitOpsEngineSingleInstanceNamespace}}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
		return nil, fmt.Errorf("unable to retrieve gitopsengine namespace in determineGitOpsEngineInstanceForNewApplication")
	}

	kubeSystemNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", Namespace: "kube-system"}}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(kubeSystemNamespace), kubeSystemNamespace); err != nil {
		return nil, fmt.Errorf("unable to retrieve kube-system namespace in determineGitOpsEngineInstanceForNewApplication")
	}

	gitopsEngineInstance, _, err := sharedutil.UncheckedGetOrCreateGitopsEngineInstanceByInstanceNamespaceUID(ctx, *namespace, string(kubeSystemNamespace.UID), dbq, actionLog)
	if err != nil {
		return nil, fmt.Errorf("unable to get or create engine instance for new application: %v", err)
	}

	// When we support multiple Argo CD instance, the algorithm would be:
	//
	// algorithm input:
	// - user
	// - managed environment
	//
	// output:
	// - gitops engine instance
	//
	// In a way that ensures that applications are balanced between instances.
	// Preliminary thoughts: https://docs.google.com/document/d/15E8d5frNuTFEdCHMlNSk0LQr6DI7BtiypxIC2AnW-OQ/edit#

	return gitopsEngineInstance, nil
}

func getOrCreateClusterAccess(ctx context.Context, ca *db.ClusterAccess, dbq db.DatabaseQueries) error {

	if err := dbq.GetClusterAccessByPrimaryKey(ctx, ca); err != nil {

		if !db.IsResultNotFoundError(err) {
			return err
		}
	} else {
		return nil
	}

	if err := dbq.CreateClusterAccess(ctx, ca); err != nil {
		return err
	}

	return nil
}

func getOrCreateClusterUserByNamespaceUID(ctx context.Context, namespaceUID string, dbq db.DatabaseQueries) (*db.ClusterUser, error) {

	// TODO: GITOPS-1577 - KCP support: for now, we assume that the namespace UID that the request occurred in is the user id.
	clusterUser := db.ClusterUser{User_name: namespaceUID}

	err := dbq.GetClusterUserByUsername(ctx, &clusterUser)
	if err != nil {
		if db.IsResultNotFoundError(err) {

			if err := dbq.CreateClusterUser(ctx, &clusterUser); err != nil {
				return nil, err
			}

		} else {
			return nil, err
		}

	}

	return &clusterUser, nil
}

func getWorkspaceIDFromNamespaceID(namespace v1.Namespace) string {
	// Here we assume that the namespace UID is the same as the workspace UID. If/when that changes, this should be updated.
	return string(namespace.UID)
}

type argoCDSpecInput struct {
	crName      string
	crNamespace string

	destinationNamespace string
	destinationName      string

	sourceRepoURL        string
	sourcePath           string
	sourceTargetRevision string
}

func createSpecField(fieldsParam argoCDSpecInput) string {

	sanitize := func(input string) string {
		input = strings.ReplaceAll(input, "\"", "")
		input = strings.ReplaceAll(input, "'", "")
		input = strings.ReplaceAll(input, "`", "")
		input = strings.ReplaceAll(input, "\r", "")
		input = strings.ReplaceAll(input, "\n", "")
		input = strings.ReplaceAll(input, "&", "")
		input = strings.ReplaceAll(input, ";", "")

		return input
	}

	fields := argoCDSpecInput{
		// MAKE SURE YOU SANITIZE ANY NEW FIELDS THAT ARE ADDED!!!!
		crName:               sanitize(fieldsParam.crName),
		crNamespace:          sanitize(fieldsParam.crNamespace),
		destinationNamespace: sanitize(fieldsParam.destinationNamespace),
		// MAKE SURE YOU SANITIZE ANY NEW FIELDS THAT ARE ADDED!!!!
		destinationName:      sanitize(fieldsParam.destinationName),
		sourceRepoURL:        sanitize(fieldsParam.sourceRepoURL),
		sourcePath:           sanitize(fieldsParam.sourcePath),
		sourceTargetRevision: sanitize(fieldsParam.sourceTargetRevision),
		// MAKE SURE YOU SANITIZE ANY NEW FIELDS THAT ARE ADDED!!!!
	}

	text := `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: \"` + fields.crName + `\"
  namespace: \"` + fields.crNamespace + `\"
spec:
  destination:
    name: \"` + fields.destinationName + `\"
    namespace: \"` + fields.destinationNamespace + `\"
  project: default
  source:
    path: ` + fields.sourcePath + `
    repoURL: ` + fields.sourceRepoURL + `
    targetRevision: "` + fields.sourceTargetRevision + `"`

	return text
}

func stringOperation(obj *db.Operation) string {
	if obj == nil {
		return "(nil)"
	}

	return fmt.Sprintf("id: %s, resource: %s [%s], status-update: %v", obj.Operation_id, obj.Resource_type, obj.Resource_id, obj.Last_state_update)
}

// stringEventLoopEvent is a utility function for debug purposes.
func stringEventLoopEvent(obj *eventLoopEvent) string {
	if obj == nil {
		return "(nil)"
	}

	return fmt.Sprintf("[%s] %s/%s/%s, for %s", obj.eventType, obj.request.Namespace, obj.request.Name, string(obj.reqResource), obj.workspaceID)

}
