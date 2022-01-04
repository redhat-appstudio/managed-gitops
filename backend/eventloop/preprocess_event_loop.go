package eventloop

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/util"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Preprocess Event Loop
//
//
// Invariants:
// - The cache should only ever use values from the database. It should be eventually consistent with the database.

// EventReceived is called by controllers to inform of it changes to API CRs
func (evl *PreprocessEventLoop) EventReceived(req ctrl.Request, reqResource managedgitopsv1alpha1.GitOpsResourceType, client client.Client, eventType EventLoopEventType, workspaceID string) {

	event := eventLoopEvent{request: req, eventType: eventType, workspaceID: workspaceID, client: client, reqResource: reqResource}
	evl.eventLoopInputChannel <- event
}

type PreprocessEventLoop struct {
	eventLoopInputChannel chan eventLoopEvent
	nextStep              *controllerEventLoop
}

func NewPreprocessEventLoop() *PreprocessEventLoop {
	channel := make(chan eventLoopEvent)

	res := &PreprocessEventLoop{}
	res.eventLoopInputChannel = channel
	res.nextStep = newControllerEventLoop()

	go preprocessEventLoopRouter(channel, res.nextStep)

	return res

}

func preprocessEventLoopRouter(input chan eventLoopEvent, nextStep *controllerEventLoop /*, workspaceID string*/) {

	ctx := context.Background()

	log := log.FromContext(ctx)

	taskRetryLoop := newTaskRetryLoop()

	var resourcesSeenMutex sync.RWMutex

	// (cache key) -> (uid of the sync/syncrun resource, the last time it was seen)
	resourcesSeen := map[string]string{}
	// TODO: PERF - Add a size limit to this: evict LRU if over a certain size, to keep from hitting memory limit.

	dbQueries, err := db.NewProductionPostgresDBQueries(false)
	if err != nil {
		fmt.Println(err)
		return
		// TODO: DEBT - shouldn't return here
	}

	for {

		// Block on waiting for more events
		newEvent := <-input
		mapKey := string(newEvent.reqResource) + "-" + newEvent.request.Name + "-" + newEvent.request.Namespace + "-" + newEvent.workspaceID
		// TODO: PERF - Use a more memory efficient key

		// Pass the event to the retry loop, for processing
		task := &processEventTask{
			newEvent:           newEvent,
			mapKey:             mapKey,
			nextStep:           nextStep,
			dbQueries:          dbQueries,
			log:                log,
			resourcesSeen:      resourcesSeen,
			resourcesSeenMutex: &resourcesSeenMutex,
		}

		taskRetryLoop.addTaskIfNotPresent(mapKey, task, util.ExponentialBackoff{Factor: 2, Min: time.Millisecond * 200, Max: time.Second * 10, Jitter: true})

	}
}

type processEventTask struct {
	newEvent  eventLoopEvent
	mapKey    string
	nextStep  *controllerEventLoop
	dbQueries db.DatabaseQueries
	log       logr.Logger

	// resourcesSeen should only be accessed while holding resourcesSeenMutex
	resourcesSeen      map[string]string
	resourcesSeenMutex *sync.RWMutex
}

func (task *processEventTask) performTask(taskContext context.Context) (bool, error) {

	return task.processEvent(taskContext, task.newEvent, task.mapKey, task.nextStep, task.dbQueries, task.log), nil

}

func (task *processEventTask) processEvent(ctx context.Context, newEvent eventLoopEvent, mapKey string,
	nextStep *controllerEventLoop, dbQueries db.DatabaseQueries, log logr.Logger) bool {

	log = log.WithValues("workspaceID", newEvent.workspaceID, "name", newEvent.request.Name, "namespace", newEvent.request.Namespace)

	log.V(sharedutil.LogLevel_Debug).Info("preprocess event loop router received event:", "event", stringEventLoopEvent(&newEvent))

	var resource client.Object

	if newEvent.reqResource == managedgitopsv1alpha1.GitOpsDeploymentTypeName {
		resource = &managedgitopsv1alpha1.GitOpsDeployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      newEvent.request.Name,
				Namespace: newEvent.request.Namespace,
			},
		}
	} else if newEvent.reqResource == managedgitopsv1alpha1.GitOpsDeploymentSyncRunTypeName {
		resource = &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
			ObjectMeta: v1.ObjectMeta{
				Name:      newEvent.request.Name,
				Namespace: newEvent.request.Namespace,
			},
		}
	} else {
		log.Error(nil, "SEVERE - unexpected request resource type: "+string(newEvent.reqResource))
		return false
	}

	if err := newEvent.client.Get(ctx, client.ObjectKeyFromObject(resource), resource); err != nil {

		if !apierr.IsNotFound(err) {
			log.Error(err, "unable to retrieve resource during preprocess", "resource", resource)
			return true
		} else {
			log.Info("CR doesn't exist in namespace, so the resource is likely deleted.", "resource", resource)
		}

		// Past this point in the if block, the CR necessarily doesn't exist

		// Check the local cache, to see if we have seen this resource before
		{
			// TODO: NAO - is there a way we can avoid going to the database for gitopsdeplsyncruns?

			gitopsDeplUID, err := lookInCacheForAssociatedGitOpsDeplId(ctx, newEvent.reqResource, mapKey, task.resourcesSeen, task.resourcesSeenMutex, dbQueries, log)
			if err != nil {
				// If a generic error occurred (database or client connection issue), then log the error
				// and return true, so that we can retry.
				log.Error(err, "unable to retrieve resource from local cache", "resource", resource)
				return true
			}

			// Clear the cache after retrieving it, because the resource has necessarily been deleted from the namespace.
			task.resourcesSeenMutex.Lock()
			delete(task.resourcesSeen, mapKey)
			task.resourcesSeenMutex.Unlock()

			// If the GitOpsDeployment CR UID was found in the cache, then tag the event and emit it.
			if gitopsDeplUID != "" {
				newEvent.associatedGitopsDeplUID = gitopsDeplUID
				// Emit delete with uid found in local cache
				emitEvent(newEvent, nextStep, log)
				return false
			}
		}

		// If not found in local cache, check the database.

		if newEvent.reqResource == managedgitopsv1alpha1.GitOpsDeploymentSyncRunTypeName {

			var items []db.APICRToDatabaseMapping

			// 1) Go from GitOpsDeploymentSyncRun CR -> Sync Operation DB table, by searching
			// for the CR's namespace/name/workspace ID tuple.
			if err := dbQueries.UncheckedListAPICRToDatabaseMappingByAPINamespaceAndName(ctx,
				db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				newEvent.request.Name, newEvent.request.Namespace, newEvent.workspaceID,
				db.APICRToDatabaseMapping_DBRelationType_SyncOperation, &items); err != nil {

				log.Error(err, "unable to retrieve api to database mapping in preprocessEventLoopRouter")
				return true
			}

			if len(items) > 1 {
				log.Error(nil, "SEVERE: Unexpected number of items associated with GitOpsDeploymentSyncRun resource in database", "name", newEvent.request.Name, "namespace", newEvent.request.Namespace, "workspaceId", newEvent.workspaceID)
				return false

			} else if len(items) == 1 {

				// 2) We have found the pointer from GitOpsDeploymentSyncRun name/namespace to SyncOperation,
				// so next attempt to retrieve the SyncOperation
				syncOperation := &db.SyncOperation{
					SyncOperation_id: items[0].DBRelationKey,
				}
				if err := dbQueries.UncheckedGetSyncOperationById(ctx, syncOperation); err != nil {

					if !db.IsResultNotFoundError(err) {
						log.Error(err, "unable to retrieve sync operation for gitopsdeplsyncrun")
						return true
					}

					// Sync operation doesn't exist, so not much more we can do.
					return false
				}

				// Make sure the SyncOperation's target application hasn't already been deleted.
				if syncOperation.Application_id == "" {
					log.Info("syncOperation '" + syncOperation.SyncOperation_id + "'was found in processEvent, but application field was nil (likely it was deleted.)")
					return false
				}

				// 3) We have found the SyncOperation, now use it find the DeploymentToApplicationMapping
				// using the application id.
				dtam := db.DeploymentToApplicationMapping{
					Application_id: syncOperation.Application_id,
				}

				// 4) Finally, we have found the DeploymentToApplicationMapping, which contains the gitops cr uid
				if err := dbQueries.UncheckedGetDeploymentToApplicationMappingByApplicationId(ctx, &dtam); err != nil {

					if !db.IsResultNotFoundError(err) {
						log.Error(err, "unable to retrieve dtam for application id: "+dtam.Application_id)
						return true
					}

					// depl to app mapping for this application doesn't exist, so not much we can do.
					return false
				}

				// Success, tag the event and emit it
				newEvent.associatedGitopsDeplUID = dtam.Deploymenttoapplicationmapping_uid_id

				// Emit delete, with UID from database
				emitEvent(newEvent, nextStep, log)

				return false

			} else {
				// In this else block:
				// - the CR doesn't exist
				// - the local cache hasn't previously seen a CR with this name/namespace/resource type
				// - we couldn't find any reference to this CR in the database

				// So it's safe to ignore it (after logging it as INFO)
				log.Info("Deleted CR " + string(newEvent.reqResource) + " wasn't present in local cache or DB, so ignoring.")
				return false
			}

		} else if newEvent.reqResource == managedgitopsv1alpha1.GitOpsDeploymentTypeName {

			// Look for a corresponding DeploymentoApplicationMapping for the GitOpsDeployment CR in the namespace
			var items []db.DeploymentToApplicationMapping
			if err := dbQueries.UncheckedListDeploymentToApplicationMappingByNamespaceAndName(ctx, newEvent.request.Name, newEvent.request.Namespace,
				newEvent.workspaceID, &items); err != nil {
				log.Error(err, "unable to retrieve gitopsdeployment by namespacename in processEvent")
				return true
			}

			if len(items) > 1 {
				log.Error(nil, "SEVERE: unexpected number of items associated with GitOpsDeployment resource in database")
				return false

			} else if len(items) == 1 {
				newEvent.associatedGitopsDeplUID = items[0].Deploymenttoapplicationmapping_uid_id

				// Emit delete, with UID from database
				emitEvent(newEvent, nextStep, log)

				return false
			} else {
				// In this else block:
				// - the CR doesn't exist
				// - the local cache hasn't previously seen a CR with this name/namespace/resource type
				// - we couldn't find any reference to this CR in the database

				// So it's safe to ignore it (after logging it as INFO)
				log.Info("Deleted CR " + string(newEvent.reqResource) + " wasn't present in local cache or DB, so ignoring.")
				return false
			}
		}

	} else {

		// In this else block, the event we received is for a GitOpsDeployment(SyncRun) that exists in the namespace.

		// The UID that this CR had when we previously saw it
		previousGitopsDeplUID, err := lookInCacheForAssociatedGitOpsDeplId(ctx, newEvent.reqResource, mapKey, task.resourcesSeen, task.resourcesSeenMutex, dbQueries, log)
		if err != nil {
			log.Error(err, "unexpected error when checking database contents with local map", "mapKey", mapKey)
			return true
		}

		// Update the cache with the latest UID for this resource
		task.resourcesSeenMutex.Lock()
		task.resourcesSeen[mapKey] = string(resource.GetUID())
		task.resourcesSeenMutex.Unlock()

		if previousGitopsDeplUID != "" {
			emitEventForExistingResource(previousGitopsDeplUID, newEvent, resource, nextStep, log)
			return false
		}

		// Not found in the local cache, so check the database to see if this CR was previously processed
		{
			previousGitopsDeplUID, err = getGitOpsDeplIdFromDatabaseUsingNameAndNamespace(ctx, newEvent, dbQueries, log)
			if err != nil {
				log.Error(err, "unexpected error when resolving the gitopsdeplid using name and namespace")
				return true
			}

			// If we found a corresponding database entry (indicating this CR was previously processed) then
			// emit the event for it.
			if previousGitopsDeplUID != "" {
				emitEventForExistingResource(previousGitopsDeplUID, newEvent, resource, nextStep, log)
				return false
			}
		}

		// Finally, it's not in the local cache, it's not the database, but it exists in the namespace,
		// so it's just a never before seen/processed resource.
		if newEvent.reqResource == managedgitopsv1alpha1.GitOpsDeploymentSyncRunTypeName {

			gitopsDeplSyncRun, ok := resource.(*managedgitopsv1alpha1.GitOpsDeploymentSyncRun)
			if !ok {
				log.Error(nil, "SEVERE: unable to cast resource to GitOpsDeploymentSyncRun")
				return false
			}

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: v1.ObjectMeta{
					Name:      gitopsDeplSyncRun.Spec.GitopsDeploymentName,
					Namespace: resource.GetNamespace(),
				},
			}
			if err := newEvent.client.Get(ctx, client.ObjectKeyFromObject(gitopsDepl), gitopsDepl); err != nil {
				if !apierr.IsNotFound(err) {
					log.Error(err, "unable to retrieve gitopsdepl referenced by gitopsdeplsyncrun")
					return true
				}
				newEvent.associatedGitopsDeplUID = orphanedResourceGitopsDeplUID
			} else {
				newEvent.associatedGitopsDeplUID = string(gitopsDepl.GetUID())
			}

		} else {
			newEvent.associatedGitopsDeplUID = string(resource.GetUID())
		}

		emitEvent(newEvent, nextStep, log)

	}

	return false
}

// emitEvent passes the given event to the controller event loop
func emitEvent(event eventLoopEvent, nextStep *controllerEventLoop, log logr.Logger) {

	if nextStep == nil {
		log.Error(nil, "SEVERE: controllerEventLoop pointer should never be nil")
		return
	}

	log.V(sharedutil.LogLevel_Debug).Info("Emitting event to workspace event loop", "event", stringEventLoopEvent(&event))

	nextStep.eventLoopInputChannel <- event

}

func emitEventForExistingResource(gitopsDeplUID string, newEvent eventLoopEvent, resource client.Object, nextStep *controllerEventLoop, log logr.Logger) {

	// if it matches value from client, use provided id
	if gitopsDeplUID == string(resource.GetUID()) {
		newEvent.associatedGitopsDeplUID = gitopsDeplUID
		emitEvent(newEvent, nextStep, log)
		return
	}

	// If the cache contains a different value than the resource we just acquired, it's a delete of
	// an old resource, AND a create of a new one.

	// TODO: DEBT - create a concrete example of why this is needed.

	// otherwise, report delete and create

	newerEvent := newEvent
	newerEvent.associatedGitopsDeplUID = string(resource.GetUID())

	newEvent.associatedGitopsDeplUID = gitopsDeplUID

	if newerEvent.associatedGitopsDeplUID == newEvent.associatedGitopsDeplUID {
		log.Error(nil, "SEVERE - failed sanity check, two events had same uid in emitEventForExistingResource")
		return
	}

	// Report the deletion of the old resource
	emitEvent(newEvent, nextStep, log)

	// Report the creation of the new resource
	emitEvent(newerEvent, nextStep, log)

}

// getGitOpsDeplIdFromDatabaseUsingNameAndNamespace returns gitopsdepl cr uid if found, "" if not found, or error if a generic error occurred.
// An error should only be returned if there is a reasonable expectation of success if this function were to be retried.
// (for example, network issues tend to be ephemeral, so we can return an error for suspected network issues)
func getGitOpsDeplIdFromDatabaseUsingNameAndNamespace(ctx context.Context, newEvent eventLoopEvent,
	dbQueries db.DatabaseQueries, log logr.Logger) (string, error) {

	log = log.WithValues("name", newEvent.request.Name, "namespace", newEvent.request.Namespace)

	if newEvent.reqResource == managedgitopsv1alpha1.GitOpsDeploymentTypeName {

		var items []db.DeploymentToApplicationMapping

		if err := dbQueries.UncheckedListDeploymentToApplicationMappingByNamespaceAndName(ctx, newEvent.request.Name, newEvent.request.Namespace, newEvent.workspaceID, &items); err != nil {
			log.Error(err, "unable to list depltoappmapping in getGitOpsDeplFromDatabaseUsingNameAndNamespace")
			return "", err
		}

		if len(items) > 1 {
			log.Error(nil, "SEVERE: unexpected number of application mappings when resolving depltoappmapping for gitopsdepl")
			return "", nil
		} else if len(items) == 1 {
			return items[0].Deploymenttoapplicationmapping_uid_id, nil
		} else {
			return "", nil
		}

	} else if newEvent.reqResource == managedgitopsv1alpha1.GitOpsDeploymentSyncRunTypeName {

		var items []db.APICRToDatabaseMapping

		// 1) Go from GitOpsDeploymentSyncRun CR -> Sync Operation DB table, by searching
		// by the SyncRun CR's namespace/name/workspace ID tuple.
		if err := dbQueries.UncheckedListAPICRToDatabaseMappingByAPINamespaceAndName(ctx,
			db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
			newEvent.request.Name, newEvent.request.Namespace, newEvent.workspaceID,
			db.APICRToDatabaseMapping_DBRelationType_SyncOperation, &items); err != nil {
			log.Error(err, "unable to list api cr to db mapping in getGitOpsDeplFromDatabaseUsingNameAndNamespace")
			return "", err
		}

		if len(items) > 1 {
			log.Error(nil, "SEVERE: unexpected number of api cr to database mappings in getGitopsDeplUdFromDatabaseUsingNameAndNamespace")
			return "", nil

		} else if len(items) == 1 {

			// 2) We have found the pointer from GitOpsDeploymentSyncRun name/namespace to SyncOperation,
			// so next attempt to retrieve the SyncOperation
			syncOperation := &db.SyncOperation{
				SyncOperation_id: items[0].DBRelationKey,
			}
			if err := dbQueries.UncheckedGetSyncOperationById(ctx, syncOperation); err != nil {

				if db.IsResultNotFoundError(err) {
					log.V(sharedutil.LogLevel_Warn).Info("syncoperation id from apicr to db mapping didn't exist", "operationID", syncOperation.SyncOperation_id)
					return "", nil
				} else {
					log.Error(err, "unable to retrieve sync operation by id: "+syncOperation.SyncOperation_id)
					return "", err
				}

			}

			// It's possible the Application (that this sync operation is targetting) is already deleted, if so just return.
			if syncOperation.Application_id == "" {
				log.Info("syncOperation '" + syncOperation.SyncOperation_id + "' was found in getGitOpsDeplFromDatabaseUsingNameAndNamespace, but application field was nil (likely it was deleted.)")
				return "", nil
			}

			// 3) We have the application id, so go from Application ID DB table -> GitOpsDeployment CR
			dtam := db.DeploymentToApplicationMapping{
				Application_id: syncOperation.Application_id,
			}
			if err := dbQueries.UncheckedGetDeploymentToApplicationMappingByApplicationId(ctx, &dtam); err != nil {

				if db.IsResultNotFoundError(err) {
					log.V(sharedutil.LogLevel_Warn).Info("dtam not found when using application id from sync operation", "application_id", dtam.Application_id)
					return "", nil
				}

				log.Error(err, "unable to retrieve depltoappmapping by id: "+dtam.Application_id)
				return "", err
			}

			// Success, return the GitOpsDeployment UID from the entry
			return dtam.Deploymenttoapplicationmapping_uid_id, nil
		} else {
			// otherwise, not found.
			return "", nil
		}

	} else {
		return "", fmt.Errorf("SEVERE - unexpected request resource type")
	}

}

// lookInCacheForAssociatedGitOpsDeplId looks in 'resourcesSeen' cache for the UID of the GitOpsDeployment that corresponds
// to the given resource.
func lookInCacheForAssociatedGitOpsDeplId(ctx context.Context, resourceType managedgitopsv1alpha1.GitOpsResourceType, key string,
	resourcesSeen map[string]string, resourcesSeenMutex *sync.RWMutex, dbQueries db.DatabaseQueries, log logr.Logger) (string, error) {

	resourcesSeenMutex.RLock()
	mapUID, exists := resourcesSeen[key]
	resourcesSeenMutex.RUnlock()

	// Check the local cache
	if exists {

		if resourceType == managedgitopsv1alpha1.GitOpsDeploymentSyncRunTypeName {
			// If the resource is a GitOpsDeploymentSyncRun, we need to do an additional lookup up
			// to determie what the corresponding GitOpsDeployment is
			return getGitOpsDeplIdFromSyncRunCR(ctx, mapUID, dbQueries, log)

		} else if resourceType == managedgitopsv1alpha1.GitOpsDeploymentTypeName {
			// If the resource is a GitOpsDeployment, then the mapuid is the UID of the resource, so we are done.
			return mapUID, nil

		} else {
			return "", fmt.Errorf("unsupported resource type in getGitopsDeplIdFromMap")
		}
	}

	return "", nil

}

// getGitOpsDeplIdFromSyncRunCR returns gitopsdepl cr uid if found, "" if not found, or error if a generic error occurred
func getGitOpsDeplIdFromSyncRunCR(ctx context.Context, gitopsSyncRunUID string, dbQueries db.DatabaseQueries, log logr.Logger) (string, error) {

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

	// 2) We now have the SyncOperation table entry primary key, so retrieve it.
	syncOperation := &db.SyncOperation{
		SyncOperation_id: cr.DBRelationKey,
	}
	if err := dbQueries.UncheckedGetSyncOperationById(ctx, syncOperation); err != nil {

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

	// 3) We have the application id, so go from Application ID DB table -> GitOpsDeployment CR
	dtam := db.DeploymentToApplicationMapping{
		Application_id: syncOperation.Application_id,
	}
	if err := dbQueries.UncheckedGetDeploymentToApplicationMappingByApplicationId(ctx, &dtam); err != nil {

		if db.IsResultNotFoundError(err) {
			return "", nil
		}

		return "", fmt.Errorf("unable to retrieve depltoappmapping '%v' for sync run CR: %v", dtam.Application_id, err)
	}

	return dtam.Deploymenttoapplicationmapping_uid_id, nil

}
