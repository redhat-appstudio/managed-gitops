package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) CreateManagedEnvironment(ctx context.Context, obj *ManagedEnvironment) error {

	if err := validateQueryParams(obj.Clustercredentials_id, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if isEmpty(obj.Managedenvironment_id) {
			obj.Managedenvironment_id = generateUuid()
		}
	} else {
		if !isEmpty(obj.Managedenvironment_id) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.Managedenvironment_id = generateUuid()
	}

	if isEmpty(obj.Name) {
		return fmt.Errorf("managed environment name field should not be empty")
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting managed environment: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllManagedEnvironments(ctx context.Context) ([]ManagedEnvironment, error) {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return nil, err
	}

	var managedEnvironments []ManagedEnvironment
	if err := dbq.dbConnection.Model(&managedEnvironments).Context(ctx).Select(); err != nil {
		return nil, err
	}

	return managedEnvironments, nil
}

func (dbq *PostgreSQLDatabaseQueries) GetManagedEnvironmentByClusterCredentials(ctx context.Context, clusterCredentialId string, ownerId string) ([]ManagedEnvironment, error) {

	if err := validateQueryParams(clusterCredentialId, dbq); err != nil {
		return nil, err
	}

	var result []ManagedEnvironment

	if err := dbq.dbConnection.Model(&result).
		Where("me.clustercredentials_id = ?", clusterCredentialId).
		Where("ca.clusteraccess_user_id = ?", ownerId).
		Join("JOIN clusteraccess AS ca ON ca.clusteraccess_managed_environment_id = me.managedenvironment_id").
		Context(ctx).
		Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving ManagedEnvironment: %v", err)
	}

	if len(result) == 0 {
		return nil, NewResultNotFoundError("error on retrieving GetManagedEnvironmentByClusterCredentials")
	}

	return result, nil

}

func (dbq *PostgreSQLDatabaseQueries) GetManagedEnvironmentById(ctx context.Context, managedEnvid string, ownerId string) (*ManagedEnvironment, error) {

	if err := validateQueryParams(managedEnvid, dbq); err != nil {
		return nil, err
	}
	var result []ManagedEnvironment

	if err := dbq.dbConnection.Model(&result).
		Where("me.managedenvironment_id = ?", managedEnvid).
		Where("ca.clusteraccess_user_id = ?", ownerId).
		Join("JOIN clusteraccess AS ca ON ca.clusteraccess_managed_environment_id = me.managedenvironment_id").
		Context(ctx).
		Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving ManagedEnvironment: %v", err)
	}

	if len(result) >= 2 {
		return nil, fmt.Errorf("multiple results returned from GetManagedEnvironmentById")
	}

	if len(result) == 0 {
		return nil, NewResultNotFoundError("error on retrieving GetGitopsEngineInstanceById")
	}

	return &result[0], nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteManagedEnvironmentById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	if isEmpty(ownerId) {
		return 0, fmt.Errorf("owner id is empty")
	}

	existingValue, err := dbq.GetManagedEnvironmentById(ctx, id, ownerId)
	if err != nil || existingValue == nil || existingValue.Managedenvironment_id != id {
		return 0, fmt.Errorf("unable to locate managed environment id, or access denied: %s", id)
	}

	result := &ManagedEnvironment{
		Managedenvironment_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

// This method does NOT check whether the user has access
func (dbq *PostgreSQLDatabaseQueries) UnsafeDeleteManagedEnvironmentById(ctx context.Context, id string) (int, error) {

	if err := validateUnsafeQueryParams(id, dbq); err != nil {
		return 0, err
	}

	result := &ManagedEnvironment{
		Managedenvironment_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
