package application_info_cache

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// The cache exists because we do not want to overwhelm the controller (or the database) with various operations frequently: most
// of the time these reconciliations are changes that might not be useful for us.
//
// To avoid unnecessary read/write to the database, the cache is being implemented in backend-shared which will be used
// by cluster-agent's Application contrller reconciliation.
//
// The cache should only be used by 'application_controller.go'.
//
// The cache:
// - will always return the most recent ApplicationState value from the database
// - in contrast, the cache will return a value for an Application that is at most 60 seconds old
//   (the Application in the cache state will be eventually consistent with the database)
//     - since it is eventually consistent, the calling code needs to be aware of this in its logic.

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
	ApplicationInfoCacheMessage_DebugOnly_Shutdown
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

// DebugOnly_Shutdown should only be called in unit tests. This function terminates the cache loop.
func (asc *ApplicationInfoCache) DebugOnly_Shutdown(ctx context.Context) {

	responseChannel := make(chan applicationInfoCacheResponse)

	asc.channel <- applicationInfoCacheRequest{
		ctx:             ctx,
		msgType:         ApplicationInfoCacheMessage_DebugOnly_Shutdown,
		responseChannel: responseChannel,
	}

	<-responseChannel

}

func applicationInfoCacheLoop(inputChan chan applicationInfoCacheRequest) {

	startTimer(&ApplicationInfoCache{
		channel: inputChan,
	})

	log := log.FromContext(context.Background())

	cacheAppState := map[string]applicationStateCacheEntry{}
	cacheApp := map[string]applicationCacheEntry{}

	dbQueries, err := db.NewSharedProductionPostgresDBQueries(false)
	if err != nil {
		log.Error(err, "SEVERE: unexpected error in calling dbQueries")
		return
	}

outer_for_loop:
	for {

		request := <-inputChan
		if request.msgType == ApplicationStateCacheMessage_Get {
			processGetAppStateMessage(dbQueries, request, cacheApp, cacheAppState, log)

		} else if request.msgType == ApplicationStateCacheMessage_Create {
			processCreateAppStateMessage(dbQueries, request, cacheApp, cacheAppState)

		} else if request.msgType == ApplicationStateCacheMessage_Update {
			processUpdateAppStateMessage(dbQueries, request, cacheApp, cacheAppState, log)

		} else if request.msgType == ApplicationStateCacheMessage_Delete {
			processDeleteAppStateMessage(dbQueries, request, cacheApp, cacheAppState, log)

		} else if request.msgType == ApplicationCacheMessage_Get {
			processGetAppMessage(dbQueries, request, cacheApp, cacheAppState, log)

		} else if request.msgType == ApplicationInfoCacheMessage_ExpireCacheEntries {
			processExpireCacheEntriesMessage(cacheApp, cacheAppState, inputChan)

		} else if request.msgType == ApplicationInfoCacheMessage_DebugOnly_Shutdown {
			processDebugOnlyShutdownMessage(request, log)
			break outer_for_loop

		} else {
			log.Error(nil, "SEVERE: unimplemented message type")
			continue
		}

	}
}

func processDebugOnlyShutdownMessage(req applicationInfoCacheRequest, log logr.Logger) {
	log.Info("DEBUG-ONLY: terminating info cache. You should only see this in unit tests.")
	req.responseChannel <- applicationInfoCacheResponse{}
}

func processCreateAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry) {
	err := dbQueries.CreateApplicationState(req.ctx, &req.createOrUpdateAppStateObject)

	if err == nil {

		// Create the cache on success
		var appState db.ApplicationState = req.createOrUpdateAppStateObject

		newCacheEntry := applicationStateCacheEntry{
			appState:        appState,
			cacheExpireTime: time.Now().Add(1 * time.Minute),
		}

		cacheAppState[appState.Applicationstate_application_id] = newCacheEntry

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

func processUpdateAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry, log logr.Logger) {

	err := dbQueries.UpdateApplicationState(req.ctx, &req.createOrUpdateAppStateObject)

	if err == nil {

		// Update the cache on success
		var appState db.ApplicationState = req.createOrUpdateAppStateObject

		newCacheEntry := applicationStateCacheEntry{
			appState:        appState,
			cacheExpireTime: time.Now().Add(1 * time.Minute),
		}

		cacheAppState[appState.Applicationstate_application_id] = newCacheEntry

	} else {
		// Invalidate the cache on database error, and return the error back to the caller
		delete(cacheApp, req.createOrUpdateAppStateObject.Applicationstate_application_id)
		delete(cacheAppState, req.createOrUpdateAppStateObject.Applicationstate_application_id)
	}

	req.responseChannel <- applicationInfoCacheResponse{
		err: err,
	}

}

func processDeleteAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry, log logr.Logger) {

	if db.IsEmpty(req.primaryKey) {
		err := fmt.Errorf("SEVERE: PrimaryKey should not be nil")
		log.Error(err, "")
		req.responseChannel <- applicationInfoCacheResponse{
			rowsAffectedForDelete: 0,
			err:                   err,
		}
		return
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

func processGetAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry, log logr.Logger) {

	appState := db.ApplicationState{
		Applicationstate_application_id: req.primaryKey,
	}

	if db.IsEmpty(req.primaryKey) {
		err := fmt.Errorf("SEVERE: PrimaryKey should not be nil: " + req.primaryKey + " not found")
		log.Error(err, "")
		req.responseChannel <- applicationInfoCacheResponse{
			applicationState: db.ApplicationState{},
			valueFromCache:   false,
			err:              err,
		}
		return
	}

	var err error
	var valueFromCache bool

	res, exists := cacheAppState[appState.Applicationstate_application_id]
	if !exists {
		// Since it's not in the cache, we get it from the database

		// Update valueFromCache to false, since data is retrieved from DB
		valueFromCache = false

		if err = dbQueries.GetApplicationStateById(req.ctx, &appState); err != nil {

			// If there is an error, invalidate the cache for both the Application and ApplicationState
			delete(cacheAppState, appState.Applicationstate_application_id)
			delete(cacheApp, appState.Applicationstate_application_id)

			appState = db.ApplicationState{}

		} else {
			// Update the cache if we successfully get a result from the database
			if appState.Applicationstate_application_id != "" {

				newCacheEntry := applicationStateCacheEntry{
					appState:        appState,
					cacheExpireTime: time.Now().Add(1 * time.Minute),
				}

				cacheAppState[appState.Applicationstate_application_id] = newCacheEntry

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

func processGetAppMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cacheApp map[string]applicationCacheEntry, cacheAppState map[string]applicationStateCacheEntry, log logr.Logger) {
	app := db.Application{
		Application_id: req.primaryKey,
	}

	if db.IsEmpty(req.primaryKey) {
		err := fmt.Errorf("SEVERE: PrimaryKey should not be nil: " + req.primaryKey + " not found")
		log.Error(err, "")
		req.responseChannel <- applicationInfoCacheResponse{
			application:    db.Application{},
			valueFromCache: false,
			err:            err,
		}
		return
	}

	var err error
	var valueFromCache bool

	if res, exists := cacheApp[app.Application_id]; !exists {
		// If it's not in the cache, then get it from the database

		// Update valueFromCache to false, since data is retrieved from db
		valueFromCache = false

		if err = dbQueries.GetApplicationById(req.ctx, &app); err != nil {
			// Error occurred: invalidate the cache and return the error
			delete(cacheApp, app.Application_id)
			delete(cacheAppState, app.Application_id)

			app = db.Application{}

		} else {
			// No error, so update the cache with the result from the database

			if app.Application_id != "" {
				newCacheEntry := applicationCacheEntry{
					app:             app,
					cacheExpireTime: time.Now().Add(1 * time.Minute),
				}

				cacheApp[app.Application_id] = newCacheEntry
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
