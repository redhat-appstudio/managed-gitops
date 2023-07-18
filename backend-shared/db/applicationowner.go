package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllApplicationOwners(ctx context.Context, obj *[]ApplicationOwner) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(obj).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateApplicationOwner(ctx context.Context, obj *ApplicationOwner) error {

	if IsEmpty(obj.ApplicationOwnerApplicationID) {
		return fmt.Errorf("primary key applicationowner_application_id id should not be empty")
	}

	if IsEmpty(obj.ApplicationOwnerUserID) {
		return fmt.Errorf("primary key applicationowner_user_id should not be empty")
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting applicationOwner: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteApplicationOwner(ctx context.Context, applicationowner_application_id string, applicationowner_user_id string) (int, error) {

	if IsEmpty(applicationowner_application_id) {
		return 0, fmt.Errorf("primary key applicationowner_application_id id should not be empty")
	}

	if IsEmpty(applicationowner_user_id) {
		return 0, fmt.Errorf("primary key applicationowner_user_id should not be empty")
	}

	result := &ApplicationOwner{}

	deleteResult, err := dbq.dbConnection.Model(result).
		Where("application_owner_application_id = ?", applicationowner_application_id).
		Where("application_owner_user_id = ?", applicationowner_user_id).
		Context(ctx).
		Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) GetApplicationOwnerByPrimaryKey(ctx context.Context, obj *ApplicationOwner) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("GetApplicationOwnerByPrimaryKey",
		"application_owner_application_id = ?", obj.ApplicationOwnerApplicationID,
		"application_owner_user_id = ?", obj.ApplicationOwnerUserID); err != nil {
		return err
	}

	var dbResults []ApplicationOwner

	err := dbq.dbConnection.Model(&dbResults).
		Where("application_owner_application_id = ?", obj.ApplicationOwnerApplicationID).
		Where("application_owner_user_id = ?", obj.ApplicationOwnerUserID).
		Context(ctx).Select()

	if err != nil {
		return fmt.Errorf("unable to retrieve ApplicationOwner in GetApplicationOwnerByPrimaryKey: %v", err)
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("No results for ApplicationOwner")
	}

	if len(dbResults) != 1 {
		return fmt.Errorf("unexpected number of results for GetApplicationOwnerByPrimaryKey")
	}

	*obj = dbResults[0]

	return nil
}

// GetAsLogKeyValues returns an []interface that can be passed to log.Info(...).
// e.g. log.Info("Creating database resource", obj.GetAsLogKeyValues()...)
func (obj *ApplicationOwner) GetAsLogKeyValues() []interface{} {
	if obj == nil {
		return []interface{}{}
	}

	return []interface{}{"application_owner_application_id = ?", obj.ApplicationOwnerApplicationID,
		"application_owner_user_id = ?", obj.ApplicationOwnerUserID}

}
