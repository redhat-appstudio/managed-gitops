package db

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplicationStatesFunctions(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	ctx := context.Background()

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()
	_, managedEnvironment, _, gitopsEngineInstance, _, err := CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}

	application := &Application{
		Application_id:          "test-my-application",
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	err = dbq.CreateApplication(ctx, application)

	if !assert.NoError(t, err) {
		return
	}

	applicationState := &ApplicationState{
		Applicationstate_application_id: application.Application_id,
		Health:                          "Progressing",
		Sync_Status:                     "Unknown",
	}

	err = dbq.CreateApplicationState(ctx, applicationState)
	assert.NoError(t, err)

	fetchObj := &ApplicationState{
		Applicationstate_application_id: application.Application_id,
	}
	err = dbq.GetApplicationStateById(ctx, fetchObj)
	assert.NoError(t, err)
	assert.ObjectsAreEqualValues(fetchObj, applicationState)

	applicationState.Health = "Healthy"
	applicationState.Sync_Status = "Synced"
	err = dbq.UpdateApplicationState(ctx, applicationState)
	assert.NoError(t, err)

	err = dbq.GetApplicationStateById(ctx, fetchObj)
	assert.NoError(t, err)
	assert.ObjectsAreEqualValues(fetchObj, applicationState)

	rowsAffected, err := dbq.DeleteApplicationStateById(ctx, fetchObj.Applicationstate_application_id)
	assert.NoError(t, err)
	assert.True(t, rowsAffected == 1)
	err = dbq.GetApplicationStateById(ctx, fetchObj)
	assert.True(t, IsResultNotFoundError(err))

	// Set the invalid value
	applicationState.Sync_Status = strings.Repeat("abc", 100)
	err = dbq.CreateApplicationState(ctx, applicationState)
	assert.True(t, isMaxLengthError(err))

	err = dbq.UpdateApplicationState(ctx, applicationState)
	assert.True(t, isMaxLengthError(err))

}
