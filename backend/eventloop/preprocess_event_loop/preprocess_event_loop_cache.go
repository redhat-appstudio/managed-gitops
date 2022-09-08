package preprocess_event_loop

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
)

// gitopsDeplSyncRunCache is a cache of which GitOpsDeploymentSyncRun K8s resources refer to which GitOpsDeployment K8s resources, by UID.
// It is a light-weight wrapper over the threadsafe hashicorp LRU cache
//
// The '.spec.gitopsDeploymentName' of GitOpsDeploymentSyncRun refers to a GitOpsDeployment in the namespace by name.
// We keep track of which SyncRun points to which GitOpsDeployment (because it can't change after it is set) in the database,
// and this cache caches that relationship, so that we don't need to go to the database every time.
type gitopsDeplSyncRunCache struct {

	// GitopsDeploymentSyncRun resource UID -> GitopsDeployment resource UID
	// - string -> string
	newCache *lru.Cache
}

func newGitOpsDeplSyncRunCache(maxCacheSize int) (*gitopsDeplSyncRunCache, error) {

	lruCache, err := lru.New(maxCacheSize)
	if err != nil {
		return nil, err
	}

	return &gitopsDeplSyncRunCache{
		newCache: lruCache,
	}, nil
}

func (cache *gitopsDeplSyncRunCache) deletedAssociatedGitOpsDeploymentForSyncRun(gitopsDeploymentSyncRunUID string) {

	cache.newCache.Remove(gitopsDeploymentSyncRunUID)
}

func (cache *gitopsDeplSyncRunCache) putAssociatedGitOpsDeploymentForSyncRun(gitopsDeploymentSyncRunUID string, gitopsDeploymentUID string) {

	cache.newCache.Add(gitopsDeploymentSyncRunUID, gitopsDeploymentUID)
}

func (cache *gitopsDeplSyncRunCache) getAssociatedGitOpsDeploymentForSyncRun(gitopsDeploymentSyncRunUID string) string {

	uid, exists := cache.newCache.Get(gitopsDeploymentSyncRunUID)
	if !exists {
		return ""
	}

	return (uid).(string)

}

func newLastUIDCache(maxCacheSize int) (*recentUIDCache, error) {

	lruCache, err := lru.New(maxCacheSize)
	if err != nil {
		return nil, err
	}

	return &recentUIDCache{
		resourcesSeen: lruCache,
	}, nil

}

// recentUIDCache keeps an in-memory cache of the K8s resource UID for GitOps API resources we have seen and/or processed
// It is a light-weight wrapper over the threadsafe hashicorp LRU cache.
type recentUIDCache struct {
	// (cache key) -> (uid of the sync/syncrun/etc resource, the last time it was seen)
	// - where cache key is a string representing an event by its type/name/namespace/namespace uid, generated in 'mapKey' from 'preprocessEventLoopRouter'
	// (resource type)-(resource name)-(resource namespace)-(resource namespace uid) -> UID of the resource the last time it was seen
	// resourcesSeen map[string]string
	resourcesSeen *lru.Cache

	// Note: resourcesSeen should only be accessed while holding 'resourcesSeenMutex'
	// resourcesSeenMutex *sync.RWMutex
}

func lastUIDCache_generateMapKey(newEvent eventlooptypes.EventLoopEvent) string {

	mapKey := string(newEvent.ReqResource) + "-" + newEvent.Request.Name + "-" + newEvent.Request.Namespace + "-" + newEvent.WorkspaceID
	return mapKey
}

func (cache *recentUIDCache) deleteMostRecentUIDForResourceFromEvent(newEvent eventlooptypes.EventLoopEvent) {

	mapKey := lastUIDCache_generateMapKey(newEvent)
	cache.resourcesSeen.Remove(mapKey)
}

func (cache *recentUIDCache) putMostRecentUIDForResourceFromEvent(uid string, newEvent eventlooptypes.EventLoopEvent) {

	mapKey := lastUIDCache_generateMapKey(newEvent)
	cache.resourcesSeen.Add(mapKey, uid)
}

func (cache *recentUIDCache) getMostRecentUIDForResourceFromEvent(newEvent eventlooptypes.EventLoopEvent) string {

	mapKey := lastUIDCache_generateMapKey(newEvent)
	mapUID, exists := cache.resourcesSeen.Get(mapKey)

	if !exists {
		return ""
	}

	return (mapUID).(string)
}
