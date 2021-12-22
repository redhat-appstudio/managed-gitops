package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UncheckedGetSyncOperationById(ctx context.Context, syncOperation *SyncOperation) error {

	if err := validateQueryParamsEntity(syncOperation, dbq); err != nil {
		return err
	}

	if isEmpty(syncOperation.SyncOperation_id) {
		return fmt.Errorf("sync operation id is empty")
	}

	var dbResults []SyncOperation

	if err := dbq.dbConnection.Model(&dbResults).
		Where("so.syncoperation_id = ?", syncOperation.SyncOperation_id).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving UncheckedGetSyncOperationById: %v", err)
	}

	if len(dbResults) >= 2 {
		return fmt.Errorf("multiple results returned from UncheckedGetSyncOperationById")
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("no results found for UncheckedGetSyncOperationById")
	}

	*syncOperation = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UncheckedCreateSyncOperation(ctx context.Context, obj *SyncOperation) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if isEmpty(obj.SyncOperation_id) {
			obj.SyncOperation_id = generateUuid()
		}
	} else {
		if !isEmpty(obj.SyncOperation_id) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.SyncOperation_id = generateUuid()
	}

	if err := isEmptyValues("UncheckedCreateSyncOperation",
		"Application_id", obj.Application_id,
		"DeploymentNameField", obj.DeploymentNameField,
		"Revision", obj.Revision); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting application: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) UncheckedDeleteSyncOperationById(ctx context.Context, id string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("sync operation id was empty in delete")
	}

	result := &SyncOperation{}

	deleteResult, err := dbq.dbConnection.Model(result).
		Where("so.syncoperation_id = ?", id).
		Context(ctx).
		Delete()

	if err != nil {
		return 0, fmt.Errorf("error on deleting syncoperation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

var _ AppScopedDisposableResource = &SyncOperation{}

func (obj *SyncOperation) DisposeAppScoped(ctx context.Context, dbq ApplicationScopedQueries) error {
	if dbq == nil {
		return fmt.Errorf("missing database interface in syncoperation dispose")
	}

	_, err := dbq.UncheckedDeleteSyncOperationById(ctx, obj.SyncOperation_id)
	return err
}
