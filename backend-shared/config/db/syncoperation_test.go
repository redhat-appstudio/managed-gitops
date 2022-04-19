package db

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateandGetSyncOperation(t *testing.T) {
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

	operation := &Operation{
		Operation_id:            "test-operation",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "fake resource id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.CreateOperation(ctx, operation, operation.Operation_owner_user_id)

	if !assert.NoError(t, err) {
		return
	}

	insertRow := SyncOperation{
		SyncOperation_id:    "test-sync",
		Application_id:      application.Application_id,
		Operation_id:        operation.Operation_id,
		DeploymentNameField: "testDeployment",
		Revision:            "testRev",
		DesiredState:        "Terminated",
	}

	err = dbq.CreateSyncOperation(ctx, &insertRow)

	if !assert.NoError(t, err) {
		return
	}
	fetchRow := SyncOperation{
		SyncOperation_id: "test-sync",
	}
	err = dbq.GetSyncOperationById(ctx, &fetchRow)
	if !assert.NoError(t, err) && !assert.ObjectsAreEqualValues(fetchRow, insertRow) {
		assert.Fail(t, "Values between Fetched Row and Inserted Row don't match.")
	}

	rowCount, err := dbq.DeleteSyncOperationById(ctx, insertRow.SyncOperation_id)
	assert.NoError(t, err)
	assert.True(t, rowCount == 1)
	fetchRow = SyncOperation{
		SyncOperation_id: "test-sync",
	}

	err = dbq.GetSyncOperationById(ctx, &fetchRow)
	assert.True(t, IsResultNotFoundError(err))

	// Set the invalid value
	insertRow.DeploymentNameField = strings.Repeat("abc", 100)
	err = dbq.CreateSyncOperation(ctx, &insertRow)
	assert.True(t, isMaxLengthError(err))
}
