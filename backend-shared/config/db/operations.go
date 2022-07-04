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

		if IsEmpty(obj.Operation_id) {
			obj.Operation_id = generateUuid()
		}

	} else {

		if !IsEmpty(obj.Operation_id) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.Operation_id = generateUuid()
	}

	// State

	if err := isEmptyValues("CreateOperation",
		"Instance_id", obj.Instance_id,
		"Operation_id", obj.Operation_id,
		"Operation_owner_user_id", obj.Operation_owner_user_id,
		"Resource_id", obj.Resource_id,
		"Resource_type", obj.Resource_type,
		"State", obj.State); err != nil {
		return err
	}

	// Verify the instance exists
	gei := GitopsEngineInstance{Gitopsengineinstance_id: obj.Instance_id}
	if err := dbq.GetGitopsEngineInstanceById(ctx, &gei); err != nil {
		return fmt.Errorf("unable to retrieve operation's gitops engine instance ID: '%v' %v", obj.Instance_id, err)
	}

	obj.Created_on = time.Now()
	obj.Last_state_update = obj.Created_on

	// Initial state is waiting
	obj.State = OperationState_Waiting

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting operation: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UpdateOperation(ctx context.Context, obj *Operation) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UpdateOperation",
		"Instance_id", obj.Instance_id,
		"Operation_id", obj.Operation_id,
		"Operation_owner_user_id", obj.Operation_owner_user_id,
		"Resource_id", obj.Resource_id,
		"Resource_type", obj.Resource_type,
		"State", obj.State); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).WherePK().Context(ctx).Update()
	if err != nil {
		return fmt.Errorf("error on updating operation: %v, %v", err, obj.Operation_id)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d, %v", result.RowsAffected(), obj.Operation_id)
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) GetOperationById(ctx context.Context, operation *Operation) error {

	if err := validateQueryParamsEntity(operation, dbq); err != nil {
		return err
	}

	if IsEmpty(operation.Operation_id) {
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

func (dbq *PostgreSQLDatabaseQueries) CheckedGetOperationById(ctx context.Context, operation *Operation, ownerId string) error {

	if err := validateQueryParamsEntity(operation, dbq); err != nil {
		return err
	}

	if IsEmpty(operation.Operation_id) {
		return fmt.Errorf("invalid pk")
	}

	if IsEmpty(ownerId) {
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

func (dbq *PostgreSQLDatabaseQueries) DeleteOperationById(ctx context.Context, id string) (int, error) {

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

func (dbq *PostgreSQLDatabaseQueries) CheckedDeleteOperationById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	if IsEmpty(ownerId) {
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

func (dbq *PostgreSQLDatabaseQueries) ListOperationsByResourceIdAndTypeAndOwnerId(ctx context.Context, resourceID string, resourceType string, operations *[]Operation, ownerId string) error {

	if err := validateQueryParamsEntity(operations, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("ListOperationsByResourceIdAndTypeAndOwnerId",
		"ownerId", ownerId,
		"resourceId", resourceID,
		"resourceType", resourceType); err != nil {
		return err
	}

	var dbResults []Operation

	// TODO: GITOPSRVCE-68 - PERF - Add index for this

	if err := dbq.dbConnection.Model(&dbResults).
		Where("op.resource_id = ?", resourceID).
		Where("op.resource_type = ?", resourceType).
		Where("op.operation_owner_user_id = ?", ownerId).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving ListOperationsByResourceIdAndTypeAndOwnerId: %v", err)
	}

	*operations = dbResults

	return nil
}

func (operation *Operation) DisposeAppScoped(ctx context.Context, dbq ApplicationScopedQueries) error {

	if err := isEmptyValues("DisposeAppScoped-Operation", "dbq", dbq); err != nil {
		return err
	}
	_, err := dbq.DeleteOperationById(ctx, operation.Operation_id)

	return err
func (dbq *PostgreSQLDatabaseQueries) ListOperationsToBeGarbageCollected(operations *[]Operation) error {

	return nil
}
