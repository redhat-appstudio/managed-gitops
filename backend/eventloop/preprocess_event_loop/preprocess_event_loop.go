package preprocess_event_loop

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Preprocess Event Loop
//
// The pre-process event loop is responsible for:
// - receives all events all the API Resources controllers
// - pass the events to the next layer, which is controller_event_loop
// - detecting cases where the user deletes/creates a resource with the same name, before we have have processed the delete
// - ensures that 'associatedGitopsDeplUID' field is set for all requests that are processed by the GitOps service.
//     - if the event is for a GitOpsDeployment, then this field matches the UID of the resource (but for deleted resources, we need to retrieve the uid from the database)
//     - if the event is for a GitOpsDeploymentSyncRun, then this field matches the UID of the GitOp
//
// Invariants:
// - The cache should only ever use values from the database. It should be eventually consistent with the database.

// EventReceived is called by controllers to inform of it changes to API CRs
func (evl *PreprocessEventLoop) EventReceived(req ctrl.Request, reqResource eventlooptypes.GitOpsResourceType,
	client client.Client, eventType eventlooptypes.EventLoopEventType, namespaceID string) {

	event := eventlooptypes.EventLoopEvent{Request: req, EventType: eventType, WorkspaceID: namespaceID,
		Client: client, ReqResource: reqResource}

	evl.eventLoopInputChannel <- event
}

type PreprocessEventLoop struct {
	eventLoopInputChannel chan eventlooptypes.EventLoopEvent
	nextStep              *eventloop.ControllerEventLoop
}

func NewPreprocessEventLoop() *PreprocessEventLoop {
	channel := make(chan eventlooptypes.EventLoopEvent)

	res := &PreprocessEventLoop{}
	res.eventLoopInputChannel = channel
	res.nextStep = eventloop.NewControllerEventLoop()

	go preprocessEventLoopRouter(channel, res.nextStep)

	return res

}

const (
	// maxCacheSize is the maxmimum size of the LRU cache.
	maxCacheSize = 1000
)

func preprocessEventLoopRouter(input chan eventlooptypes.EventLoopEvent, nextStep *eventloop.ControllerEventLoop) {

	ctx := context.Background()
	log := log.FromContext(ctx).WithName("preprocess-event-loop")

	taskRetryLoop := sharedutil.NewTaskRetryLoop("event-loop-router-retry-loop")

	dbQueries, err := db.NewSharedProductionPostgresDBQueries(false)
	if err != nil {
		log.Error(err, "SEVERE: preProcessEventLoopRouter exiting before startup")
		os.Exit(1)
		return
	}

	lastUIDCache, err := newLastUIDCache(maxCacheSize)
	if err != nil {
		log.Error(err, "SEVERE: unexpected error on initializing cache")
		os.Exit(1)
		return
	}

	gitopsDeplSyncRunCache, err := newGitOpsDeplSyncRunCache(maxCacheSize)
	if err != nil {
		log.Error(err, "SEVERE: unexpected error on initializing cache")
		os.Exit(1)
		return
	}

	for {

		// Block on waiting for more events
		newEvent := <-input

		// Example: GitOpsDeployment-my_gops_depl-user_ns-(uid of namespace)
		mapKey := string(newEvent.ReqResource) + "-" + newEvent.Request.Name + "-" + newEvent.Request.Namespace + "-" + newEvent.WorkspaceID
		// TODO: GITOPSRVCE-68 - PERF - Use a more memory efficient key

		// Pass the event to the retry loop, for processing
		task := &processEventTask{
			newEvent:               newEvent,
			nextStep:               nextStep,
			dbQueries:              dbQueries,
			log:                    log,
			recentUIDCache:         lastUIDCache,
			gitopsDeplSyncRunCache: gitopsDeplSyncRunCache,
		}

		taskRetryLoop.AddTaskIfNotPresent(mapKey, task, sharedutil.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true})

	}
}

type processEventTask struct {
	// newEvent is an event received from one of the controllers (GitOpsDeployment/SyncRun/etc)
	newEvent eventlooptypes.EventLoopEvent

	// nextStep is a reference to the controller event loop, which is where the preprocess event loop will output events, after processing them
	nextStep *eventloop.ControllerEventLoop

	dbQueries db.DatabaseQueries
	log       logr.Logger

	// recentUIDCache is mapping from (name-namespace-namespaceuid) of resource to its K8s UID, the last time it was seen
	recentUIDCache *recentUIDCache

	// gitopsDeplSyncRunCache is a cache of which GitOpsDeploymentSyncRun K8s resources refer to which GitOpsDeployment K8s resources, by UID. See struct for details.
	gitopsDeplSyncRunCache *gitopsDeplSyncRunCache
}

// PerformTask returns true if the task should be retried (for example, because it failed), false otherwise.
func (task *processEventTask) PerformTask(taskContext context.Context) (bool, error) {

	return task.processEvent(taskContext, task.newEvent, task.nextStep, task.dbQueries, task.log), nil

}

// processEvent returns true if the task should be retried (for example, because it failed), false otherwise.
func (task *processEventTask) processEvent(ctx context.Context, newEvent eventlooptypes.EventLoopEvent,
	nextStep *eventloop.ControllerEventLoop, dbQueries db.DatabaseQueries, log logr.Logger) bool {

	log = log.WithValues("namespaceID", newEvent.WorkspaceID, "name", newEvent.Request.Name, "namespace", newEvent.Request.Namespace)

	log.V(sharedutil.LogLevel_Debug).Info("preprocess event loop router received event:", "event", eventlooptypes.StringEventLoopEvent(&newEvent))

	// Repository credentials are workspace-scoped, and thus do not need to go through this preprocess-event processing.
	if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentRepositoryCredentialTypeName {
		newEvent.AssociatedGitopsDeplUID = eventlooptypes.NoAssociatedGitOpsDeploymentUID
		emitEvent(newEvent, nextStep, "repo cred", log)
		return false
	}

	// Managed environments are passed to all gitopsdeployments of a workspace, and thus do not need to go through this preprocess-event processing.
	if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName {
		newEvent.AssociatedGitopsDeplUID = eventlooptypes.NoAssociatedGitOpsDeploymentUID
		emitEvent(newEvent, nextStep, "managed env", log)
		return false
	}

	// GitOpsDeployment, and GitOpsDeploymentSyncRun need to be processed separately from other resources, below, because
	// they are sharded via the 'newEvent.AssociatedGitopsDeplUID' value. Thus we need to determine the 'AssociatedGitopsDeplUID'
	// for every GitOpsDeployment/SyncRun event that we process, and store it in that field, before emitting the event.

	// Retrieve the event object
	resource, err := newEvent.GetReqResourceAsSimpleClientObject()
	if err != nil {
		log.Error(err, "Unable to convert req resource")
		return false
	}
	if err := newEvent.Client.Get(ctx, client.ObjectKeyFromObject(resource), resource); err != nil {

		if !apierr.IsNotFound(err) {
			log.Error(err, "unable to retrieve resource during preprocess loop", "resource", resource)
			return true
		}

		// If the resource we received the event for no longer exists, it has been deleted.
		// So we will need to use the cache and database values to figure out what resources to clean up.
		log.Info("CR doesn't exist in namespace, so the resource is likely deleted.", "resource", resource)
		return processResourceThatDoesntExistInNamespace(ctx, newEvent, nextStep, task.recentUIDCache, task.gitopsDeplSyncRunCache, dbQueries, log)
	}

	if resource.GetUID() == "" {
		log.Error(nil, "SEVERE: preprocess event loop resource had invalid UID")
		return false
	}
	// After the point in the code, we have successfully retrieved the event's resource (e.g. the GitOpsDeployment/SyncRun/Credential)
	// - the event we received is for a GitOpsDeployment/SyncRun that exists, and is in the namespace.

	// handleEventIfCached will handle the event if the resources for the event are in the cache. If not, it will return false, indicating that
	// it did not handle the event
	shouldRetry, isEventHandled := task.handleEventIfCached(ctx, newEvent, resource, nextStep, dbQueries, log)
	if isEventHandled {
		// Event has been handled (either with or without an error), so return and inform the caller whether they should retry the task
		return shouldRetry
	}

	// If we couldn't handle the event from the cache, then use the database.
	return task.handleEventFromDatabase(ctx, newEvent, resource, nextStep, dbQueries, log)

}

// handleEventFromDatabase processes 'newEvent' using the database.
// It returns true if the task should be retried, false otherwise.
func (task *processEventTask) handleEventFromDatabase(ctx context.Context, newEvent eventlooptypes.EventLoopEvent, resource client.Object,
	nextStep *eventloop.ControllerEventLoop, dbQueries db.DatabaseQueries, log logr.Logger) bool {

	if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {

		// Retrieve all of the SyncRun resources that were previously associated with this name/namespace/namespace uid
		associatedResources, err := getAssociatedResourcesForSyncRunAPINamespaceAndNameFromDB(ctx, newEvent, dbQueries, log)
		if err != nil {
			log.Error(err, "unable to retrieve associated syncrun resources")
			return true
		}

		// For each of the SyncRun resources tracked in the database...
		var currentUID syncRunAssociatedResource
		for _, associatedResource := range associatedResources {

			if associatedResource.resourceAPIUID != string(resource.GetUID()) &&
				associatedResource.deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id != "" {

				// For any old resources that don't match the current resource, emit them as old events
				var oldEvent eventlooptypes.EventLoopEvent = newEvent
				oldEvent.AssociatedGitopsDeplUID = associatedResource.deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id
				emitEvent(oldEvent, nextStep, "old sync run from db", log)
			}

			if associatedResource.resourceAPIUID == string(resource.GetUID()) {
				// Keep track if there is a database entry for the current resource

				if currentUID.resourceAPIUID != "" {
					// Sanity check that we are not setting currentUID multiple times (that it is empty before we set it)
					log.Error(nil, "SEVERE: there were multiple associated resources with the UID of the event resource")
				}

				currentUID = associatedResource
			}
		}

		if currentUID.deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id != "" {

			// Update the local cache
			task.gitopsDeplSyncRunCache.putAssociatedGitOpsDeploymentForSyncRun(string(resource.GetUID()),
				currentUID.deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id)

			// If we found the associated gitopsdeployment, set it
			newEvent.AssociatedGitopsDeplUID = currentUID.deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id
		} else {
			// Otherwise, it's orphaned
			newEvent.AssociatedGitopsDeplUID = eventlooptypes.OrphanedResourceGitopsDeplUID
		}

		emitEvent(newEvent, nextStep, "new sync run from db", log)

		return false

	} else if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentTypeName {

		var items []db.DeploymentToApplicationMapping

		if err := dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(ctx, newEvent.Request.Name, newEvent.Request.Namespace, newEvent.WorkspaceID, &items); err != nil {
			log.Error(err, "unable to list depltoappmapping in getGitOpsDeplFromDatabaseUsingNameAndNamespace")
			return true
		}

		for _, item := range items {

			// For any old resources that don't match the current resource, emit them as old events
			if item.Deploymenttoapplicationmapping_uid_id != string(resource.GetUID()) {

				var oldEvent eventlooptypes.EventLoopEvent = newEvent
				oldEvent.AssociatedGitopsDeplUID = item.Deploymenttoapplicationmapping_uid_id
				emitEvent(oldEvent, nextStep, "old gitopsdepl", log)
			}
		}

		// Finally, emit the new event
		newEvent.AssociatedGitopsDeplUID = string(resource.GetUID())
		emitEvent(newEvent, nextStep, "new gitopsdepl", log)

		return false

	} else {
		log.Error(nil, "SEVERE: unexpected resource type")
		return false
	}

}

// handleEventIfCached will handle the event if the resources for the event are in the cache. If not, it will return false, indicating that
// it did not handle the event.
// returns bools: (shouldRetry: whether or not the task should be called again, because it failed), (whether this function processed the event)
func (task *processEventTask) handleEventIfCached(ctx context.Context, newEvent eventlooptypes.EventLoopEvent, resource client.Object,
	nextStep *eventloop.ControllerEventLoop, dbQueries db.DatabaseQueries, log logr.Logger) (bool, bool) {

	// Check the cache to find what the previously associated GitOpsDeployment for this resource was.
	previousResourceUIDFromCache, previousGitopsDeplUIDFromCache, err := retrieveCacheValuesForResource(ctx, newEvent, task.recentUIDCache, task.gitopsDeplSyncRunCache, dbQueries, log)
	if err != nil {
		log.Error(err, "unexpected error when checking database contents with local map")
		return true, true
	}

	// Update the cache with the latest UID for this resource
	// (We intentionally need to do this after we call retrieveCacheValuesForResource)
	task.recentUIDCache.putMostRecentUIDForResourceFromEvent(string(resource.GetUID()), newEvent)

	// If the resource value wasn't found in the cache, or the previous gitopsdeployment resource wasn't found in the cache, then just return
	// and indicate we did not process the event.
	if previousGitopsDeplUIDFromCache == "" || previousResourceUIDFromCache == "" {
		return false, false
	}

	// if previousGitopsDeplUIDFromCache != "" && previousResourceUIDFromCache != "" {
	if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentTypeName {
		if previousResourceUIDFromCache != string(resource.GetUID()) {
			// emit for the old resource

			var oldEvent eventlooptypes.EventLoopEvent = newEvent
			oldEvent.AssociatedGitopsDeplUID = previousGitopsDeplUIDFromCache
			emitEvent(oldEvent, nextStep, "old event", log)

		}
		newEvent.AssociatedGitopsDeplUID = string(resource.GetUID())
		emitEvent(newEvent, nextStep, "new event", log)
		return false, true

	} else if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {
		if previousResourceUIDFromCache != string(resource.GetUID()) {
			// emit for both new and existing, using previousGitOpsDeplUID for existing

			// The last time we processed a GitOpsDeploymentSyncRun with this name/namespace/namespace uid, the
			// GitOpsDeploymentSyncRun had a different UID (indicating it was a different object), which
			// was likely deleted.
			//
			// So, first emit an event for the previous GitOpsDeploymentSyncRun object.
			var oldEvent eventlooptypes.EventLoopEvent = newEvent
			oldEvent.AssociatedGitopsDeplUID = previousGitopsDeplUIDFromCache
			emitEvent(oldEvent, nextStep, "old event", log)

			// Next, we attempt to determine the current GitOpsDeployment that should be associated with the
			// GitOpsDeploymentSyncRun (in newEvent) that we are current processing.
			// - We do this because previousResourceUIDFromCache does not apply, because it still refers to the old UID.
			associatedGitOpsDepl, err := getGitOpsDeplIdFromSyncRunCR(ctx, string(resource.GetUID()), task.gitopsDeplSyncRunCache, dbQueries, log)
			if err != nil {
				log.Error(err, "unable to retrieve new gitopsdepl for gitopsdeplsyncrun")
				return true, true
			}

			if associatedGitOpsDepl != "" {
				newEvent.AssociatedGitopsDeplUID = associatedGitOpsDepl
				emitEvent(newEvent, nextStep, "new event", log)

			} else {
				newEvent.AssociatedGitopsDeplUID = eventlooptypes.OrphanedResourceGitopsDeplUID
				emitEvent(newEvent, nextStep, "new orphaned event", log)
			}
			return false, true

		} else {
			// emit for new
			newEvent.AssociatedGitopsDeplUID = previousGitopsDeplUIDFromCache
			emitEvent(newEvent, nextStep, "existing syncrun from cache", log)
			return false, true
		}
	} else {
		log.Error(nil, "SEVERE: unexpected resource type", "resource", resource)
		return false, true
	}
}

// emitEvent passes the given event to the controller event loop
func emitEvent(event eventlooptypes.EventLoopEvent, nextStep *eventloop.ControllerEventLoop, debugStr string, log logr.Logger) {

	if nextStep == nil {
		log.Error(nil, "SEVERE: controllerEventLoop pointer should never be nil")
		return
	}

	log.V(sharedutil.LogLevel_Debug).Info("Emitting event to workspace event loop",
		"event", eventlooptypes.StringEventLoopEvent(&event), "debug-context", debugStr)

	nextStep.EventLoopInputChannel <- event

}

// This function processes the case where the event loop received an event for an API resource that doesn't in the K8s namespace.
// returns true if the task should be retried (for example, because it failed), false otherwise.
func processResourceThatDoesntExistInNamespace(ctx context.Context, newEvent eventlooptypes.EventLoopEvent, nextStep *eventloop.ControllerEventLoop,
	lastUIDCache *recentUIDCache, gitopsDeplSyncRunCache *gitopsDeplSyncRunCache, dbQueries db.DatabaseQueries, log logr.Logger) bool {

	// Check the local cache, to see if we have seen this resource before
	// - only check resources which we cache: GitOpsDeployment and GitOpsDeplomentSyncRun
	if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentTypeName ||
		newEvent.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {

		recentUIDForResource, gitopsDeplUID, err := retrieveCacheValuesForResource(ctx, newEvent, lastUIDCache, gitopsDeplSyncRunCache, dbQueries, log)
		if err != nil {
			// If a generic error occurred (database or client connection issue), then log the error
			// and return true, so that we can retry.
			log.Error(err, "unable to retrieve resource from local cache", "resource", newEvent.ReqResource)
			return true
		}

		// Clear the cache after retrieving it, because the resource has necessarily been deleted from the namespace.
		lastUIDCache.deleteMostRecentUIDForResourceFromEvent(newEvent)

		// If the we have the UID for the deleted resource, delete it
		if recentUIDForResource != "" {
			gitopsDeplSyncRunCache.deletedAssociatedGitOpsDeploymentForSyncRun(recentUIDForResource)
		}

		// If the GitOpsDeployment CR UID was found in the cache, then tag the event and emit it.
		if gitopsDeplUID != "" {
			newEvent.AssociatedGitopsDeplUID = gitopsDeplUID
			// Emit delete with uid found in local cache
			emitEvent(newEvent, nextStep, "found in local cache", log)
			return false
		}

		// Otherwise, not found in the cache, so exit the 'if' block, and next check the db...

	} else if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentRepositoryCredentialTypeName {

		lastUIDCache.deleteMostRecentUIDForResourceFromEvent(newEvent)

		// GitOpsDeploymentRepositoryCredentials doesn't have an associated GitOpsDeployment, so we use
		// the 'noAssociatedGitOpsDeploymentUID' and emit the delete event to the next layer.
		newEvent.AssociatedGitopsDeplUID = eventlooptypes.NoAssociatedGitOpsDeploymentUID
		emitEvent(newEvent, nextStep, "repo creds deleted", log)
		return false

	} else {
		// This shouldn't happen.
		log.Error(nil, "SEVERE: unexpected resource type: ", "resource", newEvent.ReqResource)
		return false
	}

	// Next: If the item wasn't found in the local cache above, check the database.

	if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {

		associatedResources, err := getAssociatedResourcesForSyncRunAPINamespaceAndNameFromDB(ctx, newEvent, dbQueries, log)
		if err != nil {
			log.Error(err, "unable to retrieve associated resourced ")
			return true
		}

		// For each of the SyncRun resources tracked in the database...
		for _, associatedResource := range associatedResources {

			if associatedResource.resourceAPIUID != "" {
				// Ensure it is removed from the sync run cache
				gitopsDeplSyncRunCache.deletedAssociatedGitOpsDeploymentForSyncRun(associatedResource.resourceAPIUID)
			}

			// For any old resources that don't match the current resource, emit them as old events
			if associatedResource.deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id != "" {
				var oldEvent eventlooptypes.EventLoopEvent = newEvent
				oldEvent.AssociatedGitopsDeplUID = associatedResource.deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id
				emitEvent(oldEvent, nextStep, "old sync run from db, on deleted resource", log)
			}
		}

		return false

	} else if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentTypeName {

		// Look for a corresponding DeploymentoApplicationMapping for the GitOpsDeployment CR in the namespace
		var items []db.DeploymentToApplicationMapping
		if err := dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(ctx, newEvent.Request.Name, newEvent.Request.Namespace,
			newEvent.WorkspaceID, &items); err != nil {
			log.Error(err, "unable to retrieve gitopsdeployment by namespacename in processEvent")
			return true
		}

		for _, item := range items {

			var event eventlooptypes.EventLoopEvent = newEvent
			event.AssociatedGitopsDeplUID = item.Deploymenttoapplicationmapping_uid_id

			// Emit delete, with UID from database
			emitEvent(event, nextStep, "found with uid from database", log)
		}

		// In this block:
		// - the CR doesn't exist
		// - the local cache hasn't previously seen a CR with this name/namespace/resource type
		// - we informed the next layer of any existing DeploymentToApplicationMappings that reference thie name/namespace
		// So we are done.

		return false

	} else {
		log.Error(nil, "SEVERE: no logic for processing req resource"+string(newEvent.ReqResource))
		return false
	}

}

// getGitOpsDeplIdFromDatabaseUsingNameAndNamespace returns gitopsdepl cr uid if found, "" if not found, or error if a generic error occurred.
// An error should only be returned if there is a reasonable expectation of success if this function were to be retried.
// (for example, network issues tend to be ephemeral, so we can return an error for suspected network issues)

// TODO: GITOPSRVCE-166: I believe this function is no longer needed; remove it once the rest of the syncoperation code in this file is fully covered.
//
// func getGitOpsDeplIdFromDatabaseUsingNameAndNamespace(ctx context.Context, newEvent eventlooptypes.EventLoopEvent,
// 	dbQueries db.DatabaseQueries, log logr.Logger) (string, error) {

// 	log = log.WithValues("name", newEvent.Request.Name, "namespace", newEvent.Request.Namespace)

// 	if newEvent.ReqResource == managedgitopsv1alpha1.GitOpsDeploymentTypeName {

// 		var items []db.DeploymentToApplicationMapping

// 		if err := dbQueries.ListDeploymentToApplicationMappingByNamespaceAndName(ctx, newEvent.Request.Name, newEvent.Request.Namespace, newEvent.WorkspaceID, &items); err != nil {
// 			log.Error(err, "unable to list depltoappmapping in getGitOpsDeplFromDatabaseUsingNameAndNamespace")
// 			return "", err
// 		}

// 		if len(items) > 1 {
// 			log.Error(nil, "SEVERE: unexpected number of application mappings when resolving depltoappmapping for gitopsdepl")
// 			return "", nil
// 		} else if len(items) == 1 {
// 			return items[0].Deploymenttoapplicationmapping_uid_id, nil
// 		} else {
// 			return "", nil
// 		}

// 	} else if newEvent.ReqResource == managedgitopsv1alpha1.GitOpsDeploymentSyncRunTypeName {

// 		var items []db.APICRToDatabaseMapping

// 		// 1) Go from GitOpsDeploymentSyncRun CR -> Sync Operation DB table, by searching
// 		// by the SyncRun CR's namespace/name/namespace ID tuple.
// 		if err := dbQueries.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx,
// 			db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
// 			newEvent.Request.Name, newEvent.Request.Namespace, newEvent.WorkspaceID,
// 			db.APICRToDatabaseMapping_DBRelationType_SyncOperation, &items); err != nil {
// 			log.Error(err, "unable to list api cr to db mapping in getGitOpsDeplFromDatabaseUsingNameAndNamespace")
// 			return "", err
// 		}

// 		if len(items) > 1 {
// 			log.Error(nil, "SEVERE: unexpected number of api cr to database mappings in getGitopsDeplUdFromDatabaseUsingNameAndNamespace")
// 			return "", nil

// 		} else if len(items) == 1 {

// 			// 2) We have found the pointer from GitOpsDeploymentSyncRun name/namespace to SyncOperation,
// 			// so next attempt to retrieve the SyncOperation
// 			syncOperation := &db.SyncOperation{
// 				SyncOperation_id: items[0].DBRelationKey,
// 			}
// 			if err := dbQueries.GetSyncOperationById(ctx, syncOperation); err != nil {

// 				if db.IsResultNotFoundError(err) {
// 					log.V(sharedutil.LogLevel_Warn).Info("syncoperation id from apicr to db mapping didn't exist", "operationID", syncOperation.SyncOperation_id)
// 					return "", nil
// 				} else {
// 					log.Error(err, "unable to retrieve sync operation by id: "+syncOperation.SyncOperation_id)
// 					return "", err
// 				}

// 			}

// 			// It's possible the Application (that this sync operation is targetting) is already deleted, if so just return.
// 			if syncOperation.Application_id == "" {
// 				log.Info("syncOperation '" + syncOperation.SyncOperation_id + "' was found in getGitOpsDeplFromDatabaseUsingNameAndNamespace, but application field was nil (likely it was deleted.)")
// 				return "", nil
// 			}

// 			// 3) We have the application id, so go from Application ID DB table -> GitOpsDeployment CR
// 			dtam := db.DeploymentToApplicationMapping{
// 				Application_id: syncOperation.Application_id,
// 			}
// 			if err := dbQueries.GetDeploymentToApplicationMappingByApplicationId(ctx, &dtam); err != nil {

// 				if db.IsResultNotFoundError(err) {
// 					log.V(sharedutil.LogLevel_Warn).Info("dtam not found when using application id from sync operation", "application_id", dtam.Application_id)
// 					return "", nil
// 				}

// 				log.Error(err, "unable to retrieve depltoappmapping by id: "+dtam.Application_id)
// 				return "", err
// 			}

// 			// Success, return the GitOpsDeployment UID from the entry
// 			return dtam.Deploymenttoapplicationmapping_uid_id, nil
// 		} else {
// 			// otherwise, not found.
// 			return "", nil
// 		}

// 	} else {
// 		return "", fmt.Errorf("SEVERE - unexpected request resource type")
// 	}
// }

// syncRunAssociatedResource is returned by the 'getAssociatedResourcesForSyncRunAPINamespaceAndName' function. It contains a set of resources
// that were previously associated with a SyncRun with the given name/namespace/namespace uid
type syncRunAssociatedResource struct {

	// resourceAPIUID is the UID of the K8s resource, if we found one that previously had the name/namespace/namespace UID of the event.
	resourceAPIUID string

	// syncOperation is the SyncOperation that the GitOpsDeploymentSyncRun referred to
	syncOperation db.SyncOperation

	// deploymentToApplicationMapping is the DeploymentToApplicationMapping resource of the Application that SyncOperation was targeting.
	deploymentToApplicationMapping db.DeploymentToApplicationMapping
}

// getAssociatedResourcesForSyncRunAPINamespaceAndNameFromDB retrieves all of the SyncRun resources that were previously associated
// with this name/namespace/namespace uid.
func getAssociatedResourcesForSyncRunAPINamespaceAndNameFromDB(ctx context.Context, newEvent eventlooptypes.EventLoopEvent, dbQueries db.DatabaseQueries, log logr.Logger) ([]syncRunAssociatedResource, error) {

	var items []db.APICRToDatabaseMapping

	// 1) Go from GitOpsDeploymentSyncRun CR -> Sync Operation DB table, by searching
	// by the SyncRun CR's namespace/name/namespace ID tuple.
	if err := dbQueries.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx,
		db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
		newEvent.Request.Name, newEvent.Request.Namespace, newEvent.WorkspaceID,
		db.APICRToDatabaseMapping_DBRelationType_SyncOperation, &items); err != nil {
		log.Error(err, "unable to list api cr to db mapping in getGitOpsDeplFromDatabaseUsingNameAndNamespace")
		return nil, err
	}

	res := []syncRunAssociatedResource{}

	for index := range items {

		item := items[index]

		syncRunAssociatedResource := syncRunAssociatedResource{
			resourceAPIUID: item.APIResourceUID,
		}

		// 2) We have found the pointer from GitOpsDeploymentSyncRun name/namespace to SyncOperation,
		// so next attempt to retrieve the SyncOperation
		syncOperation := &db.SyncOperation{
			SyncOperation_id: items[0].DBRelationKey,
		}
		if err := dbQueries.GetSyncOperationById(ctx, syncOperation); err != nil {

			if db.IsResultNotFoundError(err) {
				log.V(sharedutil.LogLevel_Warn).Info("syncoperation id from apicr to db mapping didn't exist", "operationID", syncOperation.SyncOperation_id)
			} else {
				log.Error(err, "unable to retrieve sync operation by id: "+syncOperation.SyncOperation_id)
				return nil, err
			}

			// SyncOperation was not found, so continue without setting the value in syncRunAssociatedResource
		} else {
			// SyncOperation was found
			syncRunAssociatedResource.syncOperation = *syncOperation
		}

		// It's possible the Application (that this sync operation is targetting) is already deleted, if so just return.
		if syncOperation.Application_id != "" {

			// 3) We have the application id, so go from Application ID DB table -> GitOpsDeployment CR
			dtam := db.DeploymentToApplicationMapping{
				Application_id: syncOperation.Application_id,
			}
			if err := dbQueries.GetDeploymentToApplicationMappingByApplicationId(ctx, &dtam); err != nil {

				if db.IsResultNotFoundError(err) {
					// Warn that we could retrieve the DTAM, but continue.
					log.V(sharedutil.LogLevel_Warn).Info("dtam not found when using application id from sync operation", "application_id", dtam.Application_id)
				} else {
					log.Error(err, "unable to retrieve depltoappmapping by id: "+dtam.Application_id)
					return nil, err
				}

			} else {
				// The DTAM was found, so set it in the syncRunAssociatedResource
				syncRunAssociatedResource.deploymentToApplicationMapping = dtam
			}

		} else {
			// The SyncOperation row did not contain an Application_id reference that we could follow, so just continue
			log.Info("syncOperation '" + syncOperation.SyncOperation_id + "' was found in getGitOpsDeplFromDatabaseUsingNameAndNamespace, but application field was nil (likely it was deleted.)")
		}

		res = append(res, syncRunAssociatedResource)

	}

	return res, nil
}

// retrieveCacheValuesForResource checks the cache to see if we previously saw the resource, and if we did, what it's associated GitOpsDeployment was.
// returns: (uid of resource last time it was seen, uid of gitopsdeployment when it was last seen, error)
func retrieveCacheValuesForResource(ctx context.Context,
	newEvent eventlooptypes.EventLoopEvent,
	recentUIDCacheParam *recentUIDCache,
	gitopsDeplSyncRunCache *gitopsDeplSyncRunCache,
	dbQueries db.DatabaseQueries, log logr.Logger) (string, string, error) {

	recentUIDForResource := recentUIDCacheParam.getMostRecentUIDForResourceFromEvent(newEvent)
	if recentUIDForResource == "" {
		// Not in the cache, so return
		return "", "", nil
	}

	if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentSyncRunTypeName {
		// If the resource is a GitOpsDeploymentSyncRun, we need to do an additional lookup up
		// to determine what the corresponding GitOpsDeployment is
		gitopsDeplUID, err := getGitOpsDeplIdFromSyncRunCR(ctx, recentUIDForResource, gitopsDeplSyncRunCache, dbQueries, log)
		if err != nil {
			return "", "", err
		}

		return recentUIDForResource, gitopsDeplUID, nil

	} else if newEvent.ReqResource == eventlooptypes.GitOpsDeploymentTypeName {
		// If the resource is a GitOpsDeployment, then the value is the UID of the resource, so we are done.
		return recentUIDForResource, recentUIDForResource, nil

	} else {
		return "", "", fmt.Errorf("unsupported resource type in getGitopsDeplIdFromMap: %v", newEvent.ReqResource)
	}

}

// getGitOpsDeplIdFromSyncRunCR attempts to locate the GitOpsDeployment resource that is associated with a
// particular GitOpsDeploymentSyncRun resource.
// - In order to determine that, we look if there is a SyncOperation associated with the GitOpsDeploymentSyncRun.
// - When then use that to look up the Application row, and then the GitOpsDeployment UID
//
// returns gitopsdepl cr uid if found, "" if not found, or error if a generic error occurred
func getGitOpsDeplIdFromSyncRunCR(ctx context.Context, gitopsSyncRunUID string, gitopsDeplSyncRunCache *gitopsDeplSyncRunCache,
	dbQueries db.DatabaseQueries, log logr.Logger) (string, error) {

	valueFromCache := gitopsDeplSyncRunCache.getAssociatedGitOpsDeploymentForSyncRun(gitopsSyncRunUID)
	if valueFromCache != "" {
		return valueFromCache, nil
	}

	// 1) Retrieve the APICRToDatabaseMapping, to allow us to go from GitOpsDeploymentSync CR -> SyncOperation in DB
	cr := db.APICRToDatabaseMapping{
		APIResourceType: db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
		APIResourceUID:  gitopsSyncRunUID,
		DBRelationType:  db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
	}
	if err := dbQueries.GetDatabaseMappingForAPICR(ctx, &cr); err != nil {

		if db.IsResultNotFoundError(err) {
			return "", nil
		}

		return "", fmt.Errorf("unable to retrieve db mapping for sync run CR: %v", err)
	}

	// 2) We now have the primary key for a SyncOperation row, so retrieve it.
	syncOperation := &db.SyncOperation{
		SyncOperation_id: cr.DBRelationKey,
	}
	if err := dbQueries.GetSyncOperationById(ctx, syncOperation); err != nil {

		if db.IsResultNotFoundError(err) {
			return "", nil
		}

		return "", fmt.Errorf("unable to retrieve sync operation db entry '%v' for sync run CR: %v", syncOperation.SyncOperation_id, err)
	}

	// It possible the Application (that this sync operation is targetting) is already deleted, if so just return.
	if syncOperation.Application_id == "" {
		log.Info("syncOperation '" + syncOperation.SyncOperation_id + "'was found in processEvent, but application field was nil (likely it was deleted.)")
		return "", nil
	}

	// 3) We have the application_id for the Application row, so go from Application ID DB table -> GitOpsDeployment CR
	dtam := db.DeploymentToApplicationMapping{
		Application_id: syncOperation.Application_id,
	}
	if err := dbQueries.GetDeploymentToApplicationMappingByApplicationId(ctx, &dtam); err != nil {

		if db.IsResultNotFoundError(err) {
			return "", nil
		}

		return "", fmt.Errorf("unable to retrieve depltoappmapping '%v' for sync run CR: %v", dtam.Application_id, err)
	}

	gitOpsDeploymentUID := dtam.Deploymenttoapplicationmapping_uid_id

	// Update the cache with the syncrun uid -> gitopsdeployment uid relationship
	gitopsDeplSyncRunCache.putAssociatedGitOpsDeploymentForSyncRun(gitopsSyncRunUID, gitOpsDeploymentUID)

	return gitOpsDeploymentUID, nil

}
