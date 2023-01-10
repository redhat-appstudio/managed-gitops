package application_info_cache

import (
	"context"
	"time"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
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
	ctx     context.Context
	msgType ApplicationInfoCacheMessageType

	// if createappstate or updateappstate is called, this value will contain the db.ApplicationState object provided by the caller
	// otherwise, it will be empty
	createOrUpdateAppStateObject db.ApplicationState

	// primaryKey is the application/applicationstate database primary key id
	// Note: it is no set for Create or Update, for that, use 'createOrUpdateAppStateObject'
	primaryKey      string
	responseChannel chan applicationInfoCacheResponse
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
