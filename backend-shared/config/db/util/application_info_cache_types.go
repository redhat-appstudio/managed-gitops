package util

import (
	"context"
	"time"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

type ApplicationInfoCacheMessageType int

type applicationStateCacheEntry struct {
	appState        db.ApplicationState
	cacheExpireTime time.Time // after this time, the entry should be removed from the cache.

}

type applicationCacheEntry struct {
	app             db.Application
	cacheExpireTime time.Time // after this time, the entry should be removed from the cache.

}

type ApplicationInfoCache struct {
	channel chan applicationInfoCacheRequest
}

type applicationInfoCacheRequest struct {
	ctx                          context.Context
	msgType                      ApplicationInfoCacheMessageType
	createOrUpdateAppStateObject db.ApplicationState
	primaryKey                   string
	responseChannel              chan applicationInfoCacheResponse
}

type applicationInfoCacheResponse struct {
	application      db.Application
	applicationState db.ApplicationState
	err              error

	// valueFromCache is true if the value that was returned came from the cache, false otherwise.
	valueFromCache bool

	// Delete only: rowsAffectedForDelete contains the number of rows that were affected by the deletion oepration
	rowsAffectedForDelete int
}
