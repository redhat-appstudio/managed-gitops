package db

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetApplicationById(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	_, managedEnvironment, _, gitopsEngineInstance, _, err := CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}
	applicationput := Application{
		Application_id:          "test-my-application-1",
		Name:                    "test-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	err = dbq.CreateApplication(ctx, &applicationput)
	if !assert.NoError(t, err) {
		return
	}

	applicationget := Application{
		Application_id: applicationput.Application_id,
	}

	err = dbq.GetApplicationById(ctx, &applicationget)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, applicationput, applicationget)

	// check for non existent primary key

	applicationNotExist := Application{
		Application_id: "test-my-application-1-not-exist",
	}

	err = dbq.GetApplicationById(ctx, &applicationNotExist)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

}

func TestCreateApplications(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	_, managedEnvironment, _, gitopsEngineInstance, _, err := CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}
	applicationput := Application{
		Application_id:          "test-my-application-1",
		Name:                    "test-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	err = dbq.CreateApplication(ctx, &applicationput)
	if !assert.NoError(t, err) {
		return
	}

	applicationget := Application{
		Application_id: applicationput.Application_id,
	}

	err = dbq.GetApplicationById(ctx, &applicationget)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, applicationput, applicationget)

	// Set the invalid value
	applicationput.Name = strings.Repeat("abc", 100)
	err = dbq.CreateApplication(ctx, &applicationput)
	assert.True(t, isMaxLengthError(err))
}

func TestDeleteApplicationById(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	_, managedEnvironment, _, gitopsEngineInstance, _, err := CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}
	application := &Application{
		Application_id:          "test-my-application-1",
		Name:                    "test-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	err = dbq.CreateApplication(ctx, application)
	if !assert.NoError(t, err) {
		return
	}

	rowsAffected, err := dbq.DeleteApplicationById(ctx, application.Application_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	err = dbq.GetApplicationById(ctx, application)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

}

func TestUpdateApplication(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	_, managedEnvironment, _, gitopsEngineInstance, _, err := CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}
	applicationput := &Application{
		Application_id:          "test-my-application-1",
		Name:                    "test-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	err = dbq.CreateApplication(ctx, applicationput)
	if !assert.NoError(t, err) {
		return
	}

	applicationget := Application{

		Application_id:          applicationput.Application_id,
		Name:                    "test-application-update",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
		SeqID:                   int64(DefaultValue),
	}

	err = dbq.UpdateApplication(ctx, &applicationget)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.GetApplicationById(ctx, &applicationget)
	if !assert.NoError(t, err) {
		return
	}

	if !assert.NotEqual(t, applicationput, applicationget) {
		return
	}

	// Set the invalid value
	applicationget.Name = strings.Repeat("abc", 100)
	err = dbq.UpdateApplication(ctx, &applicationget)
	assert.True(t, isMaxLengthError(err))
}
