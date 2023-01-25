package application_event_loop

import (
	"context"
	"fmt"
	"time"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/gitopserrors"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ErrDeploymentNameIsImmutable = "deployment name field is immutable: changing it from its initial value is not supported"

	ErrRevisionIsImmutable = "revision change is not supported: changing it from its initial value is not supported"
)

// This file is responsible for processing events related to GitOpsDeploymentSyncRun CR.

func (action *applicationEventLoopRunner_Action) applicationEventRunner_handleSyncRunModified(ctx context.Context, dbQueries db.ApplicationScopedQueries) error {

	// Handle all GitOpsDeploymentSyncRun related events
	err := action.applicationEventRunner_handleSyncRunModifiedInternal(ctx, dbQueries)

	syncRunCR, clientErr := getGitOpsDeploymentSyncRun(ctx, action.workspaceClient, action.eventResourceName, action.eventResourceNamespace)
	if clientErr != nil {
		if !apierr.IsNotFound(clientErr) {
			return fmt.Errorf("unable to get GitOpsDeploymentSyncRun: %v", err)
		}
		return nil
	}

	conditionType := managedgitopsv1alpha1.GitOpsDeploymentSyncRunConditionErrorOccurred
	if err != nil {

		errMsg := err.UserError()
		if errMsg == "" {
			errMsg = gitopserrors.UnknownError
		}

		if err := setGitOpsDeploymentSyncRunCondition(ctx, action.workspaceClient, syncRunCR, conditionType, managedgitopsv1alpha1.SyncRunReasonType(conditionType), managedgitopsv1alpha1.GitOpsConditionStatusTrue, errMsg); err != nil {
			return fmt.Errorf("failed to update the status of GitOpsDeploymentSyncRun: %v", err)
		}

		return err.DevError()
	}

	if err := setGitOpsDeploymentSyncRunCondition(ctx, action.workspaceClient, syncRunCR, conditionType, managedgitopsv1alpha1.SyncRunReasonType(""), managedgitopsv1alpha1.GitOpsConditionStatusFalse, ""); err != nil {
		return fmt.Errorf("failed to update the status of GitOpsDeploymentSyncRun: %v", err)
	}

	return nil

}

func setGitOpsDeploymentSyncRunCondition(ctx context.Context, k8sClient client.Client, syncRunCR *managedgitopsv1alpha1.GitOpsDeploymentSyncRun, conditionType managedgitopsv1alpha1.SyncRunConditionType, reason managedgitopsv1alpha1.SyncRunReasonType, status managedgitopsv1alpha1.GitOpsConditionStatus, message string) error {

	conditions := syncRunCR.Status.Conditions
	conditionIndex := findConditionIndex(conditions, conditionType)

	now := metav1.Now()

	// create a new condition if it is absent
	if conditionIndex == -1 {
		syncRunCR.Status.Conditions = append(conditions, managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition{
			Type:               conditionType,
			LastTransitionTime: &now,
			Status:             status,
			Reason:             reason,
			Message:            message,
		})

	} else {
		// update the existing condition if it has changed
		condition := &conditions[conditionIndex]
		if message != condition.Message || status != condition.Status || reason != condition.Reason {
			condition.LastTransitionTime = &now
		}

		condition.Message = message
		condition.Status = status
		condition.Reason = reason
	}

	return k8sClient.Status().Update(ctx, syncRunCR)
}

func findConditionIndex(conditions []managedgitopsv1alpha1.GitOpsDeploymentSyncRunCondition, conditionType managedgitopsv1alpha1.SyncRunConditionType) int {

	for i, condition := range conditions {
		if condition.Type == conditionType {
			return i
		}
	}

	return -1
}

func getGitOpsDeploymentSyncRun(ctx context.Context, k8sClient client.Client, name, namespace string) (*managedgitopsv1alpha1.GitOpsDeploymentSyncRun, error) {
	syncRunCR := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(syncRunCR), syncRunCR)
	if err != nil {
		return nil, err
	}

	return syncRunCR, nil
}

func (a *applicationEventLoopRunner_Action) applicationEventRunner_handleSyncRunModifiedInternal(ctx context.Context,
	dbQueries db.ApplicationScopedQueries) gitopserrors.UserError {

	log := a.log

	namespace := corev1.Namespace{}
	if err := a.workspaceClient.Get(ctx, types.NamespacedName{Name: a.eventResourceNamespace}, &namespace); err != nil {
		userError := fmt.Sprintf("unable to retrieve the contents of the namespace '%s' containing the API resource '%s'. Does it exist?",
			a.eventResourceNamespace, a.eventResourceName)
		devError := fmt.Errorf("unable to retrieve namespace '%s': %v", a.eventResourceNamespace, err)
		return gitopserrors.NewUserDevError(userError, devError)
	}

	clusterUser, _, err := a.sharedResourceEventLoop.GetOrCreateClusterUserByNamespaceUID(ctx, a.workspaceClient, namespace, log)
	if err != nil {
		userError := "unable to locate managed environment for new application in syncrun modified"
		devError := fmt.Errorf("unable to retrieve cluster user in applicationEventRunner_handleSyncRunModifiedInternal, '%s': %v",
			string(namespace.UID), err)
		return gitopserrors.NewUserDevError(userError, devError)
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
				userError := "unable to retrieve the GitOpsDeploymentSyncRun object from the namespace, due to unknown error."
				log.Error(err, "unable to locate object in handleSyncRunModified", "request", syncRunKey)
				return gitopserrors.NewUserDevError(userError, err)
			}
		}
	}

	// Retrieve the SyncOperation row that corresponds to the SyncRun resource
	var apiCRToDBList []db.APICRToDatabaseMapping

	// dbEntryExists is true if there already exists a APICRToDBMapping for the GitOpsDeploymentSyncRun CR
	dbEntryExists := false

	if syncRunCRExists {

		if syncRunCR == (&managedgitopsv1alpha1.GitOpsDeploymentSyncRun{}) {
			err := fmt.Errorf("SEVERE - sync run cr is empty")
			log.Error(err, err.Error())
			return gitopserrors.NewDevOnlyError(err)
		}

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
				userError := "unable to retrieve GitOpsDeploymentSyncRun metadata from the internal database, due to an unknown error"
				log.Error(err, "unable to resource APICRToDatabaseMapping", "uid", string(syncRunCR.UID))
				return gitopserrors.NewUserDevError(userError, err)
			}
		} else {
			// Match found in database
			apiCRToDBList = append(apiCRToDBList, mapping)
			dbEntryExists = true
		}

	} else {

		// The CR no longer exists (it was likely deleted), so instead we retrieve the UID of the SyncRun from
		// the APICRToDatabaseMapping table, by combination of (name/namespace/namespace uid).

		if err := dbQueries.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx, db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
			a.eventResourceName, a.eventResourceNamespace, eventlooptypes.GetWorkspaceIDFromNamespaceID(namespace),
			db.APICRToDatabaseMapping_DBRelationType_SyncOperation, &apiCRToDBList); err != nil {
			userError := "unable to retrive data related to previous GitOpsDeploymentSyncRun in the namespace, due to an unknown error"
			log.Error(err, "unable to find API CR to DB Mapping, by API name/namespace/uid",
				"name", a.eventResourceName, "namespace", a.eventResourceNamespace, "UID", string(namespace.UID))
			return gitopserrors.NewUserDevError(userError, err)
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

	log.Info("workspaceEventLoopRunner_handleSyncRunModified", "syncRunCRExists", syncRunCRExists, "dbEntryExists", dbEntryExists)

	if !syncRunCRExists && !dbEntryExists {
		log.Info("neither sync run CR exists, nor db entry, so our work is done.")
		// if neither exists, our work is done
		return nil
	}

	// Get the SyncOperation table entry pointed to by the resource
	var syncOperation db.SyncOperation

	if dbEntryExists {

		if len(apiCRToDBList) != 1 {
			err := fmt.Errorf("SEVERE - Update only supports one operation parameter")
			log.Error(err, err.Error())
			return gitopserrors.NewDevOnlyError(err)
		}

		apiCRToDBMapping := apiCRToDBList[0]

		if apiCRToDBMapping.DBRelationType != db.APICRToDatabaseMapping_DBRelationType_SyncOperation {
			err := fmt.Errorf("SEVERE - db relation type should be syncoperation")
			log.Error(err, err.Error())
			return gitopserrors.NewDevOnlyError(err)
		}

		syncOperation = db.SyncOperation{SyncOperation_id: apiCRToDBMapping.DBRelationKey}

		if err := dbQueries.GetSyncOperationById(ctx, &syncOperation); err != nil {

			log.Error(err, "unable to retrieve sync operation by id on modified", "operationID", syncOperation.SyncOperation_id)
			return gitopserrors.NewDevOnlyError(err)
		}

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
				userError := fmt.Sprintf("Unable to retrieve GitOpsDeployment '%s' referenced by the GitOpsDeploymentSyncRun", gitopsDepl.Name)
				log.Error(err, "handleSyncRunModified error")
				return gitopserrors.NewUserDevError(userError, err)
			}

			// If there was a generic error in retrieving the key, return it
			log.Error(err, "unable to retrieve gitopsdeployment referenced in syncrun")
			return gitopserrors.NewDevOnlyError(fmt.Errorf("SEVERE - All cases should be handled by above if statements"))
		}

		// return an error if 'Automated' sync policy is enabled. Argo CD doesn't allow syncing an Application with automated sync policy.
		if gitopsDepl.Spec.Type != managedgitopsv1alpha1.GitOpsDeploymentSpecType_Manual {
			userErr := fmt.Sprintf("invalid GitOpsDeploymentSyncRun '%s'. Syncing a GitOpsDeployment with Automated sync policy is not allowed", syncRunCR.Name)
			devErr := fmt.Errorf(userErr)
			log.Error(devErr, "failed to process GitOpsDeploymentSyncRun")
			return gitopserrors.NewUserDevError(userErr, devErr)
		}

		// The GitopsDepl CR exists, so use the UID of the CR to retrieve the database entry, if possible
		deplToAppMapping := &db.DeploymentToApplicationMapping{Deploymenttoapplicationmapping_uid_id: string(gitopsDepl.UID)}

		if err = dbQueries.GetDeploymentToApplicationMappingByDeplId(ctx, deplToAppMapping); err != nil {
			log.Error(err, "unable to retrieve deployment to application mapping, on sync run modified", "uid", string(gitopsDepl.UID))
			return gitopserrors.NewDevOnlyError(err)
		}

		application = &db.Application{Application_id: deplToAppMapping.Application_id}
		if err := dbQueries.GetApplicationById(ctx, application); err != nil {
			log.Error(err, "unable to retrieve application, on sync run modified", "applicationId", string(deplToAppMapping.Application_id))
			return gitopserrors.NewDevOnlyError(err)
		}

		if gitopsEngineInstance, err = a.sharedResourceEventLoop.GetGitopsEngineInstanceById(ctx, application.Engine_instance_inst_id,
			a.workspaceClient, namespace, a.log); err != nil {

			log.Error(err, "unable to retrieve gitopsengineinstance, on sync run modified", "instanceId", string(application.Engine_instance_inst_id))
			return gitopserrors.NewDevOnlyError(err)
		}

		if dbEntryExists {
			// Handle update:
			// If both GitOpsDeploymentSyncRun CR and the DB entry exists, then the CR is being updated.
			// Validate and return an error if the immutable fields are updated.
			return a.handleUpdatedGitOpsDeplSyncRunEvent(ctx, syncRunCR, dbQueries, syncOperation)
		} else {
			// Handle create:
			// If the gitopsdeplsyncrun CR exists, but the database entry doesn't, then this is the first time we
			// have seen the GitOpsDeplSyncRun CR.
			// Create it in the DB and create the operation.

			return a.handleNewGitOpsDeplSyncRunEvent(ctx, syncRunCR, dbQueries, application, gitopsEngineInstance, namespace, *clusterUser)
		}

	}

	if !syncRunCRExists && dbEntryExists {
		// Handle delete:
		// If the gitopsdeplsyncrun CR doesn't exist, but database row does, then the CR has been deleted, so handle it.

		return a.handleDeletedGitOpsDeplSyncRunEvent(ctx, dbQueries, syncOperation, apiCRToDBList, namespace, clusterUser)
	}

	return nil

}

// handleDeletedGitOpsDeplSyncRunEvent handles GitOpsDeploymentSyncRun events where the user has just deleted a GitOpsDeploymentSyncRun resource.
// In this case, we need to update the state of the SyncOperation DB row to 'Terminated' and inform the cluster-agent component to cancel the sync operation.
//
// Finally, delete the SyncOperation and APICRToDBMapping rows in the database for this resource.
//
// Returns:
// - error is non-nil, if an error occurred
func (a *applicationEventLoopRunner_Action) handleDeletedGitOpsDeplSyncRunEvent(ctx context.Context, dbQueries db.ApplicationScopedQueries, syncOperation db.SyncOperation, apiCRToDBList []db.APICRToDatabaseMapping, namespace corev1.Namespace, clusterUser *db.ClusterUser) gitopserrors.UserError {

	// Deleting the CR should terminate a sync operation, if it was previously in progress.

	log := a.log
	log.Info("Received GitOpsDeploymentSyncRun event for a GitOpsDeploymentSyncRun resource that no longer exists")

	// 1) Update the state of the SyncOperation DB table to say that we want to terminate it, if it is runing
	syncOperation.DesiredState = db.SyncOperation_DesiredState_Terminated
	if err := dbQueries.UpdateSyncOperation(ctx, &syncOperation); err != nil {
		log.Error(err, "unable to update the sync operation as terminated", "syncOperationID", syncOperation.SyncOperation_id)
		return gitopserrors.NewDevOnlyError(err)
	}

	application := &db.Application{Application_id: syncOperation.Application_id}
	if err := dbQueries.GetApplicationById(ctx, application); err != nil {
		log.Error(err, "unable to retrieve application, on sync run modified", "applicationId", string(syncOperation.Application_id))
		return gitopserrors.NewDevOnlyError(err)
	}

	gitopsEngineInstance, err := a.sharedResourceEventLoop.GetGitopsEngineInstanceById(ctx, application.Engine_instance_inst_id,
		a.workspaceClient, namespace, a.log)
	if err != nil {

		log.Error(err, "unable to retrieve gitopsengineinstance, on sync run modified", "instanceId", string(application.Engine_instance_inst_id))
		return gitopserrors.NewDevOnlyError(err)
	}

	dbOperationInput := db.Operation{
		Instance_id:   gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:   syncOperation.SyncOperation_id,
		Resource_type: db.OperationResourceType_SyncOperation,
	}

	// 2) Create the operation, in order to inform the cluster agent it needs to cancel the sync operation
	operationClient, err := a.getK8sClientForGitOpsEngineInstance(ctx, gitopsEngineInstance)
	if err != nil {
		log.Error(err, "unable to retrieve gitopsengine instance from handleSyncRunModified, when resource was deleted")
		return gitopserrors.NewDevOnlyError(err)
	}

	waitForOperation := !a.testOnlySkipCreateOperation // if it's for a unit test, we don't wait for the operation
	k8sOperation, dbOperation, err := operations.CreateOperation(ctx, waitForOperation, dbOperationInput, clusterUser.Clusteruser_id,
		dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, operationClient, log)
	if err != nil {
		log.Error(err, "could not create operation, when resource was deleted", "namespace", dbutil.GetGitOpsEngineSingleInstanceNamespace())

		return gitopserrors.NewDevOnlyError(err)
	}

	// 3) Clean up the operation and database table entries
	if err := operations.CleanupOperation(ctx, *dbOperation, *k8sOperation, dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, operationClient, !a.testOnlySkipCreateOperation, log); err != nil {
		return gitopserrors.NewDevOnlyError(err)
	}

	var allErrors error

	// Remove the mappings and their associated operations and syncoperations.
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
		return gitopserrors.NewDevOnlyError(allErrors)
	}

	// Success: the CR no longer exists, and we have completed cleanup.
	return nil

}

// handleNewGitOpsDeplSyncRunEvent handles GitOpsDeploymentSyncRun events where the user has just created a new GitOpsDeploymentSyncRun resource.
// In this case, we need to create SyncOperation and APICRToDBMapping rows in the database.
//
// Finally, we need to inform the cluster-agent component (via Operation), so that it can sync the Argo CD Application.
//
// Returns:
// - error is non-nil, if an error occurred
func (a *applicationEventLoopRunner_Action) handleNewGitOpsDeplSyncRunEvent(ctx context.Context, syncRunCRParam *managedgitopsv1alpha1.GitOpsDeploymentSyncRun, dbQueries db.ApplicationScopedQueries, application *db.Application, gitopsEngineInstance *db.GitopsEngineInstance, namespace corev1.Namespace, clusterUser db.ClusterUser) gitopserrors.UserError {

	log := a.log
	log.Info("Received GitOpsDeploymentSyncRun event for a new GitOpsDeploymentSyncRun resource")

	if application == nil || gitopsEngineInstance == nil {
		err := fmt.Errorf("app or engine instance were nil in handleSyncRunModified app: %v, instance: %v", application, gitopsEngineInstance)
		log.Error(err, "unexpected nil value of required objects")
		return gitopserrors.NewDevOnlyError(err)
	}

	// createdResources is a list of database entries created in this function; if an error occurs, we delete them
	// in reverse order.
	var createdResources []db.AppScopedDisposableResource

	// Create sync operation
	syncOperation := &db.SyncOperation{
		Application_id:      application.Application_id,
		DeploymentNameField: syncRunCRParam.Spec.GitopsDeploymentName,
		Revision:            syncRunCRParam.Spec.RevisionID,
		DesiredState:        db.SyncOperation_DesiredState_Running,
	}
	if err := dbQueries.CreateSyncOperation(ctx, syncOperation); err != nil {
		log.Error(err, "unable to create sync operation in database")

		return gitopserrors.NewDevOnlyError(err)
	}
	createdResources = append(createdResources, syncOperation)
	log.Info("Created a Sync Operation: " + syncOperation.SyncOperation_id)

	newApiCRToDBMapping := db.APICRToDatabaseMapping{
		APIResourceType: db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
		APIResourceUID:  string(syncRunCRParam.UID),
		DBRelationType:  db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
		DBRelationKey:   syncOperation.SyncOperation_id,

		APIResourceName:      syncRunCRParam.Name,
		APIResourceNamespace: syncRunCRParam.Namespace,
		NamespaceUID:         eventlooptypes.GetWorkspaceIDFromNamespaceID(namespace),
	}
	if err := dbQueries.CreateAPICRToDatabaseMapping(ctx, &newApiCRToDBMapping); err != nil {
		log.Error(err, "unable to create api to db mapping in database")

		// If we were unable to retrieve the client, delete the resources we created in the previous steps
		dbutil.DisposeApplicationScopedResources(ctx, createdResources, dbQueries, log)

		return gitopserrors.NewDevOnlyError(err)
	}
	log.Info(fmt.Sprintf("Created a ApiCRToDBMapping: (APIResourceType: %s, APIResourceUID: %s, DBRelationType: %s)", newApiCRToDBMapping.APIResourceType, newApiCRToDBMapping.APIResourceUID, newApiCRToDBMapping.DBRelationType))
	createdResources = append(createdResources, &newApiCRToDBMapping)

	operationClient, err := a.getK8sClientForGitOpsEngineInstance(ctx, gitopsEngineInstance)
	if err != nil {
		log.Error(err, "unable to retrieve gitopsengine instance from handleSyncRunModified")

		// If we were unable to retrieve the client, delete the resources we created in the previous steps
		dbutil.DisposeApplicationScopedResources(ctx, createdResources, dbQueries, log)

		// Return the original error
		return gitopserrors.NewDevOnlyError(err)
	}

	dbOperationInput := db.Operation{
		Instance_id:   gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:   syncOperation.SyncOperation_id,
		Resource_type: db.OperationResourceType_SyncOperation,
	}

	k8sOperation, dbOperation, err := operations.CreateOperation(ctx, false, dbOperationInput, clusterUser.Clusteruser_id,
		dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, operationClient, log)
	if err != nil {
		log.Error(err, "could not create operation", "namespace", dbutil.GetGitOpsEngineSingleInstanceNamespace())

		// If we were unable to create the operation, delete the resources we created in the previous steps
		dbutil.DisposeApplicationScopedResources(ctx, createdResources, dbQueries, log)

		return gitopserrors.NewDevOnlyError(err)
	}

	backoff := sharedutil.ExponentialBackoff{Factor: 1.3, Min: time.Millisecond * 1000, Max: time.Second * 10, Jitter: true}

outer_for:

	for {

		if a.testOnlySkipCreateOperation {
			break outer_for
		}

		if isComplete, err := operations.IsOperationComplete(ctx, dbOperation, dbQueries); err != nil {
			log.Error(err, "an error occurred on retrieving operation status")

			break outer_for
		} else if isComplete {
			// Our work is done: the operation is complete
			break outer_for
		}

		currentSyncRunCR := syncRunCRParam.DeepCopy()
		if err := a.workspaceClient.Get(ctx, client.ObjectKeyFromObject(currentSyncRunCR), currentSyncRunCR); err != nil {

			if apierr.IsNotFound(err) {
				log.Info("the SyncRunCR that we were watching is no longer present, exiting the sync process.")
				break outer_for
			} else {
				log.Error(err, "an unexpected error occurred while attempting to retrieve SyncRun CR")
				// continue for unexpected errors: we expect them to be transient, not permanent
			}
		} else {
			// No error occurred: SyncRun CR still exists in the Namespace

			// Compare the current CR in the namespace, with the CR copy we started this function with
			if currentSyncRunCR.UID != syncRunCRParam.UID {
				// The SyncRun UID changed versus the one that we started working on at the beginning of this function.
				// - This means the original SyncRun CR was deleted, and recreated.
				// - We should thus exit the current sync operation, to allow the new CR to be processed by the runner.
				log.Info("The SyncRun CR UID has changed, versus the SyncRun CR that we began with, exiting the sync process")
				break outer_for
			}
		}

		backoff.DelayOnFail(ctx)

	}

	if err := operations.CleanupOperation(ctx, *dbOperation, *k8sOperation, dbutil.GetGitOpsEngineSingleInstanceNamespace(), dbQueries, operationClient, !a.testOnlySkipCreateOperation, log); err != nil {
		return gitopserrors.NewDevOnlyError(err)
	}

	return nil
}

// handleUpdatedGitOpsDeplSyncRunEvent handles GitOpsDeploymentSyncRun events where the user has just updated an existing GitOpsDeploymentSyncRun resource.
// In this case, we need to ensure that the immutable fields GitOpsDeploymentName and RevisionID are not updated.
//
// Returns:
// - error is non-nil, if an error occurred
func (a *applicationEventLoopRunner_Action) handleUpdatedGitOpsDeplSyncRunEvent(ctx context.Context, syncRunCR *managedgitopsv1alpha1.GitOpsDeploymentSyncRun, dbQueries db.ApplicationScopedQueries, syncOperation db.SyncOperation) gitopserrors.UserError {
	log := a.log
	log.Info("Received GitOpsDeploymentSyncRun event for an existing GitOpsDeploymentSyncRun resource")

	if syncOperation.DeploymentNameField != syncRunCR.Spec.GitopsDeploymentName {
		err := fmt.Errorf(ErrDeploymentNameIsImmutable)
		log.Error(err, ErrDeploymentNameIsImmutable)
		return gitopserrors.NewUserDevError(ErrDeploymentNameIsImmutable, err)
	}

	if syncOperation.Revision != syncRunCR.Spec.RevisionID {
		err := fmt.Errorf(ErrRevisionIsImmutable)
		log.Error(err, ErrRevisionIsImmutable)
		return gitopserrors.NewUserDevError(ErrRevisionIsImmutable, err)
	}

	return nil
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
	} else {
		log.Info("Sync Operation deleted with ID: " + apiCRToDB.DBRelationKey)
	}

	var operations []db.Operation
	if err := dbQueries.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, apiCRToDB.DBRelationKey, db.OperationResourceType_SyncOperation,
		&operations, clusterUser.Clusteruser_id); err != nil {

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
			} else {
				log.Info("Operation deleted with ID: " + operationId)
			}
		}
	}

	rowsDeleted, err = dbQueries.DeleteAPICRToDatabaseMapping(ctx, apiCRToDB)
	if err != nil {
		log.Error(err, "unable to delete apiCRToDBmapping", "mapping", apiCRToDB.APIResourceUID)
		return err

	} else if rowsDeleted == 0 {
		log.V(sharedutil.LogLevel_Warn).Error(err, "unexpected number of rows deleted of apiCRToDBmapping", "mapping", apiCRToDB.APIResourceUID)
	} else {
		log.Info("Deleted APICRToDatabaseMapping")
	}

	return nil
}
