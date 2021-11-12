package db

import (
	"context"
	"fmt"
	"time"
)

// Unsafe: Should only be used in test code.
func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllOperations(ctx context.Context) ([]Operation, error) {
	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}
	if !dbq.allowUnsafe {
		return nil, fmt.Errorf("unsafe call to ListAllOperations")
	}

	var operations []Operation
	err := dbq.dbConnection.Model(&operations).Context(ctx).Select()

	if err != nil {
		return nil, err
	}

	return operations, nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateOperation(ctx context.Context, obj *Operation, ownerId string) error {

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if dbq.allowTestUuids {

		if isEmpty(obj.Operation_id) {
			obj.Operation_id = generateUuid()
		}

	} else {

		if !isEmpty(obj.Operation_id) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.Operation_id = generateUuid()
	}

	// State

	if isEmpty(obj.Operation_owner_user_id) {
		return fmt.Errorf("operation 'operation_owner_user_id' field should not be empty")
	}

	if isEmpty(obj.Instance_id) {
		return fmt.Errorf("operation 'Instance_id' field should not be empty")
	}

	if isEmpty(obj.Resource_id) {
		return fmt.Errorf("operation 'Resource_id' field should not be empty")
	}

	if isEmpty(obj.Resource_type) {
		return fmt.Errorf("operation'Resource_type' field should not be empty")
	}

	if ownerId != obj.Operation_owner_user_id {
		return fmt.Errorf("operation owner user must match the user issuing the create request")
	}

	// Verify the instance exists, and the user has access to it
	gei, err := dbq.GetGitopsEngineInstanceById(ctx, obj.Instance_id, ownerId)
	if err != nil || gei == nil {
		return fmt.Errorf("unable to retrieve operation's instance ID")
	}

	obj.Created_on = time.Now()
	obj.Last_state_update = obj.Created_on

	// Initial state is waiting
	obj.State = OperationState_Waiting

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting operation: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetOperationById(ctx context.Context, id string, ownerId string) (*Operation, error) {

	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return nil, fmt.Errorf("invalid pk")
	}

	if isEmpty(ownerId) {
		return nil, fmt.Errorf("owner id is empty")
	}

	result := &Operation{
		Operation_id: id,
	}

	if err := dbq.dbConnection.Model(result).
		Where("operation_id = ?", id).
		Where("operation_owner_user_id = ?", ownerId).
		Context(ctx).
		Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving operation %v", err)
	}

	return result, nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteOperationById(ctx context.Context, id string, ownerId string) (int, error) {
	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
	}

	if isEmpty(ownerId) {
		return 0, fmt.Errorf("owner id is empty")
	}

	result := &Operation{
		Operation_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().
		Where("operation_owner_user_id = ?", ownerId).
		Context(ctx).
		Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
