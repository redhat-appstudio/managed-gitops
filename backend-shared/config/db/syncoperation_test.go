package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateSyncOperation(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)

	ctx := context.Background()
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	_, managedEnvironment, _, gitopsEngineInstance, clusterAccess, err := createSampleData(t, dbq)
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

	err = dbq.CheckedCreateApplication(ctx, application, clusterAccess.Clusteraccess_user_id)

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

	rowCount, err := dbq.DeleteSyncOperationById(ctx, insertRow.SyncOperation_id)
	if !assert.True(t, rowCount == 1) {
		return
	}

	if !assert.NoError(t, err) {
		return
	}
}

func TestGetSyncOperationById(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)

	ctx := context.Background()
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	_, managedEnvironment, _, gitopsEngineInstance, clusterAccess, err := createSampleData(t, dbq)
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

	err = dbq.CheckedCreateApplication(ctx, application, clusterAccess.Clusteraccess_user_id)

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
		SyncOperation_id: insertRow.SyncOperation_id,
	}

	err = dbq.GetSyncOperationById(ctx, &fetchRow)

	if !assert.NoError(t, err) {
		return
	}

	if !assert.Equal(t, fetchRow, insertRow) {
		return
	}

	rowCount, err := dbq.DeleteSyncOperationById(ctx, insertRow.SyncOperation_id)
	if !assert.True(t, rowCount == 1) {
		return
	}

	if !assert.NoError(t, err) {
		return
	}
}

func TestDeleteSyncOperationById(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)

	ctx := context.Background()
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	_, managedEnvironment, _, gitopsEngineInstance, clusterAccess, err := createSampleData(t, dbq)
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

	err = dbq.CheckedCreateApplication(ctx, application, clusterAccess.Clusteraccess_user_id)

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
	rowsAffected, err := dbq.DeleteSyncOperationById(ctx, insertRow.SyncOperation_id)
	if !assert.NoError(t, err) {
		return
	}
	assert.EqualValues(t, 1, rowsAffected)
}

func TestUpdateSyncOperationRemoveApplicationField(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)

	ctx := context.Background()
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	_, managedEnvironment, _, gitopsEngineInstance, clusterAccess, err := createSampleData(t, dbq)
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

	err = dbq.CheckedCreateApplication(ctx, application, clusterAccess.Clusteraccess_user_id)

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

	rowsAffected, err := dbq.UpdateSyncOperationRemoveApplicationField(ctx, application.Application_id)
	if !assert.NoError(t, err) {
		return
	}
	assert.EqualValues(t, 1, rowsAffected)
	rowsAffected, err = dbq.DeleteSyncOperationById(ctx, insertRow.SyncOperation_id)
	if !assert.NoError(t, err) {
		return
	}
	assert.EqualValues(t, 1, rowsAffected)
}
