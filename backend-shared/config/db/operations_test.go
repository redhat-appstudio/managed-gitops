package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var timestamp = time.Date(2022, time.March, 11, 12, 3, 49, 514935000, time.UTC)
var DefaultValue = 101

func TestGetOperationById(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	_, _, _, gitopsEngineInstance, _, err := createSampleData(t, dbq)
	if !assert.NoError(t, err) {
		return
	}
	operationput := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.CreateOperation(ctx, &operationput, operationput.Operation_owner_user_id)
	if !assert.NoError(t, err) {
		return
	}

	operationget := Operation{
		Operation_id: operationput.Operation_id,
	}

	err = dbq.GetOperationById(ctx, &operationget)
	if !assert.NoError(t, err) {
		return
	}

	assert.IsType(t, timestamp, operationget.Created_on)
	assert.IsType(t, timestamp, operationget.Last_state_update)
	operationget.Created_on = operationput.Created_on
	operationget.Last_state_update = operationput.Last_state_update
	assert.Equal(t, operationput, operationget)

	operationNotExist := Operation{
		Operation_id: "test-operation-1-not-exist",
	}

	err = dbq.GetOperationById(ctx, &operationNotExist)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

}

func TestCreateOperation(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	_, _, _, gitopsEngineInstance, _, err := createSampleData(t, dbq)
	if !assert.NoError(t, err) {
		return
	}
	operationput := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.CreateOperation(ctx, &operationput, operationput.Operation_owner_user_id)
	if !assert.NoError(t, err) {
		return
	}

	operationget := Operation{
		Operation_id: operationput.Operation_id,
	}

	err = dbq.GetOperationById(ctx, &operationget)
	if !assert.NoError(t, err) {
		return
	}

	assert.IsType(t, timestamp, operationget.Created_on)
	assert.IsType(t, timestamp, operationget.Last_state_update)
	operationget.Created_on = operationput.Created_on
	operationget.Last_state_update = operationput.Last_state_update
	assert.Equal(t, operationput, operationget)

}

func TestDeleteOperationById(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()
	_, _, _, gitopsEngineInstance, _, err := createSampleData(t, dbq)
	if !assert.NoError(t, err) {
		return
	}
	operation := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.CreateOperation(ctx, &operation, operation.Operation_owner_user_id)
	if !assert.NoError(t, err) {
		return
	}

	rowsAffected, err := dbq.DeleteOperationById(ctx, operation.Operation_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	err = dbq.GetOperationById(ctx, &operation)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

}

func TestListOperationsByResourceIdAndTypeAndOwnerId(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	_, _, _, gitopsEngineInstance, _, err := createSampleData(t, dbq)
	if !assert.NoError(t, err) {
		return
	}

	operation := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	var operations []Operation

	err = dbq.CreateOperation(ctx, &operation, operation.Operation_owner_user_id)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, operation.Resource_id, operation.Resource_type, &operations, operation.Operation_owner_user_id)
	assert.NoError(t, err)

	assert.IsType(t, timestamp, operation.Created_on)
	assert.IsType(t, timestamp, operation.Last_state_update)
	operation.Created_on = operations[0].Created_on
	operation.Last_state_update = operations[0].Last_state_update

	assert.Equal(t, operations[0].Last_state_update, operation.Last_state_update)
	assert.Equal(t, len(operations), 1)
}

func TestUpdateOperation(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	_, _, _, gitopsEngineInstance, _, err := createSampleData(t, dbq)
	if !assert.NoError(t, err) {
		return
	}
	operationput := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.CreateOperation(ctx, &operationput, operationput.Operation_owner_user_id)
	if !assert.NoError(t, err) {
		return
	}

	operationget := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id-update",
		Resource_type:           "GitopsEngineInstance-update",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
		SeqID:                   int64(DefaultValue),
	}

	assert.IsType(t, timestamp, operationget.Created_on)
	assert.IsType(t, timestamp, operationget.Last_state_update)
	operationget.Created_on = operationput.Created_on
	operationget.Last_state_update = operationput.Last_state_update

	err = dbq.UpdateOperation(ctx, &operationget)
	if !assert.NoError(t, err) {
		return
	}
	err = dbq.GetOperationById(ctx, &operationget)
	if !assert.NoError(t, err) {
		return
	}

	if !assert.NotEqual(t, operationput, operationget) {
		return
	}

	// Set the invalid value
	operationput.Operation_id = strings.Repeat("abc", 100)
	err = dbq.CreateOperation(ctx, &operationput, operationput.Operation_owner_user_id)
	assert.True(t, isMaxLengthError(err))

	operationget.Operation_owner_user_id = strings.Repeat("abc", 100)
	err = dbq.UpdateOperation(ctx, &operationget)
	assert.True(t, isMaxLengthError(err))
}
