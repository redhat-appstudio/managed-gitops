package util

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/prometheus/common/log"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

// A wrapper over the ApplicationStateCache entries of the database
// Note: This should only be used by cluster-agent's application controller.
func NewApplicationInfoCache() *ApplicationInfoCache {

	res := &ApplicationInfoCache{
		channel: make(chan applicationInfoCacheRequest),
	}

	go applicationInfoCacheLoop(res.channel)

	return res
}

const (
	ApplicationStateCacheMessage_Get ApplicationInfoCacheMessageType = iota
	ApplicationCacheMessage_Get
	ApplicationStateCacheMessage_Create
	ApplicationStateCacheMessage_Update
	ApplicationStateCacheMessage_Delete
	ApplicationInfoCacheMessage_ExpireCacheEntries
)

func (asc *ApplicationInfoCache) GetApplicationById(ctx context.Context, id string) (db.Application, bool, error) {

	responseChannel := make(chan applicationInfoCacheResponse)

	asc.channel <- applicationInfoCacheRequest{
		ctx:             ctx,
		primaryKey:      id,
		msgType:         ApplicationCacheMessage_Get,
		responseChannel: responseChannel,
	}

	var response applicationInfoCacheResponse

	select {
	case response = <-responseChannel:
	case <-ctx.Done():
		return db.Application{}, response.valueFromCache, fmt.Errorf("context cancelled in GetApplicationById")
	}

	if response.err != nil {
		return db.Application{}, response.valueFromCache, response.err
	}

	return response.application, response.valueFromCache, nil

}

func (asc *ApplicationInfoCache) GetApplicationStateById(ctx context.Context, id string) (db.ApplicationState, bool, error) {

	responseChannel := make(chan applicationInfoCacheResponse)

	asc.channel <- applicationInfoCacheRequest{
		ctx:             ctx,
		primaryKey:      id,
		msgType:         ApplicationStateCacheMessage_Get,
		responseChannel: responseChannel,
	}

	var response applicationInfoCacheResponse

	select {
	case response = <-responseChannel:
	case <-ctx.Done():
		return db.ApplicationState{}, response.valueFromCache, fmt.Errorf("context cancelled in GetApplicationStateById")
	}

	if response.err != nil {
		return db.ApplicationState{}, response.valueFromCache, response.err
	}

	return response.applicationState, response.valueFromCache, nil

}

func (asc *ApplicationInfoCache) CreateApplicationState(ctx context.Context, appState db.ApplicationState) error {
	responseChannel := make(chan applicationInfoCacheResponse)

	asc.channel <- applicationInfoCacheRequest{
		ctx:                          ctx,
		createOrUpdateAppStateObject: appState,
		msgType:                      ApplicationStateCacheMessage_Create,
		responseChannel:              responseChannel,
	}

	var response applicationInfoCacheResponse

	select {
	case response = <-responseChannel:
	case <-ctx.Done():
		return fmt.Errorf("context cancelled in CreateApplicationState")
	}

	if response.err != nil {
		return response.err
	}

	return nil

}
func (asc *ApplicationInfoCache) UpdateApplicationState(ctx context.Context, appState db.ApplicationState) error {

	responseChannel := make(chan applicationInfoCacheResponse)

	asc.channel <- applicationInfoCacheRequest{
		ctx:                          ctx,
		createOrUpdateAppStateObject: appState,
		msgType:                      ApplicationStateCacheMessage_Update,
		responseChannel:              responseChannel,
	}

	var response applicationInfoCacheResponse

	select {
	case response = <-responseChannel:
	case <-ctx.Done():
		return fmt.Errorf("context cancelled in UpdateApplicationState")
	}

	if response.err != nil {
		return response.err
	}

	return nil
}

func (asc *ApplicationInfoCache) DeleteApplicationStateById(ctx context.Context, id string) (int, error) {
	responseChannel := make(chan applicationInfoCacheResponse)

	asc.channel <- applicationInfoCacheRequest{
		ctx:             ctx,
		primaryKey:      id,
		msgType:         ApplicationStateCacheMessage_Delete,
		responseChannel: responseChannel,
	}

	var response applicationInfoCacheResponse

	select {
	case response = <-responseChannel:
	case <-ctx.Done():
		return 0, fmt.Errorf("context cancelled in DeleteApplicationStateById")
	}

	if response.err != nil {
		return response.rowsAffectedForDelete, response.err
	}

	return response.rowsAffectedForDelete, nil
}

func applicationInfoCacheLoop(inputChan chan applicationInfoCacheRequest) {

	startTimer(&ApplicationInfoCache{
		channel: inputChan,
	})

	cacheAppState := map[string]applicationStateCacheEntry{}
	cacheApp := map[string]applicationCacheEntry{}

	dbQueries, err := db.NewProductionPostgresDBQueries(false)
	if err != nil {
		log.Error(err, "SEVERE: unexpected error in calling dbQueries")
		return
	}

	for {

		request := <-inputChan
		if request.msgType == ApplicationStateCacheMessage_Get {
			processGetAppStateMessage(dbQueries, request, cacheApp, cacheAppState)

		} else if request.msgType == ApplicationStateCacheMessage_Create {
			processCreateAppStateMessage(dbQueries, request, cacheApp, cacheAppState)

		} else if request.msgType == ApplicationStateCacheMessage_Update {
			processUpdateAppStateMessage(dbQueries, request, cacheApp, cacheAppState)

		} else if request.msgType == ApplicationStateCacheMessage_Delete {
			processDeleteAppStateMessage(dbQueries, request, cacheApp, cacheAppState)

		} else if request.msgType == ApplicationCacheMessage_Get {
			processGetAppMessage(dbQueries, request, cacheApp, cacheAppState)

		} else if request.msgType == ApplicationInfoCacheMessage_ExpireCacheEntries {
			processExpireCacheEntriesMessage(cacheApp, cacheAppState, inputChan)

		} else {
			log.Error(nil, "SEVERE: unimplemented message type")
			continue
		}

	}
}

func processCreateAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry) {
	err := dbQueries.CreateApplicationState(req.ctx, &req.createOrUpdateAppStateObject)

	if err == nil {
		// Create the cache on success
		var appState db.ApplicationState = req.createOrUpdateAppStateObject

		if entryInCache, exists := cacheAppState[req.primaryKey]; !exists {
			entryInCache.appState = appState
			entryInCache.cacheExpireTime = time.Now().Add(1 * time.Minute)
			cacheAppState[req.primaryKey] = entryInCache

		}
	} else {
		// An error occurred, so remove the cache entry from both the application state, and the application,
		// so that it can be re-acquired.
		// A common error returned by dbQueries above, will be that the applicate state cannot be updated, because the application no
		// longer exists in the database.
		// This is normal, and occurs when a GitOpsDeployment is deleted (and then the corresponding Application/Application state are deleted as well).
		// In this case, we need to remove the Application/ApplicationState from the DB, as they have likely been deleted (and thus should no longer be cached.)
		delete(cacheApp, req.createOrUpdateAppStateObject.Applicationstate_application_id)
		delete(cacheAppState, req.createOrUpdateAppStateObject.Applicationstate_application_id)
	}

	req.responseChannel <- applicationInfoCacheResponse{
		err: err,
	}

}

func processUpdateAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry) {

	err := dbQueries.UpdateApplicationState(req.ctx, &req.createOrUpdateAppStateObject)

	if err == nil {
		// Update the cache on success
		var appState db.ApplicationState = req.createOrUpdateAppStateObject
		if entryInCache, exists := cacheAppState[req.primaryKey]; !exists {
			entryInCache.appState = appState
			entryInCache.cacheExpireTime = time.Now().Add(1 * time.Minute)
			cacheAppState[req.primaryKey] = entryInCache
		}
	} else {
		delete(cacheApp, req.createOrUpdateAppStateObject.Applicationstate_application_id)
		delete(cacheAppState, req.createOrUpdateAppStateObject.Applicationstate_application_id)
	}

	req.responseChannel <- applicationInfoCacheResponse{
		err: err,
	}

}

func processDeleteAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry) {

	if db.IsEmpty(req.primaryKey) {
		log.Error(fmt.Errorf("PrimaryKey should not be nil"), "SEVERE: "+req.primaryKey+" not found")
	}

	// Remove from cache
	delete(cacheAppState, req.primaryKey)
	delete(cacheApp, req.primaryKey)

	// Remove from DB
	rowsAffected, err := dbQueries.DeleteApplicationStateById(req.ctx, req.primaryKey)

	req.responseChannel <- applicationInfoCacheResponse{
		rowsAffectedForDelete: rowsAffected,
		err:                   err,
	}

}

func processGetAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry) {

	appState := db.ApplicationState{
		Applicationstate_application_id: req.primaryKey,
	}

	var err error
	var valueFromCache bool

	if db.IsEmpty(req.primaryKey) {
		log.Error("PrimaryKey should not be nil: " + req.primaryKey + " not found")
	}

	res, exists := cacheAppState[appState.Applicationstate_application_id]
	if !exists {
		// If it's not in the cache, then get it from the database
		valueFromCache = false
		err = dbQueries.GetApplicationStateById(req.ctx, &appState)
		if err != nil {
			appState = db.ApplicationState{}
		} else {
			delete(cacheAppState, req.createOrUpdateAppStateObject.Applicationstate_application_id)
			delete(cacheApp, req.createOrUpdateAppStateObject.Applicationstate_application_id)
		}

		// Update the cache if we get a result from the database
		// Update valueFromCache to false, since data is retrieved from db
		if err == nil && appState.Applicationstate_application_id != "" {
			if entryInCache, exists := cacheAppState[appState.Applicationstate_application_id]; !exists {
				entryInCache.appState = appState
				entryInCache.cacheExpireTime = time.Now().Add(1 * time.Minute)
				cacheAppState[appState.Applicationstate_application_id] = entryInCache
			}
		}

	} else {
		// If it is in the cache, return it from the cache
		valueFromCache = true
		appState = res.appState
	}

	req.responseChannel <- applicationInfoCacheResponse{
		applicationState: appState,
		valueFromCache:   valueFromCache,
		err:              err,
	}

}

func processGetAppMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry) {

	app := db.Application{
		Application_id: req.primaryKey,
	}

	var err error
	var valueFromCache bool

	if db.IsEmpty(req.primaryKey) {
		log.Error("PrimaryKey should not be nil: " + req.primaryKey + " not found")
	}

	if res, exists := cacheApp[app.Application_id]; !exists {
		// If it's not in the cache, then get it from the database
		valueFromCache = false
		err = dbQueries.GetApplicationById(req.ctx, &app)
		if err != nil {
			app = db.Application{}
		} else {
			delete(cacheApp, req.createOrUpdateAppStateObject.Applicationstate_application_id)
			delete(cacheAppState, req.createOrUpdateAppStateObject.Applicationstate_application_id)
		}

		// Update the cache if we get a result from the database
		// Update valueFromCache to false, since data is retrieved from db
		if err == nil && app.Application_id != "" {
			if entryInCache, exists := cacheApp[app.Application_id]; !exists {
				entryInCache.app = app
				entryInCache.cacheExpireTime = time.Now().Add(1 * time.Minute)
				cacheApp[app.Application_id] = entryInCache
			}
		}

	} else {
		// If it is in the cache, return it from the cache
		valueFromCache = true
		app = res.app
	}

	req.responseChannel <- applicationInfoCacheResponse{
		application:    app,
		valueFromCache: valueFromCache,
		err:            err,
	}

}

func processExpireCacheEntriesMessage(cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry, inputChan chan applicationInfoCacheRequest) {

	for key, elements := range cacheApp {
		if time.Now().After(elements.cacheExpireTime) {
			delete(cacheApp, key)
		}
	}

	for key, elements := range cacheAppState {
		if time.Now().After(elements.cacheExpireTime) {
			delete(cacheAppState, key)
		}
	}

	startTimer(&ApplicationInfoCache{
		channel: inputChan,
	})
}

// Timer for every minute to expire cache entries
// the AIC loop should expire old cache entries
func startTimer(asc *ApplicationInfoCache) {
	go func() {
		// Up to 1 second of jitter
		// #nosec
		jitter := time.Duration(int64(time.Millisecond) * int64(rand.Float64()*1000))

		// Wait 60 seconds (plus a little) for the timer to complete
		statusUpdateTimer := time.NewTimer(time.Second*60 + jitter)
		<-statusUpdateTimer.C

		// Send the message, indicating its time to expire the old cache entries
		asc.channel <- applicationInfoCacheRequest{
			msgType: ApplicationInfoCacheMessage_ExpireCacheEntries,
		}

	}()
}
