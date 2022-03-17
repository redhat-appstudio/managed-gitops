package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplicationStatesFunctions(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	ctx := context.Background()

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()
	_, managedEnvironment, _, gitopsEngineInstance, _, err := createSampleData(t, dbq)
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
		Health:                          "Healthy",
		Sync_Status:                     "Synced",
	}

	err = dbq.CreateApplicationState(ctx, applicationState)
	assert.NoError(t, err)

	fetchObj := &ApplicationState{
		Applicationstate_application_id: application.Application_id,
	}
	err = dbq.GetApplicationStateById(ctx, fetchObj)
	assert.NoError(t, err)
	assert.ObjectsAreEqualValues(fetchObj, applicationState)
	rowsAffected, err := dbq.DeleteApplicationStateById(ctx, fetchObj.Applicationstate_application_id)
	assert.NoError(t, err)
	assert.True(t, rowsAffected == 1)
	err = dbq.GetApplicationStateById(ctx, fetchObj)
	assert.True(t, IsResultNotFoundError(err))
}
