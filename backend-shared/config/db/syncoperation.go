package db

import (
	"context"
	"fmt"
)

const (
	SyncOperation_DesiredState_Running    = "Running"
	SyncOperation_DesiredState_Terminated = "Terminated"
)

func (dbq *PostgreSQLDatabaseQueries) GetSyncOperationById(ctx context.Context, syncOperation *SyncOperation) error {

	if err := validateQueryParamsEntity(syncOperation, dbq); err != nil {
		return err
	}

	if IsEmpty(syncOperation.SyncOperation_id) {
		return fmt.Errorf("sync operation id is empty")
	}

	var dbResults []SyncOperation

	if err := dbq.dbConnection.Model(&dbResults).
		Where("so.syncoperation_id = ?", syncOperation.SyncOperation_id).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving GetSyncOperationById: %v", err)
	}

	if len(dbResults) >= 2 {
		return fmt.Errorf("multiple results returned from GetSyncOperationById")
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("no results found for GetSyncOperationById")
	}

	*syncOperation = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateSyncOperation(ctx context.Context, obj *SyncOperation) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if IsEmpty(obj.SyncOperation_id) {
			obj.SyncOperation_id = generateUuid()
		}
	} else {
		if !IsEmpty(obj.SyncOperation_id) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.SyncOperation_id = generateUuid()
	}

	if err := isEmptyValues("CreateSyncOperation",
		"Application_id", obj.Application_id,
		"DeploymentNameField", obj.DeploymentNameField,
		"Revision", obj.Revision,
		"DesiredState", obj.DesiredState); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
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

func (dbq *PostgreSQLDatabaseQueries) DeleteSyncOperationById(ctx context.Context, id string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	if IsEmpty(id) {
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

func (dbq *PostgreSQLDatabaseQueries) UpdateSyncOperation(ctx context.Context, obj *SyncOperation) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UpdateSyncOperation",
		"syncoperation_id", obj.SyncOperation_id,
		"application_id", obj.Application_id,
		"deployment_name", obj.DeploymentNameField,
		"revision", obj.Revision,
		"desired_state", obj.DesiredState,
	); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).WherePK().Context(ctx).Update()
	if err != nil {
		return fmt.Errorf("error on updating SyncOperation: %v, %v", err, obj.SyncOperation_id)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d, %v", result.RowsAffected(), obj.SyncOperation_id)
	}

	return nil
}

// UpdateSyncOperationRemoveApplicationField locates any SyncOperations that reference 'applicationID', and sets the
// applicationID field to nil.
func (dbq *PostgreSQLDatabaseQueries) UpdateSyncOperationRemoveApplicationField(ctx context.Context, applicationId string) (int, error) {

	if err := validateQueryParamsNoPK(dbq); err != nil {
		return 0, err
	}

	if err := isEmptyValues("UpdateOperationRemoveApplicationField",
		"applicationId", applicationId); err != nil {
		return 0, err
	}

	operation := SyncOperation{
		Application_id: applicationId,
	}

	res, err := dbq.dbConnection.Model(&operation).Set("application_id = ?", nil).Where("application_id = ?", applicationId).Update()

	if err != nil {
		return 0, err
	}

	return res.RowsAffected(), err
}

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllSyncOperations(ctx context.Context, syncOperations *[]SyncOperation) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}
	if err := dbq.dbConnection.Model(syncOperations).Context(ctx).Select(); err != nil {
		return err
	}
	return nil
}

var _ AppScopedDisposableResource = &SyncOperation{}

func (obj *SyncOperation) DisposeAppScoped(ctx context.Context, dbq ApplicationScopedQueries) error {
	if dbq == nil {
		return fmt.Errorf("missing database interface in syncoperation dispose")
	}

	_, err := dbq.DeleteSyncOperationById(ctx, obj.SyncOperation_id)
	return err
}
