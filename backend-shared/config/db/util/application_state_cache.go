package util

import (
	"context"
	"fmt"

	"github.com/prometheus/common/log"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

// A wrapper over the ApplicationStateCache entries of the datbase

func NewApplicationStateCache() *ApplicationStateCache {

	res := &ApplicationStateCache{
		channel: make(chan applicationStateCacheRequest),
	}

	go applicationStateCacheLoop(res.channel)

	return res
}

type ApplicationStateCacheMessageType int

const (
	ApplicationStateCacheMessage_Get ApplicationStateCacheMessageType = iota
	ApplicationStateCacheMessage_Create
	ApplicationStateCacheMessage_Update
	ApplicationStateCacheMessage_Delete
)

type ApplicationStateCache struct {
	channel chan applicationStateCacheRequest
}

type applicationStateCacheRequest struct {
	ctx     context.Context
	msgType ApplicationStateCacheMessageType

	createOrUpdateObject db.ApplicationState
	primaryKey           string
	responseChannel      chan applicationStateCacheResponse
}

type applicationStateCacheResponse struct {
	applicationState db.ApplicationState
	err              error

	// valueFromCache is true if the value that was returned came from the cache, false otherwise.
	valueFromCache bool

	// Delete only: rowsAffectedForDelete contains the number of rows that were affected by the deletion oepration
	rowsAffectedForDelete int
}

func (asc *ApplicationStateCache) GetApplicationStateById(ctx context.Context, id string) (db.ApplicationState, bool, error) {

	responseChannel := make(chan applicationStateCacheResponse)

	asc.channel <- applicationStateCacheRequest{
		ctx:             ctx,
		primaryKey:      id,
		msgType:         ApplicationStateCacheMessage_Get,
		responseChannel: responseChannel,
	}

	var response applicationStateCacheResponse

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

func (asc *ApplicationStateCache) CreateApplicationState(ctx context.Context, appState db.ApplicationState) error {
	responseChannel := make(chan applicationStateCacheResponse)

	asc.channel <- applicationStateCacheRequest{
		ctx:                  ctx,
		createOrUpdateObject: appState,
		msgType:              ApplicationStateCacheMessage_Create,
		responseChannel:      responseChannel,
	}

	var response applicationStateCacheResponse

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
func (asc *ApplicationStateCache) UpdateApplicationState(ctx context.Context, appState db.ApplicationState) error {

	responseChannel := make(chan applicationStateCacheResponse)

	asc.channel <- applicationStateCacheRequest{
		ctx:                  ctx,
		createOrUpdateObject: appState,
		msgType:              ApplicationStateCacheMessage_Update,
		responseChannel:      responseChannel,
	}

	var response applicationStateCacheResponse

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

func (asc *ApplicationStateCache) DeleteApplicationStateById(ctx context.Context, id string) (int, error) {
	responseChannel := make(chan applicationStateCacheResponse)

	asc.channel <- applicationStateCacheRequest{
		ctx:             ctx,
		primaryKey:      id,
		msgType:         ApplicationStateCacheMessage_Delete,
		responseChannel: responseChannel,
	}

	var response applicationStateCacheResponse

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

func applicationStateCacheLoop(inputChan chan applicationStateCacheRequest) {

	cache := map[string]db.ApplicationState{}

	dbQueries, err := db.NewProductionPostgresDBQueries(false)
	if err != nil {
		log.Error(err, "SEVERE: unexpected error in calling dbQueries")
		return
	}

	for {

		request := <-inputChan
		if request.msgType == ApplicationStateCacheMessage_Get {
			processGetMessage(dbQueries, request, cache)

		} else if request.msgType == ApplicationStateCacheMessage_Create {
			processCreateMessage(dbQueries, request, cache)

		} else if request.msgType == ApplicationStateCacheMessage_Update {
			processUpdateMessage(dbQueries, request, cache)

		} else if request.msgType == ApplicationStateCacheMessage_Delete {
			processDeleteMessage(dbQueries, request, cache)

		} else {
			log.Error(nil, "SEVERE: unimplemented message type")
			continue
		}

	}
}

func processCreateMessage(dbQueries db.DatabaseQueries, req applicationStateCacheRequest, cache map[string]db.ApplicationState) {
	err := dbQueries.CreateApplicationState(req.ctx, &req.createOrUpdateObject)

	if err != nil {
		// Create the cache on success
		var appState db.ApplicationState = req.createOrUpdateObject
		cache[req.primaryKey] = appState
	}

	req.responseChannel <- applicationStateCacheResponse{
		err: err,
	}

}

func processUpdateMessage(dbQueries db.DatabaseQueries, req applicationStateCacheRequest, cache map[string]db.ApplicationState) {

	err := dbQueries.UpdateApplicationState(req.ctx, &req.createOrUpdateObject)

	if err != nil {
		// Update the cache on success
		var appState db.ApplicationState = req.createOrUpdateObject
		cache[req.primaryKey] = appState
	}

	req.responseChannel <- applicationStateCacheResponse{
		err: err,
	}

}

func processDeleteMessage(dbQueries db.DatabaseQueries, req applicationStateCacheRequest, cache map[string]db.ApplicationState) {

	if db.IsEmpty(req.primaryKey) {
		log.Error(fmt.Errorf("PrimaryKey should not be nil"), "SEVERE: "+req.primaryKey+" not found")
	}

	// Remove from cache
	delete(cache, req.primaryKey)

	// Remove from DB
	rowsAffected, err := dbQueries.DeleteApplicationStateById(req.ctx, req.primaryKey)

	req.responseChannel <- applicationStateCacheResponse{
		rowsAffectedForDelete: rowsAffected,
		err:                   err,
	}

}

func processGetMessage(dbQueries db.DatabaseQueries, req applicationStateCacheRequest, cache map[string]db.ApplicationState) error {

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

	req.responseChannel <- applicationStateCacheResponse{
		applicationState: appState,
		valueFromCache:   valueFromCache,
		err:              err,
	}
	return nil
}
