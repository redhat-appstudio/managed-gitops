package eventloop

import (
	"context"
	"fmt"
	"github.com/redhat-appstudio/managed-gitops/backend/condition"
	"strings"
	"time"

	"github.com/go-logr/logr"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/util/fauxargocd"
	goyaml "gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// For more information on how events are distributed between goroutines by event loop, see:
// https://miro.com/app/board/o9J_lgiqJAs=/?moveToWidget=3458764514216218600&cot=14

type GitOpsDeploymentAdapter struct {
	gitOpsDeployment *managedgitopsv1alpha1.GitOpsDeployment
	logger           logr.Logger
	client           client.Client
	conditionManager condition.Conditions
	ctx              context.Context
}

func NewGitOpsDeploymentAdapter(gitopsDeployment *managedgitopsv1alpha1.GitOpsDeployment, logger logr.Logger, client client.Client, manager condition.Conditions, ctx context.Context) *GitOpsDeploymentAdapter {
	return &GitOpsDeploymentAdapter{
		gitOpsDeployment: gitopsDeployment,
		logger:           logger,
		client:           client,
		conditionManager: manager,
		ctx:              ctx,
	}
}

func getMatchingGitOpsDeployment(name, namespace string, client client.Client) (*managedgitopsv1alpha1.GitOpsDeployment, error) {
	gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, gitopsDepl)

	if err != nil {
		return &managedgitopsv1alpha1.GitOpsDeployment{}, err
	}

	return gitopsDepl, nil
}

// SetGitOpsDeploymentCondition calls SetCondition() with GitOpsDeployment conditions
func (g *GitOpsDeploymentAdapter) SetGitOpsDeploymentCondition(conditionType managedgitopsv1alpha1.GitOpsDeploymentConditionType, reason managedgitopsv1alpha1.GitOpsDeploymentReasonType, errMessage error) error {
	conditions := &g.gitOpsDeployment.Status.Conditions

	// Create a new condition and update the object in k8s with the error message, if err does exist
	if errMessage != nil {
		g.conditionManager.SetCondition(conditions, conditionType, managedgitopsv1alpha1.GitOpsConditionStatus(corev1.ConditionTrue), reason, errMessage.Error())
		return g.client.Status().Update(g.ctx, g.gitOpsDeployment, &client.UpdateOptions{})
	} else {
		// if error does not exist, check if the condition exists or not
		if g.conditionManager.HasCondition(conditions, conditionType) {
			reason = reason + "Resolved"
			// Check the condition and mark it as resolved, if it's resolved
			if cond, _ := g.conditionManager.FindCondition(conditions, conditionType); cond.Reason != reason {
				g.conditionManager.SetCondition(conditions, conditionType, managedgitopsv1alpha1.GitOpsConditionStatus(corev1.ConditionFalse), reason, "")
				return g.client.Status().Update(g.ctx, g.gitOpsDeployment, &client.UpdateOptions{})
			}
			// do nothing, if the condition is already marked as resolved
		}
		// do nothing, if the condition does not exist anymore
	}

	return nil
}

func newApplicationEventLoopRunner(informWorkCompleteChan chan applicationEventLoopMessage, sharedResourceEventLoop *sharedResourceEventLoop,
	gitopsDeplUID string, workspaceID string, debugContext string) chan *eventLoopEvent {

	inputChannel := make(chan *eventLoopEvent)

	go func() {
		applicationEventLoopRunner(inputChannel, informWorkCompleteChan, sharedResourceEventLoop, gitopsDeplUID, workspaceID, debugContext)
	}()

	return inputChannel

}

func applicationEventLoopRunner(inputChannel chan *eventLoopEvent, informWorkCompleteChan chan applicationEventLoopMessage,
	sharedResourceEventLoop *sharedResourceEventLoop, gitopsDeplUID string, workspaceID string, debugContext string) {

	outerContext := context.Background()
	log := log.FromContext(outerContext)

	log = log.WithValues("gitopsDeplUID", gitopsDeplUID, "workspaceID", workspaceID, "debugContext", debugContext)

	log.V(sharedutil.LogLevel_Debug).Info("applicationEventLoopRunner started")

	signalledShutdown := false

	for {
		// Read from input channel: wait for an event on this application
		newEvent := <-inputChannel

		ctx, cancel := context.WithCancel(outerContext)

		defer cancel()

		// Process the event

		log.V(sharedutil.LogLevel_Debug).Info("applicationEventLoopRunner - event received", "event", stringEventLoopEvent(newEvent))

		attempts := 1
		backoff := sharedutil.ExponentialBackoff{Min: time.Duration(100 * time.Millisecond), Max: time.Duration(15 * time.Second), Factor: 2, Jitter: true}
	inner_for:
		for {

			log.V(sharedutil.LogLevel_Debug).Info("applicationEventLoopRunner - processing event", "event", stringEventLoopEvent(newEvent), "attempt", attempts)

			// Break if the request is cancelled, or the timeout expires
			select {
			case <-ctx.Done():
				break inner_for
			default:
			}

			_, err := sharedutil.CatchPanic(func() error {

				action := applicationEventLoopRunner_Action{
					getK8sClientForGitOpsEngineInstance: actionGetK8sClientForGitOpsEngineInstance,
					eventResourceName:                   newEvent.request.Name,
					eventResourceNamespace:              newEvent.request.Namespace,
					workspaceClient:                     newEvent.client,
					sharedResourceEventLoop:             sharedResourceEventLoop,
					log:                                 log,
					workspaceID:                         workspaceID,
				}

				var err error

				dbQueriesUnscoped, err := db.NewProductionPostgresDBQueries(false)
				if err != nil {
					return fmt.Errorf("unable to access database in workspaceEventLoopRunner: %v", err)
				}

				scopedDBQueries, ok := dbQueriesUnscoped.(db.ApplicationScopedQueries)
				if !ok {
					return fmt.Errorf("SEVERE: unexpected cast failure")
				}

				if newEvent.eventType == DeploymentModified {
					// Handle all GitOpsDeployment related events
					signalledShutdown, _, _, err = action.applicationEventRunner_handleDeploymentModified(ctx, scopedDBQueries)

					// Get the GitOpsDeployment object from k8s, so we can update it if necessary
					gitopsDepl, clientError := getMatchingGitOpsDeployment(newEvent.request.Name, newEvent.request.Namespace, newEvent.client)
					if clientError != nil {
						return fmt.Errorf("couldn't fetch the GitOpsDeployment instance: %v", clientError)
					}

					// Create a GitOpsDeploymentAdapter to plug any conditions
					conditionManager := condition.NewConditionManager()
					adapter := NewGitOpsDeploymentAdapter(gitopsDepl, log, newEvent.client, conditionManager, ctx)

					// Plug any conditions based on the "err" msg
					if setConditionError := adapter.SetGitOpsDeploymentCondition(managedgitopsv1alpha1.GitOpsDeploymentConditionErrorOccurred, managedgitopsv1alpha1.GitopsDeploymentReasonErrorOccurred, err); setConditionError != nil {
						return setConditionError
					}

				} else if newEvent.eventType == SyncRunModified {
					// Handle all SyncRun related events
					signalledShutdown, err = action.applicationEventRunner_handleSyncRunModified(ctx, scopedDBQueries)

				} else if newEvent.eventType == UpdateDeploymentStatusTick {
					err = action.applicationEventRunner_handleUpdateDeploymentStatusTick(ctx, newEvent.associatedGitopsDeplUID, scopedDBQueries)

				} else {
					log.Error(nil, "SEVERE: Unrecognized event type", "event type", newEvent.eventType)
				}

				// TODO: GITOPS-1582: Implement detection of workspace/api proxy delete, here, and handle cleanup

				return err

			})

			if err == nil {
				break inner_for
			} else {
				log.Error(err, "error from inner event handler in applicationEventLoopRunner", "event", stringEventLoopEvent(newEvent))
				backoff.DelayOnFail(ctx)
				attempts++
			}
		}

		// Inform the caller that we have completed a single unit of work
		informWorkCompleteChan <- applicationEventLoopMessage{messageType: applicationEventLoopMessageType_WorkComplete, event: newEvent, shutdownSignalled: signalledShutdown}

		// If the event processing logic concluded that the goroutine should shutdown, then break out of the outer for loop.
		// This is usually because the API CR no longer exists.
		if signalledShutdown {
			break
		}
	}

	log.Info("ApplicationEventLoopRunner goroutine terminated.", "signalledShutdown", signalledShutdown)
}

func (a *applicationEventLoopRunner_Action) applicationEventRunner_handleSyncRunModified(ctx context.Context, dbQueries db.ApplicationScopedQueries) (bool, error) {

	log := a.log

	namespace := corev1.Namespace{}
	if err := a.workspaceClient.Get(ctx, types.NamespacedName{Namespace: a.eventResourceNamespace, Name: a.eventResourceNamespace}, &namespace); err != nil {
		return false, fmt.Errorf("unable to retrieve namespace '%s': %v", a.eventResourceNamespace, err)
	}

	clusterUser, err := a.sharedResourceEventLoop.getOrCreateClusterUserByNamespaceUID(ctx, a.workspaceClient, namespace)
	if err != nil {
		return false, fmt.Errorf("unable to retrieve cluster user in handleDeploymentModified, '%s': %v", string(namespace.UID), err)
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
				log.Error(err, "unable to locate object in handleSyncRunModified", "request", syncRunKey)
				return false, err
			}
		}
	}

	var apiCRToDBList []db.APICRToDatabaseMapping
	dbEntryExists := false
	if syncRunCRExists {
		mapping := db.APICRToDatabaseMapping{
			APIResourceType: db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
			APIResourceUID:  string(syncRunCR.UID),
			DBRelationType:  db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
		}

		if err := dbQueries.GetDatabaseMappingForAPICR(ctx, &mapping); err != nil {
			if db.IsResultNotFoundError(err) {
				// No corresponding entry
				dbEntryExists = false
			} else {
				// A generic error occured, so just return
				log.Error(err, "unable to resource APICRToDatabaseMapping", "uid", string(syncRunCR.UID))
				return false, err
			}
		} else {
			// Match found in database
			apiCRToDBList = append(apiCRToDBList, mapping)
		}
	} else {
		// The CR no longer exists (it was likely deleted), so instead we retrieve the UID of the SyncRun from
		// the APICRToDatabaseMapping table, by combination of (name/namespace/workspace).

		if err := dbQueries.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx, db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
			a.eventResourceName, a.eventResourceNamespace, getWorkspaceIDFromNamespaceID(namespace),
			db.APICRToDatabaseMapping_DBRelationType_SyncOperation, &apiCRToDBList); err != nil {

			log.Error(err, "unable to find API CR to DB Mapping, by API name/namespace/uid",
				"name", a.eventResourceName, "namespace", a.eventResourceNamespace, "UID", string(namespace.UID))
			return false, err
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

	log.Info("workspacerEventLoopRunner_handleSyncRunModified", "syncRunCRExists", syncRunCRExists, "dbEntryExists", dbEntryExists)

	if !syncRunCRExists && !dbEntryExists {
		log.Info("neither sync run CR exists, nor db entry, so our work is done.")
		// if neither exists, our work is done
		return true, nil
	}

	// The applications and gitopsengineinstance pointed to by the gitopsdeployment (if they are non-nil)
	var application *db.Application
	var gitopsEngineInstance *db.GitopsEngineInstance

	if syncRunCRExists {
		// Sanity check that the gitopsdeployment resource exists, which is referenced by the syncrun resource
		gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      syncRunCR.Spec.GitopsDeploymentName,
				Namespace: syncRunCR.Namespace,
			},
		}
		// Retrieve the GitOpsDeployment, and locate the corresponding application and gitopsengineinstance
		if err := a.workspaceClient.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl); err != nil {

			if apierr.IsNotFound(err) {
				// If the gitopsdepl doesn't exist, we really can't proceed any further

				err := fmt.Errorf("unable to retrieve gitopsdeployment referenced in syncrun: %v", err)
				// TODO: GITOPS-1720 - ENHANCEMENT - If the gitopsDepl isn't referenced, update the status of the GitOpsDeplomentSyncRun condition as an error and return
				// TODO: GITOPS-1720 - ENHANCEMENT - implement status conditions on GitOpsDeploymentSyncRun
				log.Error(err, "handleSyncRunModified error")
				return false, err

			}

			// If there was a generic error in retrieving the key, return it
			log.Error(err, "unable to retrieve gitopsdeployment referenced in syncrun")
			return false, err
		}

		// The GitopsDepl CR exists, so use the UID of the CR to retrieve the database entry, if possible
		deplToAppMapping := &db.DeploymentToApplicationMapping{Deploymenttoapplicationmapping_uid_id: string(gitopsDepl.UID)}

		if err = dbQueries.GetDeploymentToApplicationMappingByDeplId(ctx, deplToAppMapping); err != nil {
			log.Error(err, "unable to retrieve deployment to application mapping, on sync run modified", "uid", string(gitopsDepl.UID))
			return false, err
		}

		application = &db.Application{Application_id: deplToAppMapping.Application_id}
		if err := dbQueries.GetApplicationById(ctx, application); err != nil {
			log.Error(err, "unable to retrieve application, on sync run modified", "applicationId", string(deplToAppMapping.Application_id))
			return false, err
		}

		if gitopsEngineInstance, err = a.sharedResourceEventLoop.getGitopsEngineInstanceById(ctx, application.Engine_instance_inst_id, a.workspaceClient, namespace); err != nil {
			log.Error(err, "unable to retrieve gitopsengineinstance, on sync run modified", "instanceId", string(application.Engine_instance_inst_id))
			return false, err
		}

	}

	if syncRunCRExists && !dbEntryExists {
		// Handle create:
		// If the gitopsdeplsyncrun CR exists, but the database entry doesn't, then this is the first time we
		// have seen the GitOpsDeplSyncRun CR.
		// Create it in the DB and create the operation.

		if application == nil || gitopsEngineInstance == nil {
			err := fmt.Errorf("app or engine instance were nil in handleSyncRunModified app: %v, instance: %v", application, gitopsEngineInstance)
			log.Error(err, "unexpected nil value of required objects")
			return false, err
		}

		// createdResources is a list of database entries created in this function; if an error occurs, we delete them
		// in reverse order.
		var createdResources []db.AppScopedDisposableResource

		// Create sync operation
		syncOperation := &db.SyncOperation{
			Application_id:      application.Application_id,
			Operation_id:        "delme", // TODO: GITOPS-1678 - DEBT - This field can probably be removed from the database
			DeploymentNameField: syncRunCR.Spec.GitopsDeploymentName,
			Revision:            syncRunCR.Spec.RevisionID,
			DesiredState:        db.SyncOperation_DesiredState_Running,
		}
		if err := dbQueries.CreateSyncOperation(ctx, syncOperation); err != nil {
			log.Error(err, "unable to create sync operation in database")
			return false, err
		}
		createdResources = append(createdResources, syncOperation)

		newApiCRToDBMapping := db.APICRToDatabaseMapping{
			APIResourceType: db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
			APIResourceUID:  string(syncRunCR.UID),
			DBRelationType:  db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
			DBRelationKey:   syncOperation.SyncOperation_id,

			APIResourceName:      syncRunCR.Name,
			APIResourceNamespace: syncRunCR.Namespace,
			WorkspaceUID:         getWorkspaceIDFromNamespaceID(namespace),
		}
		if err := dbQueries.CreateAPICRToDatabaseMapping(ctx, &newApiCRToDBMapping); err != nil {
			log.Error(err, "unable to create api to db mapping in database")

			// If we were unable to retrieve the client, delete the resources we created in the previous steps
			dbutil.DisposeApplicationScopedResources(ctx, createdResources, dbQueries, log)

			return false, err
		}
		createdResources = append(createdResources, &newApiCRToDBMapping)

		operationClient, err := a.getK8sClientForGitOpsEngineInstance(gitopsEngineInstance)
		if err != nil {
			log.Error(err, "unable to retrieve gitopsengine instance from handleSyncRunModified")

			// If we were unable to retrieve the client, delete the resources we created in the previous steps
			dbutil.DisposeApplicationScopedResources(ctx, createdResources, dbQueries, log)

			// Return the original error
			return false, err
		}

		dbOperationInput := db.Operation{
			Instance_id:   gitopsEngineInstance.Gitopsengineinstance_id,
			Resource_id:   syncOperation.SyncOperation_id,
			Resource_type: db.OperationResourceType_SyncOperation,
		}

		k8sOperation, dbOperation, err := CreateOperation(ctx, false && !a.testOnlySkipCreateOperation, dbOperationInput, clusterUser.Clusteruser_id,
			dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, operationClient, log)
		if err != nil {
			log.Error(err, "could not create operation", "namespace", dbutil.GetGitOpsEngineSingleInstanceNamespace())

			// If we were unable to create the operation, delete the resources we created in the previous steps
			dbutil.DisposeApplicationScopedResources(ctx, createdResources, dbQueries, log)

			return false, err
		}

		// TODO: GITOPS-1466 - STUB - Remove the 'false' in createOperation above, once cluster agent handling of operation is implemented.
		log.Info("STUB: Not waiting for create Sync Run operation to complete, in handleNewSyncRunModified")

		if err := cleanupOperation(ctx, *dbOperation, *k8sOperation, dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, operationClient, log); err != nil {
			return false, err
		}

		return false, nil
	}

	if !syncRunCRExists && dbEntryExists {
		// Handle delete:
		// If the gitopsdeplsyncrun CR doesn't exist, but database row does, then the CR has been deleted, so handle it.

		// Deleting the CR should terminate a sync operation, if it was previously in progress.

		if len(apiCRToDBList) != 1 {
			err := fmt.Errorf("SEVERE - Update only supports one operation parameter")
			log.Error(err, err.Error())
			return false, err
		}

		// 1) Get the SyncOperation table entry pointed to by the resource
		apiCRToDBMapping := apiCRToDBList[0]

		if apiCRToDBMapping.DBRelationType != db.APICRToDatabaseMapping_DBRelationType_SyncOperation {
			err := fmt.Errorf("SEVERE - db relation type should be syncoperation")
			log.Error(err, err.Error())
			return false, err
		}

		syncOperation := db.SyncOperation{SyncOperation_id: apiCRToDBMapping.DBRelationKey}

		if err := dbQueries.GetSyncOperationById(ctx, &syncOperation); err != nil {
			log.Error(err, "unable to retrieve sync operation by id on deleted", "operationID", syncOperation.SyncOperation_id)
			return false, err
		}

		// 2) Update the state of the SyncOperation DB table to say that we want to terminate it, if it is runing
		syncOperation.DesiredState = db.SyncOperation_DesiredState_Terminated

		dbOperationInput := db.Operation{
			Instance_id:   gitopsEngineInstance.Gitopsengineinstance_id,
			Resource_id:   syncOperation.SyncOperation_id,
			Resource_type: db.OperationResourceType_SyncOperation,
		}

		// 3) Create the operation, in order to inform the cluster agent it needs to cancel the sync operation
		operationClient, err := a.getK8sClientForGitOpsEngineInstance(gitopsEngineInstance)
		if err != nil {
			log.Error(err, "unable to retrieve gitopsengine instance from handleSyncRunModified, when resource was deleted")
			return false, err
		}

		k8sOperation, dbOperation, err := CreateOperation(ctx, true && !a.testOnlySkipCreateOperation, dbOperationInput, clusterUser.Clusteruser_id,
			dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, operationClient, log)
		if err != nil {
			log.Error(err, "could not create operation, when resource was deleted", "namespace", dbutil.GetGitOpsEngineSingleInstanceNamespace())
			return false, err
		}

		// 4) Clean up the operation and database table entries
		if err := cleanupOperation(ctx, *dbOperation, *k8sOperation, dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, operationClient, log); err != nil {
			return false, err
		}

		// TODO: GITOPS-1466 - STUB - need to implement support for sync operation in cluster agent
		log.Info("STUB: need to implement sync on cluster side")

		if _, err := dbQueries.DeleteSyncOperationById(ctx, syncOperation.SyncOperation_id); err != nil {
			log.Error(err, "could not delete sync operation, when resource was deleted", "namespace", dbutil.GetGitOpsEngineSingleInstanceNamespace())
			return false, err
		}

		var allErrors error

		// Remove the mappings
		for idx := range apiCRToDBList {

			apiCRToDB := apiCRToDBList[idx]

			err := a.cleanupOldSyncDBEntry(ctx, &apiCRToDB, *clusterUser, dbQueries)
			if err != nil {
				if allErrors == nil {
					allErrors = err
				} else {
					allErrors = fmt.Errorf("error: %v error: %v", err, allErrors)
				}
			}
		}

		if allErrors != nil {
			return false, allErrors
		}

		// Success: the CR no longer exists, and we have completed cleanup, so signal that the goroutine may be terminated.
		return true, nil
	}

	if syncRunCRExists && dbEntryExists {

		// Sanity checks
		if syncRunCR == nil {
			err := fmt.Errorf("SEVERE - vsync run cr is nil")
			log.Error(err, err.Error())
			return false, err
		}

		if len(apiCRToDBList) != 1 {
			err := fmt.Errorf("SEVERE - Update only supports one operation parameter")
			log.Error(err, err.Error())
			return false, err
		}

		// Get the SyncOperation table entry pointed to by the resource
		apiCRToDBMapping := apiCRToDBList[0]

		if apiCRToDBMapping.DBRelationType != db.APICRToDatabaseMapping_DBRelationType_SyncOperation {
			err := fmt.Errorf("SEVERE - db relation type should be syncoperation")
			log.Error(err, err.Error())
			return false, err
		}

		syncOperation := db.SyncOperation{SyncOperation_id: apiCRToDBMapping.DBRelationKey}

		if err := dbQueries.GetSyncOperationById(ctx, &syncOperation); err != nil {

			log.Error(err, "unable to retrieve sync operation by id on modified", "operationID", syncOperation.SyncOperation_id)
			return false, err
		}

		if syncOperation.DeploymentNameField != syncRunCR.Spec.GitopsDeploymentName {
			err := fmt.Errorf("deployment name field is immutable: changing it from its initial value is not supported")
			log.Error(err, "deployment name field change is not supported")
			return false, err
		}

		if syncOperation.Revision != syncRunCR.Spec.RevisionID {
			err := fmt.Errorf("revision change is not supported: changing it from its initial value is not supported")
			log.Error(err, "revision field change is not supported")
			return false, err
		}

		// TODO: GITOPS-1678 - DEBT - Include test case to check that the various goroutines are terminated when the CR is deleted.

		return false, nil
	}

	return false, nil

}

func (a *applicationEventLoopRunner_Action) cleanupOldSyncDBEntry(ctx context.Context, apiCRToDB *db.APICRToDatabaseMapping,
	clusterUser db.ClusterUser, dbQueries db.ApplicationScopedQueries) error {

	log := a.log

	if apiCRToDB.DBRelationType != db.APICRToDatabaseMapping_DBRelationType_SyncOperation {
		err := fmt.Errorf("SEVERE: unexpected DBRelationKey, should be SyncOperation")
		log.Error(err, err.Error())
		return err
	}

	rowsDeleted, err := dbQueries.DeleteSyncOperationById(ctx, apiCRToDB.DBRelationKey)
	if err != nil {
		log.Error(err, "unable to delete sync operation db entry on sync operation delete", "key", apiCRToDB.DBRelationKey)
		return err
	} else if rowsDeleted == 0 {
		log.V(sharedutil.LogLevel_Warn).Error(err, "unexpected number of rows deleted on sync db entry delete", "key", apiCRToDB.DBRelationKey)
	}

	var operations []db.Operation
	if err := dbQueries.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, apiCRToDB.DBRelationKey, apiCRToDB.DBRelationType, &operations, clusterUser.Clusteruser_id); err != nil {
		log.Error(err, "unable to retrieve operations pointing to sync operation", "key", apiCRToDB.DBRelationKey)
		return err
	} else {
		// Delete the operations that reference this SyncOperation
		for idx := range operations {
			operationId := operations[idx].Operation_id

			log := log.WithValues("operationId", operationId)

			rowsDeleted, err := dbQueries.CheckedDeleteOperationById(ctx, operationId, clusterUser.Clusteruser_id)
			if err != nil {
				log.Error(err, "unable to delete old operation")
				return err
			} else if rowsDeleted == 0 {
				log.V(sharedutil.LogLevel_Warn).Error(err, "unexpected number of deleted rows when deleting old operation")
			}
		}
	}

	rowsDeleted, err = dbQueries.DeleteAPICRToDatabaseMapping(ctx, apiCRToDB)
	if err != nil {
		log.Error(err, "unable to delete apiCRToDBmapping", "mapping", apiCRToDB.APIResourceUID)
		return err

	} else if rowsDeleted == 0 {
		log.V(sharedutil.LogLevel_Warn).Error(err, "unexpected number of rows deleted of apiCRToDBmapping", "mapping", apiCRToDB.APIResourceUID)
	}

	return nil
}

// applicationEventRunner_handleUpdateDeploymentStatusTick updates the status field of all the GitOpsDeploymentCRs in the workspace.
func (a *applicationEventLoopRunner_Action) applicationEventRunner_handleUpdateDeploymentStatusTick(ctx context.Context,
	gitopsDeplID string, dbQueries db.ApplicationScopedQueries) error {

	// TODO: GITOPS-1702 - PERF - In general, polling for all GitOpsDeployments in a workspace will scale poorly with large number of applications in the workspace. We should switch away from polling in the future.

	// 1) Retrieve the mapping for the CR we are processing
	mapping := db.DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: string(gitopsDeplID),
	}
	if err := dbQueries.GetDeploymentToApplicationMappingByDeplId(ctx, &mapping); err != nil {
		if db.IsResultNotFoundError(err) {
			// Our work is done
			return nil
		} else {
			a.log.Error(err, "unable to locate dtam in handleUpdateDeploymentStatusTick")
			return err
		}
	}

	// 2) Retrieve the GitOpsDeployment from the namespace, using the expected values from the database
	gitopsDeployment := &managedgitopsv1alpha1.GitOpsDeployment{}
	{
		gitopsDeploymentKey := client.ObjectKey{Namespace: mapping.DeploymentNamespace, Name: mapping.DeploymentName}

		if err := a.workspaceClient.Get(ctx, gitopsDeploymentKey, gitopsDeployment); err != nil {

			if apierr.IsNotFound(err) {
				// Our work is done
				return nil
			} else {
				a.log.Error(err, "unable to locate gitops object in handleUpdateDeploymentStatusTick", "request", gitopsDeploymentKey)
				return err
			}
		}
	}

	if string(gitopsDeployment.UID) != mapping.Deploymenttoapplicationmapping_uid_id {
		// This can occur if the GitOpsDeployment CR has been deleted from the workspace
		a.log.V(sharedutil.LogLevel_Warn).Info("On tick, the UID of the gitopsdeployment in the workspace differed from the tick value: " + string(gitopsDeployment.UID) + " " + mapping.Deploymenttoapplicationmapping_uid_id)
		return nil
	}

	// 3) Retrieve the application state for the application pointed to be the depltoappmapping
	applicationState := db.ApplicationState{Applicationstate_application_id: mapping.Application_id}
	if err := dbQueries.GetApplicationStateById(ctx, &applicationState); err != nil {

		if db.IsResultNotFoundError(err) {
			a.log.Info("ApplicationState not found for application, on deploymentStatusTick: " + applicationState.Applicationstate_application_id)
			return nil
		} else {
			return err
		}
	}

	// 4) update the health and status field of the GitOpsDepl CR

	// Update the local gitopsDeployment instance with health and status values (fetched from the database)
	gitopsDeployment.Status.Health.Status = managedgitopsv1alpha1.HealthStatusCode(applicationState.Health)
	gitopsDeployment.Status.Health.Message = applicationState.Message
	gitopsDeployment.Status.Sync.Status = managedgitopsv1alpha1.SyncStatusCode(applicationState.Sync_Status)
	gitopsDeployment.Status.Sync.Revision = applicationState.Revision

	// Update the actual object in Kubernetes
	if err := a.workspaceClient.Status().Update(ctx, gitopsDeployment, &client.UpdateOptions{}); err != nil {
		return err
	}

	// NOTE: make sure to preserve the existing conditions fields that are in the status field of the CR, when updating the status!

	return nil

}

func (a *applicationEventLoopRunner_Action) applicationEventRunner_handleDeploymentModified(ctx context.Context,
	dbQueries db.ApplicationScopedQueries) (bool, *db.Application, *db.GitopsEngineInstance, error) {

	deplName := a.eventResourceName
	deplNamespace := a.eventResourceNamespace
	workspaceClient := a.workspaceClient

	log := a.log

	gitopsDeplNamespace := corev1.Namespace{}
	if err := workspaceClient.Get(ctx, types.NamespacedName{Namespace: deplNamespace, Name: deplNamespace}, &gitopsDeplNamespace); err != nil {
		return false, nil, nil, fmt.Errorf("unable to retrieve namespace '%s': %v", deplNamespace, err)
	}

	clusterUser, err := a.sharedResourceEventLoop.getOrCreateClusterUserByNamespaceUID(ctx, workspaceClient, gitopsDeplNamespace)
	if err != nil {
		return false, nil, nil, fmt.Errorf("unable to retrieve cluster user in handleDeploymentModified, '%s': %v", string(gitopsDeplNamespace.UID), err)
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
				log.Error(err, "unable to locate object in handleDeploymentModified", "request", gitopsDeploymentKey)
				return false, nil, nil, err
			}
		}
	}

	// Next, retrieve the corresponding database for the gitopsdepl, if applicable
	deplToAppMapExistsInDB := false

	var deplToAppMappingList []db.DeploymentToApplicationMapping
	if gitopsDeploymentCRExists {
		// The CR exists, so use the UID of the CR to retrieve the database entry, if possible
		deplToAppMapping := &db.DeploymentToApplicationMapping{Deploymenttoapplicationmapping_uid_id: string(gitopsDeployment.UID)}

		if err = dbQueries.GetDeploymentToApplicationMappingByDeplId(ctx, deplToAppMapping); err != nil {

			if db.IsResultNotFoundError(err) {
				deplToAppMapExistsInDB = false
			} else {
				log.Error(err, "unable to retrieve deployment to application mapping", "uid", string(gitopsDeployment.UID))
				return false, nil, nil, err
			}

		} else {
			deplToAppMappingList = append(deplToAppMappingList, *deplToAppMapping)
			deplToAppMapExistsInDB = true
		}
	} else {
		// The CR no longer exists (it was likely deleted), so instead we retrieve the UID of the GitOpsDeployment from
		// the DeploymentToApplicationMapping table, by combination of (name/namespace/workspace).

		if err := dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(ctx, deplName, deplNamespace,
			getWorkspaceIDFromNamespaceID(gitopsDeplNamespace), &deplToAppMappingList); err != nil {

			log.Error(err, "unable to retrieve deployment to application mapping by name/namespace/uid",
				"name", deplName, "namespace", deplNamespace, "UID", string(gitopsDeplNamespace.UID))
			return false, nil, nil, err
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

	log.V(sharedutil.LogLevel_Debug).Info("workspacerEventLoopRunner_handleDeploymentModified processing event",
		"gitopsDeploymentExists", gitopsDeploymentCRExists, "deplToAppMapExists", deplToAppMapExistsInDB)

	if !gitopsDeploymentCRExists && !deplToAppMapExistsInDB {
		// if neither exists, our work is done
		return false, nil, nil, nil
	}

	if gitopsDeploymentCRExists && !deplToAppMapExistsInDB {
		// If the gitopsdepl CR exists, but the database entry doesn't,
		// then this is the first time we have seen the GitOpsDepl CR.
		// Create it in the DB and create the operation.
		return a.handleNewGitOpsDeplEvent(ctx, gitopsDeployment, clusterUser, dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries)
	}

	if !gitopsDeploymentCRExists && deplToAppMapExistsInDB {
		// If the gitopsdepl CR doesn't exist, but the database row does, then the CR has been deleted, so handle it.
		signalShutdown, err := a.handleDeleteGitOpsDeplEvent(ctx, clusterUser, dbutil.GetGitOpsEngineSingleInstanceNamespace(),
			&deplToAppMappingList, dbQueries)

		return signalShutdown, nil, nil, err
	}

	if gitopsDeploymentCRExists && deplToAppMapExistsInDB {

		if len(deplToAppMappingList) != 1 {
			err := fmt.Errorf("SEVERE - Update only supports one operation parameter")
			log.Error(err, err.Error())
			return false, nil, nil, err
		}

		// if both exist: it's an update (or a no-op)
		return a.handleUpdatedGitOpsDeplEvent(ctx, &deplToAppMappingList[0], gitopsDeployment, clusterUser,
			dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries)
	}

	return false, nil, nil, fmt.Errorf("SEVERE - All cases should be handled by above if statements")
}

func (a applicationEventLoopRunner_Action) handleDeleteGitOpsDeplEvent(ctx context.Context, clusterUser *db.ClusterUser,
	operationNamespace string, deplToAppMappingList *[]db.DeploymentToApplicationMapping, dbQueries db.ApplicationScopedQueries) (bool, error) {

	if deplToAppMappingList == nil || clusterUser == nil {
		return false, fmt.Errorf("required parameter should not be nil in handleDelete: %v %v", deplToAppMappingList, clusterUser)
	}

	workspaceNamespace := corev1.Namespace{}
	if err := a.workspaceClient.Get(ctx, types.NamespacedName{Namespace: a.eventResourceNamespace, Name: a.eventResourceNamespace}, &workspaceNamespace); err != nil {
		return false, fmt.Errorf("unable to retrieve workspace namespace")
	}

	var allErrors error

	signalShutdown := true

	// For each of the database entries that reference this gitopsdepl
	for idx := range *deplToAppMappingList {

		deplToAppMapping := (*deplToAppMappingList)[idx]

		// Clean up the database entries
		itemSignalledShutdown, err := a.cleanOldGitOpsDeploymentEntry(ctx, &deplToAppMapping, clusterUser, operationNamespace, workspaceNamespace, dbQueries)
		if err != nil {
			signalShutdown = false

			if allErrors == nil {
				allErrors = fmt.Errorf("(error: %v)", err)
			} else {
				allErrors = fmt.Errorf("(error: %v) %v", err, allErrors)
			}
		}

		signalShutdown = signalShutdown && itemSignalledShutdown
	}

	return signalShutdown, allErrors
}

func (a applicationEventLoopRunner_Action) cleanOldGitOpsDeploymentEntry(ctx context.Context, deplToAppMapping *db.DeploymentToApplicationMapping,
	clusterUser *db.ClusterUser, operationNamespace string, workspaceNamespace corev1.Namespace, dbQueries db.ApplicationScopedQueries) (bool, error) {

	dbApplicationFound := true

	// If the gitopsdepl CR doesn't exist, but database row does, then the CR has been deleted, so handle it.
	dbApplication := db.Application{
		Application_id: deplToAppMapping.Application_id,
	}
	if err := dbQueries.GetApplicationById(ctx, &dbApplication); err != nil {

		a.log.Error(err, "unable to get application by id", "id", deplToAppMapping.Application_id)

		if db.IsResultNotFoundError(err) {
			dbApplicationFound = false
			// Log the error, but continue.
		} else {
			return false, err
		}
	}

	log := a.log.WithValues("id", dbApplication.Application_id)

	// Remove the ApplicationState from the database
	rowsDeleted, err := dbQueries.DeleteApplicationStateById(ctx, deplToAppMapping.Application_id)
	if err != nil {

		log.V(sharedutil.LogLevel_Warn).Error(err, "unable to delete application state by id")
		return false, err

	} else if rowsDeleted == 0 {
		// Log the warning, but continue
		log.Info("no application rows deleted for application state", "rowsDeleted", rowsDeleted)
	}

	// Remove DeplToAppMapping
	rowsDeleted, err = dbQueries.DeleteDeploymentToApplicationMappingByDeplId(ctx, deplToAppMapping.Deploymenttoapplicationmapping_uid_id)
	if err != nil {
		log.Error(err, "unable to delete deplToAppMapping by id", "deplToAppMapUid", deplToAppMapping.Deploymenttoapplicationmapping_uid_id)
		return false, err

	} else if rowsDeleted == 0 {
		// Log the warning, but continue
		log.V(sharedutil.LogLevel_Warn).Error(nil, "unexpected number of rows deleted for deplToAppMapping", "rowsDeleted", rowsDeleted)
	}

	rowsUpdated, err := dbQueries.UpdateSyncOperationRemoveApplicationField(ctx, deplToAppMapping.Application_id)
	if err != nil {
		log.Error(err, "unable to update old sync operations", "applicationId", deplToAppMapping.Application_id)
		return false, err

	} else if rowsUpdated == 0 {
		log.Info("no sync operation rows updated, for updating old syncoperations on gitopsdepl deletion")
	}

	if !dbApplicationFound {
		log.Info("While cleaning up old gitopsdepl entries, db application wasn't found, id: " + deplToAppMapping.Application_id)
		// If the Application CR no longer exists, then our work is done.
		return true, nil
	}

	// If the Application table entry still exists, finish the cleanup...

	// Remove the Application from the database
	log.Info("deleting database Application, id: " + deplToAppMapping.Application_id)
	rowsDeleted, err = dbQueries.DeleteApplicationById(ctx, deplToAppMapping.Application_id)
	if err != nil {
		// Log the error, but continue
		log.Error(err, "unable to delete application by id", "appId", deplToAppMapping.Application_id)
	} else if rowsDeleted == 0 {
		// Log the error, but continue
		log.V(sharedutil.LogLevel_Warn).Error(nil, "unexpected number of rows deleted for application", "rowsDeleted", rowsDeleted, "appId", deplToAppMapping.Application_id)
	}

	gitopsEngineInstance, err := a.sharedResourceEventLoop.getGitopsEngineInstanceById(ctx, dbApplication.Engine_instance_inst_id, a.workspaceClient, workspaceNamespace)
	if err != nil {
		log := log.WithValues("id", dbApplication.Engine_instance_inst_id)

		if db.IsResultNotFoundError(err) {
			log.Error(err, "GitOpsEngineInstance could not be retrieved during gitopsdepl deletion handling")
			return false, err
		} else {
			log.Error(err, "Error occurred on attempting to retrieve gitops engine instance")
			return false, err
		}
	}

	// Create the operation that will delete the Argo CD application
	gitopsEngineClient, err := a.getK8sClientForGitOpsEngineInstance(gitopsEngineInstance)
	if err != nil {
		log.Error(err, "could not retrieve client for gitops engine instance", "instance", gitopsEngineInstance.Gitopsengineinstance_id)
		return false, err
	}
	dbOperationInput := db.Operation{
		Instance_id:   dbApplication.Engine_instance_inst_id,
		Resource_id:   deplToAppMapping.Application_id,
		Resource_type: db.OperationResourceType_Application,
	}

	k8sOperation, dbOperation, err := CreateOperation(ctx, true && !a.testOnlySkipCreateOperation, dbOperationInput,
		clusterUser.Clusteruser_id, operationNamespace, dbQueries, gitopsEngineClient, log)
	if err != nil {
		log.Error(err, "unable to create operation", "operation", dbOperationInput.ShortString())
		return false, err
	}

	if err := cleanupOperation(ctx, *dbOperation, *k8sOperation, operationNamespace, dbQueries, gitopsEngineClient, log); err != nil {
		log.Error(err, "unable to cleanup operation", "operation", dbOperationInput.ShortString())
		return false, err
	}

	return true, nil

}

func (a applicationEventLoopRunner_Action) handleUpdatedGitOpsDeplEvent(ctx context.Context, deplToAppMapping *db.DeploymentToApplicationMapping,
	gitopsDeployment *managedgitopsv1alpha1.GitOpsDeployment, clusterUser *db.ClusterUser, operationNamespace string,
	dbQueries db.ApplicationScopedQueries) (bool, *db.Application, *db.GitopsEngineInstance, error) {

	if deplToAppMapping == nil || gitopsDeployment == nil || clusterUser == nil {
		return false, nil, nil, fmt.Errorf("unexpected nil param in handleUpdatedGitOpsDeplEvent: %v %v %v", deplToAppMapping, gitopsDeployment, clusterUser)
	}

	log := a.log.WithValues("applicationId", deplToAppMapping.Application_id, "gitopsDeplUID", gitopsDeployment.UID)

	application := &db.Application{Application_id: deplToAppMapping.Application_id}
	if err := dbQueries.GetApplicationById(ctx, application); err != nil {
		if !db.IsResultNotFoundError(err) {
			log.Error(err, "unable to retrieve Application DB entry in handleUpdatedGitOpsDeplEvent")
			return false, nil, nil, err
		} else {

			// The application pointed to by the deplToAppMapping doesn't exist; this shouldn't happen.
			log.Error(err, "SEVERE: Application pointed to by deplToAppMapping doesn't exist, in handleUpdatedGitOpsDeplEvent")

			// Delete the deplToAppMapping, since the app doesn't exist. This should cause the gitopsdepl to be reconciled by the event loop.
			if _, err := dbQueries.DeleteDeploymentToApplicationMappingByDeplId(ctx, deplToAppMapping.Deploymenttoapplicationmapping_uid_id); err != nil {
				log.Error(err, "Unable to delete deplToAppMapping which pointed to non-existent Application, in handleUpdatedGitOpsDeplEvent")
				return false, nil, nil, err
			}
			return false, nil, nil, err
		}
	}

	workspaceNamespace := corev1.Namespace{}
	if err := a.workspaceClient.Get(ctx, types.NamespacedName{Namespace: a.eventResourceNamespace, Name: a.eventResourceNamespace}, &workspaceNamespace); err != nil {
		return false, nil, nil, fmt.Errorf("unable to retrieve workspace namespace")
	}

	engineInstanceParam, err := a.sharedResourceEventLoop.getGitopsEngineInstanceById(ctx, application.Engine_instance_inst_id, a.workspaceClient, workspaceNamespace)
	if err != nil {
		log.Error(err, "SEVERE: GitOps engine instance pointed to by Application doesn't exist, in handleUpdatedGitOpsDeplEvent", "engine-instance-id", engineInstanceParam.Gitopsengineinstance_id)
		return false, nil, nil, err
	}

	// TODO: GITOPS-1701 - ENHANCEMENT - Ensure that the backend code gracefully handles database values that are too large for the DB field (eg too many VARCHARs)

	// TODO: GITOPS-1678 - Sanity check that the application.name matches the expected value set in handleCreateGitOpsEvent

	destinationNamespace := gitopsDeployment.Spec.Destination.Namespace
	if destinationNamespace == "" {
		destinationNamespace = a.eventResourceNamespace
	}

	specFieldInput := argoCDSpecInput{
		crName:               application.Name,
		crNamespace:          engineInstanceParam.Namespace_name,
		destinationNamespace: destinationNamespace,
		// TODO: GITOPS-1722 - Fill this in with cluster credentials
		destinationName:      "in-cluster",
		sourceRepoURL:        gitopsDeployment.Spec.Source.RepoURL,
		sourcePath:           gitopsDeployment.Spec.Source.Path,
		sourceTargetRevision: gitopsDeployment.Spec.Source.TargetRevision,
		automated:            strings.EqualFold(gitopsDeployment.Spec.Type, managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated),
	}

	specFieldResult, err := createSpecField(specFieldInput)
	if err != nil {
		log.Error(err, "SEVERE: Unable to parse generated spec field")
		return false, nil, nil, err
	}

	if specFieldResult == application.Spec_field {
		log.Info("No spec change detected between Application DB entry and GitOpsDeployment CR")
		// No change required: the application database entry is consistent with the gitopsdepl CR
		return false, application, engineInstanceParam, nil
	}

	log.Info("Spec change detected between Application DB entry and GitOpsDeployment CR")

	application.Spec_field = specFieldResult

	if err := dbQueries.UpdateApplication(ctx, application); err != nil {
		log.Error(err, "Unable to update application, after mismatch detected")
		return false, nil, nil, err
	}
	// Create the operation
	gitopsEngineClient, err := a.getK8sClientForGitOpsEngineInstance(engineInstanceParam)
	if err != nil {
		log.Error(err, "unable to retrieve gitopsengineinstance for updated gitopsdepl", "gitopsEngineIstance", engineInstanceParam.EngineCluster_id)
		return false, nil, nil, err
	}

	dbOperationInput := db.Operation{
		Instance_id:   engineInstanceParam.Gitopsengineinstance_id,
		Resource_id:   application.Application_id,
		Resource_type: db.OperationResourceType_Application,
	}

	k8sOperation, dbOperation, err := CreateOperation(ctx, true && !a.testOnlySkipCreateOperation, dbOperationInput, clusterUser.Clusteruser_id,
		operationNamespace, dbQueries, gitopsEngineClient, log)
	if err != nil {
		log.Error(err, "could not create operation", "operation", dbOperation.Operation_id, "namespace", operationNamespace)
		return false, nil, nil, err
	}

	if err := cleanupOperation(ctx, *dbOperation, *k8sOperation, operationNamespace, dbQueries, gitopsEngineClient, log); err != nil {
		return false, nil, nil, err
	}

	return false, application, engineInstanceParam, nil

}

// Don't call this directly: call it via workspaceEventLoopRunner_Action
func actionGetK8sClientForGitOpsEngineInstance(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error) {

	// TODO: GITOPS-1455: When we support multiple Argo CD instances (and multiple instances on separate clusters), this logic should be updated.

	config, err := sharedutil.GetRESTConfig()
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	err = managedgitopsv1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = operation.AddToScheme(scheme)
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

func (a applicationEventLoopRunner_Action) handleNewGitOpsDeplEvent(ctx context.Context, gitopsDeployment *managedgitopsv1alpha1.GitOpsDeployment,
	clusterUser *db.ClusterUser, operationNamespace string, dbQueries db.ApplicationScopedQueries) (bool, *db.Application, *db.GitopsEngineInstance, error) {

	gitopsDeplNamespace := corev1.Namespace{}
	if err := a.workspaceClient.Get(ctx, types.NamespacedName{Namespace: gitopsDeployment.ObjectMeta.Namespace, Name: gitopsDeployment.ObjectMeta.Namespace}, &gitopsDeplNamespace); err != nil {
		return false, nil, nil, fmt.Errorf("unable to retrieve namespace for managed env, '%s': %v", gitopsDeployment.ObjectMeta.Namespace, err)
	}

	_, managedEnv, engineInstance, _, err := a.sharedResourceEventLoop.getOrCreateSharedResources(ctx, a.workspaceClient, gitopsDeplNamespace)

	if err != nil {
		a.log.Error(err, "unable to get or create required db entries on deployment modified event")
		return false, nil, nil, err
	}

	appName := "gitopsdepl-" + string(gitopsDeployment.UID)

	destinationNamespace := gitopsDeployment.Spec.Destination.Namespace
	if destinationNamespace == "" {
		destinationNamespace = a.eventResourceNamespace
	}

	specFieldInput := argoCDSpecInput{
		crName:               appName,
		crNamespace:          engineInstance.Namespace_name,
		destinationNamespace: destinationNamespace,
		// TODO: GITOPS-1722 - Fill this in with cluster credentials
		destinationName:      "in-cluster",
		sourceRepoURL:        gitopsDeployment.Spec.Source.RepoURL,
		sourcePath:           gitopsDeployment.Spec.Source.Path,
		sourceTargetRevision: gitopsDeployment.Spec.Source.TargetRevision,
		automated:            strings.EqualFold(gitopsDeployment.Spec.Type, managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated),
	}

	specFieldText, err := createSpecField(specFieldInput)
	if err != nil {
		a.log.Error(err, "SEVERE: unable to marshal generated YAML")
		return false, nil, nil, err
	}

	application := db.Application{
		Name:                    appName,
		Engine_instance_inst_id: engineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnv.Managedenvironment_id,
		Spec_field:              specFieldText,
	}

	a.log.Info("Creating new Application in DB: " + application.Application_id)

	if err := dbQueries.CreateApplication(ctx, &application); err != nil {
		a.log.Error(err, "unable to create application", "application", application, "ownerId", clusterUser.Clusteruser_id)
		return false, nil, nil, err
	}

	requiredDeplToAppMapping := &db.DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: string(gitopsDeployment.UID),
		Application_id:                        application.Application_id,
		DeploymentName:                        gitopsDeployment.Name,
		DeploymentNamespace:                   gitopsDeployment.Namespace,
		WorkspaceUID:                          getWorkspaceIDFromNamespaceID(gitopsDeplNamespace),
	}

	a.log.Info("Upserting new DeploymentToApplicationMapping in DB: " + requiredDeplToAppMapping.Deploymenttoapplicationmapping_uid_id)

	if err := dbutil.GetOrCreateDeploymentToApplicationMapping(ctx, requiredDeplToAppMapping, dbQueries, a.log); err != nil {
		a.log.Error(err, "unable to create deplToApp mapping", "deplToAppMapping", requiredDeplToAppMapping)
		return false, nil, nil, err
	}

	dbOperationInput := db.Operation{
		Instance_id:   engineInstance.Gitopsengineinstance_id,
		Resource_id:   application.Application_id,
		Resource_type: db.OperationResourceType_Application,
	}

	gitopsEngineClient, err := a.getK8sClientForGitOpsEngineInstance(engineInstance)
	if err != nil {
		return false, nil, nil, err
	}

	k8sOperation, dbOperation, err := CreateOperation(ctx, true && !a.testOnlySkipCreateOperation, dbOperationInput,
		clusterUser.Clusteruser_id, operationNamespace, dbQueries, gitopsEngineClient, a.log)
	if err != nil {
		a.log.Error(err, "could not create operation", "namespace", operationNamespace)
		return false, nil, nil, err
	}

	if err := cleanupOperation(ctx, *dbOperation, *k8sOperation, operationNamespace, dbQueries, gitopsEngineClient, a.log); err != nil {
		return false, nil, nil, err
	}

	return false, &application, engineInstance, nil
}

// applicationEventLoopRunner_Action is a short-lived struct containing data required to perform an action
// on the database, and/or on gitops engine cluster.
type applicationEventLoopRunner_Action struct {

	// logger to use
	log logr.Logger

	// The Name field of the K8s object (for example, a GitOpsDeploymentRun with a name of 'my-app-deployment')
	eventResourceName string

	// The namespace (for example, a GitOpsDeploymentRun in namespace 'my-deployments')
	eventResourceNamespace string

	// The K8s client that can be used to read/write objects on the workspace cluster
	workspaceClient client.Client

	// The UID of the workspace (namespace containing GitOps API types)
	workspaceID string

	// getK8sClientForGitOpsEngineInstance returns the K8s client that corresponds to the gitops engine instance.
	// As of this writing, only one Argo CD instance is supported, so this is trivial, but should have
	// more complex logic in the future.
	getK8sClientForGitOpsEngineInstance func(gitopsEngineInstance *db.GitopsEngineInstance) (client.Client, error)

	sharedResourceEventLoop *sharedResourceEventLoop

	// testOnlySkipCreateOperation: for unit testing purposes only, skip creation of an operation when
	// processing the action. This allows us to unit test application event functions without needing to
	// have a cluster-agent running alongside it.
	testOnlySkipCreateOperation bool
}

// cleanupOperation cleans up the database entry and (optionally) the CR, once an operation has concluded.
func cleanupOperation(ctx context.Context, dbOperation db.Operation, k8sOperation operation.Operation, operationNamespace string,
	dbQueries db.ApplicationScopedQueries, gitopsEngineClient client.Client, log logr.Logger) error {

	log = log.WithValues("operation", dbOperation.Operation_id, "namespace", operationNamespace)

	// // Delete the database entry
	// rowsDeleted, err := dbQueries.DeleteOperationById(ctx, dbOperation.Operation_id)
	// if err != nil {
	// 	return err
	// }
	// if rowsDeleted != 1 {
	// 	log.V(sharedutil.LogLevel_Warn).Error(err, "unexpected number of operation rows deleted", "operation-id", dbOperation.Operation_id, "rows", rowsDeleted)
	// }

	log.V(sharedutil.LogLevel_Debug).Info("Deleting operation CR: " + k8sOperation.Name)

	// Optional: Delete the Operation CR
	if err := gitopsEngineClient.Delete(ctx, &k8sOperation); err != nil {
		if !apierr.IsNotFound(err) {
			// Log the error, but don't return it: it's the responsibility of the cluster agent to delete the operation cr
			log.Error(err, "Unable to delete operation")
		}
	}

	return nil

}

func CreateOperation(ctx context.Context, waitForOperation bool, dbOperationParam db.Operation, clusterUserID string,
	operationNamespace string, dbQueries db.ApplicationScopedQueries, gitopsEngineClient client.Client, log logr.Logger) (*operation.Operation, *db.Operation, error) {

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

	log.Info("Creating database operation", "operation", dbOperation.ShortString())

	if err := dbQueries.CreateOperation(ctx, &dbOperation, clusterUserID); err != nil {
		log.Error(err, "unable to create operation", "operation", dbOperation.LongString())
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

	log.Info("Creating K8s Operation CR", "operation", fmt.Sprintf("%v", operation.Spec.OperationID))

	if err := gitopsEngineClient.Create(ctx, &operation, &client.CreateOptions{}); err != nil {
		log.Error(err, "unable to create K8s Operation in namespace", "operation", dbOperation.Operation_id, "namespace", operation.Namespace)
		return nil, nil, err
	}

	// Wait for operation to complete.

	if waitForOperation {
		log.Info("Waiting for Operation to complete", "operation", fmt.Sprintf("%v", operation.Spec.OperationID))

		if err = WaitForOperationToComplete(ctx, &dbOperation, dbQueries, log); err != nil {
			log.Error(err, "operation did not complete", "operation", dbOperation.Operation_id, "namespace", operation.Namespace)
			return nil, nil, err
		}

		log.Info("Operation completed", "operation", fmt.Sprintf("%v", operation.Spec.OperationID))
	}

	return &operation, &dbOperation, nil

}

func WaitForOperationToComplete(ctx context.Context, dbOperation *db.Operation, dbQueries db.ApplicationScopedQueries, log logr.Logger) error {

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
			return fmt.Errorf("operation context is Done()")
		default:
		}

	}

	return nil
}

func getWorkspaceIDFromNamespaceID(namespace corev1.Namespace) string {
	// Here we assume that the namespace UID is the same as the workspace UID. If/when that changes, this should be updated.
	return string(namespace.UID)
}

type argoCDSpecInput struct {
	// MAKE SURE YOU SANITIZE ANY NEW FIELDS THAT ARE ADDED!!!!
	crName      string
	crNamespace string
	// MAKE SURE YOU SANITIZE ANY NEW FIELDS THAT ARE ADDED!!!!
	destinationNamespace string
	destinationName      string
	// MAKE SURE YOU SANITIZE ANY NEW FIELDS THAT ARE ADDED!!!!
	sourceRepoURL        string
	sourcePath           string
	sourceTargetRevision string
	// MAKE SURE YOU SANITIZE ANY NEW FIELDS THAT ARE ADDED!!!!
	automated bool

	// Hopefully you are getting the message, here :)
}

func createSpecField(fieldsParam argoCDSpecInput) (string, error) {

	sanitize := func(input string) string {
		input = strings.ReplaceAll(input, "\"", "")
		input = strings.ReplaceAll(input, "'", "")
		input = strings.ReplaceAll(input, "`", "")
		input = strings.ReplaceAll(input, "\r", "")
		input = strings.ReplaceAll(input, "\n", "")
		input = strings.ReplaceAll(input, "&", "")
		input = strings.ReplaceAll(input, ";", "")
		input = strings.ReplaceAll(input, "%", "")

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
		automated:            fieldsParam.automated,
		// MAKE SURE YOU SANITIZE ANY NEW FIELDS THAT ARE ADDED!!!!

		// Hopefully you are getting the message, here :)
	}

	application := fauxargocd.FauxApplication{
		FauxTypeMeta: fauxargocd.FauxTypeMeta{
			Kind:       "Application",
			APIVersion: "argoproj.io/v1alpha1",
		},
		FauxObjectMeta: fauxargocd.FauxObjectMeta{
			Name:      fields.crName,
			Namespace: fields.crNamespace,
		},
		Spec: fauxargocd.FauxApplicationSpec{
			Source: fauxargocd.ApplicationSource{
				RepoURL:        fields.sourceRepoURL,
				Path:           fields.sourcePath,
				TargetRevision: fields.sourceTargetRevision,
			},
			Destination: fauxargocd.ApplicationDestination{
				Name:      fields.destinationName,
				Namespace: fields.destinationNamespace,
			},
			Project: "default",
		},
	}

	if fields.automated {
		application.Spec.SyncPolicy = &fauxargocd.SyncPolicy{
			Automated: &fauxargocd.SyncPolicyAutomated{
				Prune: false,
			},
		}
	}

	resBytes, err := goyaml.Marshal(application)

	if err != nil {
		return "", nil
	}

	return string(resBytes), nil
}

// func createSpecFieldOld(fieldsParam argoCDSpecInput) string {

// 	text := `apiVersion: argoproj.io/v1alpha1
// kind: Application
// metadata:
//   name: "` + fields.crName + `"
//   namespace: "` + fields.crNamespace + `"
// spec:
//   destination:
//     name: "` + fields.destinationName + `"
//     namespace: "` + fields.destinationNamespace + `"
//   project: default
//   source:
//     path: "` + fields.sourcePath + `"
//     repoURL: "` + fields.sourceRepoURL + `"
//     targetRevision: "` + fields.sourceTargetRevision + `"`

// 	if fields.automated {
// 		text += `
//   syncPolicy:
//     automated: {}`
// 	}

// 	return text
// }

// stringEventLoopEvent is a utility function for debug purposes.
func stringEventLoopEvent(obj *eventLoopEvent) string {
	if obj == nil {
		return "(nil)"
	}

	return fmt.Sprintf("[%s] %s/%s/%s, for workspace '%s', gitopsdepluid: '%s'", obj.eventType, obj.request.Namespace,
		obj.request.Name, string(obj.reqResource), obj.workspaceID, obj.associatedGitopsDeplUID)

}
