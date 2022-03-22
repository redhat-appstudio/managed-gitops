package util

import (
	"context"
	"testing"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/stretchr/testify/assert"
)

// TestCreateApplicationState test is used for creating of ApplicationState using cache
func TestCreateApplicationState(t *testing.T) {
	db.SetupforTestingDB(t)
	defer db.TestTeardown(t)

	asc := NewApplicationInfoCache()

	ctx := context.Background()
	dbq, err := db.NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}

	application := &db.Application{
		Application_id:          "test-my-application",
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	// An entry for the application should be there in order to create an entry for applicationState
	err = dbq.CreateApplication(ctx, application)
	if !assert.NoError(t, err) {
		return
	}

	testAppState := db.ApplicationState{
		Applicationstate_application_id: application.Application_id,
		Health:                          "Healthy",
		Sync_Status:                     "Synced",
	}
	errCreate := asc.CreateApplicationState(ctx, testAppState)
	assert.NoError(t, errCreate)

	dbAppStateObj := &db.ApplicationState{
		Applicationstate_application_id: testAppState.Applicationstate_application_id,
	}
	errGet := dbq.GetApplicationStateById(ctx, dbAppStateObj)
	assert.NoError(t, errGet)
}

// TestGetApplicationStateById test is used for retrieving of ApplicationState using cache
func TestGetApplicationStateById(t *testing.T) {
	db.SetupforTestingDB(t)
	defer db.TestTeardown(t)

	asc := NewApplicationInfoCache()

	ctx := context.Background()
	dbq, err := db.NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	// check if the application state entry is in the cache?
	testId := "test-get-appState"

	//testId doesn't exist, the test should report an error
	appState, valuefromCache, errGet := asc.GetApplicationStateById(ctx, testId)
	assert.Error(t, errGet)
	// since no entry in the valuefromcache should be false, appState should be empty
	assert.False(t, valuefromCache)
	assert.Equal(t, appState, db.ApplicationState{})

	_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}

	application := &db.Application{
		Application_id:          testId,
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	// An entry for the application should be there in order to create an entry for applicationState
	err = dbq.CreateApplication(ctx, application)
	if !assert.NoError(t, err) {
		return
	}

	// create an applicationstate then try to get it
	testAppState := db.ApplicationState{
		Applicationstate_application_id: testId,
		Health:                          "Healthy",
		Sync_Status:                     "Synced",
	}
	errCreate := asc.CreateApplicationState(ctx, testAppState)
	assert.NoError(t, errCreate)

	_, _, errGet = asc.GetApplicationStateById(ctx, testAppState.Applicationstate_application_id)
	// ideally the appState should now report an ApplicationState obj
	assert.NoError(t, errGet)

	// checking if the entry for the above exists in the database
	errGet = dbq.GetApplicationStateById(ctx, &testAppState)
	assert.NoError(t, errGet)
}

// TestUpdateApplicationState test is used for updating of ApplicationState using cache
func TestUpdateApplicationState(t *testing.T) {

	db.SetupforTestingDB(t)
	defer db.TestTeardown(t)

	asc := NewApplicationInfoCache()

	ctx := context.Background()
	dbq, err := db.NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}

	application := &db.Application{
		Application_id:          "test-update-cache-1",
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	// An entry for the application should be there in order to create an entry for applicationState
	err = dbq.CreateApplication(ctx, application)
	if !assert.NoError(t, err) {
		return
	}

	testAppState := db.ApplicationState{
		Applicationstate_application_id: application.Application_id,
		Health:                          "Healthy",
		Sync_Status:                     "Synced",
	}
	errCreate := asc.CreateApplicationState(ctx, testAppState)
	assert.NoError(t, errCreate)

	// updating the health status to Unhealthy and updating
	testAppState.Health = "Unhealthy"
	errUpdate := asc.UpdateApplicationState(ctx, testAppState)
	assert.NoError(t, errUpdate)

	appState, _, errGet := asc.GetApplicationStateById(ctx, testAppState.Applicationstate_application_id)
	assert.NoError(t, errGet)
	// the application should be retrieved from the cache hence valuefromcache should be true (is it ideal scenario?)
	// assert.True(t, valuefromCache)

	// check if the object is updated
	assert.Equal(t, testAppState, appState)
}

// TestUpdateApplicationState test is used for updating of ApplicationState using cache
func TestDeleteApplicationState(t *testing.T) {

	db.SetupforTestingDB(t)
	defer db.TestTeardown(t)

	asc := NewApplicationInfoCache()

	ctx := context.Background()
	dbq, err := db.NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}

	application := &db.Application{
		Application_id:          "test-delete-cache-1",
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	// An entry for the application should be there in order to create an entry for applicationState
	err = dbq.CreateApplication(ctx, application)
	if !assert.NoError(t, err) {
		return
	}

	testAppState := db.ApplicationState{
		Applicationstate_application_id: application.Application_id,
		Health:                          "Healthy",
		Sync_Status:                     "Synced",
	}
	errCreate := asc.CreateApplicationState(ctx, testAppState)
	assert.NoError(t, errCreate)

	testDeleteAppState := db.ApplicationState{
		Applicationstate_application_id: testAppState.Applicationstate_application_id,
	}

	rowsAffected, errCreate := asc.DeleteApplicationStateById(ctx, testDeleteAppState.Applicationstate_application_id)
	assert.NoError(t, errCreate)
	assert.GreaterOrEqual(t, rowsAffected, 1)

	// check for entry in db, which should report an error
	errGet := dbq.GetApplicationStateById(ctx, &testDeleteAppState)
	assert.Error(t, errGet)
}

// TestGetApplicationById test is used for retrieving of Application using cache
func TestGetApplicationById(t *testing.T) {
	db.SetupforTestingDB(t)
	defer db.TestTeardown(t)

	asc := NewApplicationInfoCache()

	ctx := context.Background()
	dbq, err := db.NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	// check if the application state entry is in the cache?
	testId := "test-get-appState"

	//testId doesn't exist, the test should report an error
	app, valuefromCache, errGet := asc.GetApplicationById(ctx, testId)
	assert.Error(t, errGet)
	// since no entry in the valuefromcache should be false, appState should be empty
	assert.False(t, valuefromCache)
	assert.Equal(t, app, db.Application{})

	_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}

	testapplication := &db.Application{
		Application_id:          testId,
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	// An entry for the application should be there in order to create an entry for applicationState
	err = dbq.CreateApplication(ctx, testapplication)
	if !assert.NoError(t, err) {
		return
	}

	_, _, errGet = asc.GetApplicationById(ctx, testapplication.Application_id)
	// ideally the appState should now report an ApplicationState obj
	assert.NoError(t, errGet)

	// checking if the entry for the above exists in the database
	errGet = dbq.GetApplicationById(ctx, testapplication)
	assert.NoError(t, errGet)
}
