package util

import (
	"context"
	"fmt"

	"github.com/prometheus/common/log"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

// A wrapper over the ApplicationStateCache entries of the datbase

func NewApplicationInfoCache() *ApplicationInfoCache {

	res := &ApplicationInfoCache{
		channel: make(chan applicationInfoCacheRequest),
	}

	go applicationInfoCacheLoop(res.channel)

	return res
}

type ApplicationInfoCacheMessageType int

const (
	ApplicationStateCacheMessage_Get ApplicationInfoCacheMessageType = iota
	ApplicationCacheMessage_Get      ApplicationInfoCacheMessageType = iota
	ApplicationStateCacheMessage_Create
	ApplicationStateCacheMessage_Update
	ApplicationStateCacheMessage_Delete
)

type ApplicationInfoCache struct {
	channel chan applicationInfoCacheRequest
}

type applicationInfoCacheRequest struct {
	ctx                          context.Context
	msgType                      ApplicationInfoCacheMessageType
	createOrUpdateAppObject      db.Application
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

	cacheAppState := map[string]db.ApplicationState{}
	cacheApp := map[string]db.Application{}

	dbQueries, err := db.NewProductionPostgresDBQueries(false)
	if err != nil {
		log.Error(err, "SEVERE: unexpected error in calling dbQueries")
		return
	}

	for {

		request := <-inputChan
		if request.msgType == ApplicationStateCacheMessage_Get {
			processGetAppStateMessage(dbQueries, request, cacheAppState)

		} else if request.msgType == ApplicationStateCacheMessage_Create {
			processCreateAppStateMessage(dbQueries, request, cacheAppState)

		} else if request.msgType == ApplicationStateCacheMessage_Update {
			processUpdateAppStateMessage(dbQueries, request, cacheAppState)

		} else if request.msgType == ApplicationStateCacheMessage_Delete {
			processDeleteAppStateMessage(dbQueries, request, cacheAppState)

		} else if request.msgType == ApplicationCacheMessage_Get {
			processGetAppMessage(dbQueries, request, cacheApp)

		} else {
			log.Error(nil, "SEVERE: unimplemented message type")
			continue
		}

	}
}

func processCreateAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cache map[string]db.ApplicationState) {
	err := dbQueries.CreateApplicationState(req.ctx, &req.createOrUpdateAppStateObject)

	if err != nil {
		// Create the cache on success
		var appState db.ApplicationState = req.createOrUpdateAppStateObject
		cache[req.primaryKey] = appState
	}

	req.responseChannel <- applicationInfoCacheResponse{
		err: err,
	}

}

func processUpdateAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cache map[string]db.ApplicationState) {

	err := dbQueries.UpdateApplicationState(req.ctx, &req.createOrUpdateAppStateObject)

	if err != nil {
		// Update the cache on success
		var appState db.ApplicationState = req.createOrUpdateAppStateObject
		cache[req.primaryKey] = appState
	}

	req.responseChannel <- applicationInfoCacheResponse{
		err: err,
	}

}

func processDeleteAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cache map[string]db.ApplicationState) {

	if db.IsEmpty(req.primaryKey) {
		log.Error(fmt.Errorf("PrimaryKey should not be nil"), "SEVERE: "+req.primaryKey+" not found")
	}

	// Remove from cache
	delete(cache, req.primaryKey)

	// Remove from DB
	rowsAffected, err := dbQueries.DeleteApplicationStateById(req.ctx, req.primaryKey)

	req.responseChannel <- applicationInfoCacheResponse{
		rowsAffectedForDelete: rowsAffected,
		err:                   err,
	}

}

func processGetAppStateMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cache map[string]db.ApplicationState) error {

	appState := db.ApplicationState{
		Applicationstate_application_id: req.primaryKey,
	}

	var err error
	var valueFromCache bool

	if db.IsEmpty(req.primaryKey) {
		return fmt.Errorf("PrimaryKey should not be nil: " + req.primaryKey + " not found")
	}

	res, exists := cache[appState.Applicationstate_application_id]

	if !exists {
		// If it's not in the cache, then get it from the database
		valueFromCache = false
		err = dbQueries.GetApplicationStateById(req.ctx, &appState)
		if err != nil {
			appState = db.ApplicationState{}
		}

		// Update the cache if we get a result from the database
		// Update valueFromCache to false, since data is retrieved from db
		if err == nil && appState.Applicationstate_application_id != "" {
			cache[appState.Applicationstate_application_id] = appState
		}

	} else {
		// If it is in the cache, return it from the cache
		valueFromCache = true
		appState = res
	}

	req.responseChannel <- applicationInfoCacheResponse{
		applicationState: appState,
		valueFromCache:   valueFromCache,
		err:              err,
	}
	return nil
}

func processGetAppMessage(dbQueries db.DatabaseQueries, req applicationInfoCacheRequest, cache map[string]db.Application) error {

	app := db.Application{
		Application_id: req.primaryKey,
	}

	var err error
	var valueFromCache bool

	if db.IsEmpty(req.primaryKey) {
		return fmt.Errorf("PrimaryKey should not be nil: " + req.primaryKey + " not found")
	}

	res, exists := cache[app.Application_id]

	if !exists {
		// If it's not in the cache, then get it from the database
		valueFromCache = false
		err = dbQueries.GetApplicationById(req.ctx, &app)
		if err != nil {
			app = db.Application{}
		}

		// Update the cache if we get a result from the database
		// Update valueFromCache to false, since data is retrieved from db
		if err == nil && app.Application_id != "" {
			cache[app.Application_id] = app
		}

	} else {
		// If it is in the cache, return it from the cache
		valueFromCache = true
		app = res
	}

	req.responseChannel <- applicationInfoCacheResponse{
		application:    app,
		valueFromCache: valueFromCache,
		err:            err,
	}
	return nil
}
