package eventloop

import (
	"sync"

	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
)

// gitopsDeplSyncRunCache is a cache of which GitOpsDeploymentSyncRun K8s resources refer to which GitOpsDeployment K8s resources, by UID.
//
// The '.spec.gitopsDeploymentName' of GitOpsDeploymentSyncRun refers to a GitOpsDeployment in the namespace by name.
// We keep track of which SyncRun points to which GitOpsDeployment (because it can't change after it is set) in the database,
// and this cache caches that relationship, so that we don't need to go to the database every time.
type gitopsDeplSyncRunCache struct {

	// GitopsDeploymentSyncRun resource UID -> GitopsDeployment resource UID
	gitopsDeplUID      map[string]string
	gitopsDeplUIDMutex *sync.RWMutex
}

func newGitOpsDeplSyncRunCache() *gitopsDeplSyncRunCache {
	return &gitopsDeplSyncRunCache{
		gitopsDeplUID:      map[string]string{},
		gitopsDeplUIDMutex: &sync.RWMutex{},
	}
}

func (cache *gitopsDeplSyncRunCache) deletedAssociatedGitOpsDeploymentForSyncRun(gitopsDeploymentSyncRunUID string) {
	cache.gitopsDeplUIDMutex.Lock()
	defer cache.gitopsDeplUIDMutex.Unlock()

	delete(cache.gitopsDeplUID, gitopsDeploymentSyncRunUID)

}

func (cache *gitopsDeplSyncRunCache) putAssociatedGitOpsDeploymentForSyncRun(gitopsDeploymentSyncRunUID string, gitopsDeploymentUID string) {
	cache.gitopsDeplUIDMutex.Lock()
	defer cache.gitopsDeplUIDMutex.Unlock()

	cache.gitopsDeplUID[gitopsDeploymentSyncRunUID] = gitopsDeploymentUID

}

func (cache *gitopsDeplSyncRunCache) getAssociatedGitOpsDeploymentForSyncRun(gitopsDeploymentSyncRunUID string) string {
	cache.gitopsDeplUIDMutex.RLock()
	defer cache.gitopsDeplUIDMutex.RUnlock()

	uid, exists := cache.gitopsDeplUID[gitopsDeploymentSyncRunUID]
	if !exists {
		uid = ""
	}
	return uid
}

func newLastUIDCache() *recentUIDCache {

	return &recentUIDCache{
		resourcesSeen:      map[string]string{},
		resourcesSeenMutex: &sync.RWMutex{},
	}

}

// recentUIDCache keeps an in-memory cache of the K8s resource UID for GitOps API resources we have seen and/or processed
type recentUIDCache struct {
	// (cache key) -> (uid of the sync/syncrun/etc resource, the last time it was seen)
	// - where cache key is a string representing an event by its type/name/namespace/namespace uid, generated in 'mapKey' from 'preprocessEventLoopRouter'
	// (resource type)-(resource name)-(resource namespace)-(resource namespace uid) -> UID of the resource the last time it was seen
	resourcesSeen map[string]string
	// Note: resourcesSeen should only be accessed while holding 'resourcesSeenMutex'
	resourcesSeenMutex *sync.RWMutex
}

func lastUIDCache_generateMapKey(newEvent eventlooptypes.EventLoopEvent) string {
	mapKey := string(newEvent.ReqResource) + "-" + newEvent.Request.Name + "-" + newEvent.Request.Namespace + "-" + newEvent.WorkspaceID
	return mapKey
}

func (cache *recentUIDCache) deleteMostRecentUIDForResourceFromEvent(newEvent eventlooptypes.EventLoopEvent) {
	mapKey := lastUIDCache_generateMapKey(newEvent)

	cache.resourcesSeenMutex.Lock()
	defer cache.resourcesSeenMutex.Unlock()

	delete(cache.resourcesSeen, mapKey)
}

func (cache *recentUIDCache) putMostRecentUIDForResourceFromEvent(uid string, newEvent eventlooptypes.EventLoopEvent) {

	mapKey := lastUIDCache_generateMapKey(newEvent)

	cache.resourcesSeenMutex.Lock()
	defer cache.resourcesSeenMutex.Unlock()
	cache.resourcesSeen[mapKey] = uid

}

func (cache *recentUIDCache) getMostRecentUIDForResourceFromEvent(newEvent eventlooptypes.EventLoopEvent) string {

	mapKey := lastUIDCache_generateMapKey(newEvent)

	cache.resourcesSeenMutex.RLock()
	defer cache.resourcesSeenMutex.RUnlock()

	mapUID, exists := cache.resourcesSeen[mapKey]

	if !exists {
		mapUID = ""
	}

	return mapUID
}
