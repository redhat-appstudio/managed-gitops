package db

import (
	"context"
	"fmt"
	"time"
)

// Unsafe: Should only be used in test code.
func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllOperations(ctx context.Context, operations *[]Operation) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(operations).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateOperation(ctx context.Context, obj *Operation, ownerId string) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
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
	gei := GitopsEngineInstance{Gitopsengineinstance_id: obj.Instance_id}
	if err := dbq.UncheckedGetGitopsEngineInstanceById(ctx, &gei); err != nil {
		return fmt.Errorf("unable to retrieve operation's gitops engine instance ID: '%v' %v", obj.Instance_id, err)
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

func (dbq *PostgreSQLDatabaseQueries) UncheckedGetOperationById(ctx context.Context, operation *Operation) error {

	if err := validateQueryParamsEntity(operation, dbq); err != nil {
		return err
	}

	if isEmpty(operation.Operation_id) {
		return fmt.Errorf("invalid pk")
	}

	var dbResult []Operation

	if err := dbq.dbConnection.Model(&dbResult).
		Where("operation_id = ?", operation.Operation_id).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving operation: %v", err)
	}

	if len(dbResult) == 0 {
		return NewResultNotFoundError(fmt.Sprintf("unable to locate operation '%v'", operation.Operation_id))
	}

	if len(dbResult) > 1 {
		return fmt.Errorf("unexpected number of results in GetOperationById")
	}

	*operation = dbResult[0]

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) GetOperationById(ctx context.Context, operation *Operation, ownerId string) error {

	if err := validateQueryParamsEntity(operation, dbq); err != nil {
		return err
	}

	if isEmpty(operation.Operation_id) {
		return fmt.Errorf("invalid pk")
	}

	if isEmpty(ownerId) {
		return fmt.Errorf("owner id is empty")
	}

	var dbResult []Operation

	if err := dbq.dbConnection.Model(&dbResult).
		Where("operation_id = ?", operation.Operation_id).
		Where("operation_owner_user_id = ?", ownerId).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving operation %v", err)
	}

	if len(dbResult) == 0 {
		return NewResultNotFoundError(fmt.Sprintf("unable to locate operation '%v'", operation.Operation_id))
	}

	if len(dbResult) > 1 {
		return fmt.Errorf("unexpected number of results in GetOperationById")
	}

	*operation = dbResult[0]

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) UncheckedDeleteOperationById(ctx context.Context, id string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	result := &Operation{
		Operation_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().
		Context(ctx).
		Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteOperationById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
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
