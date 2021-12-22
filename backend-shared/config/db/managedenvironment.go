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

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllManagedEnvironments(ctx context.Context, managedEnvironments *[]ManagedEnvironment) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(managedEnvironments).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) ListManagedEnvironmentForClusterCredentialsAndOwnerId(ctx context.Context, clusterCredentialId string, ownerId string, managedEnvironments *[]ManagedEnvironment) error {

	if err := validateQueryParams(clusterCredentialId, dbq); err != nil {
		return err
	}

	if isEmpty(ownerId) {
		return fmt.Errorf("owner id for ListManagedEnvironmentByClusterCredentialsAndOwnerId is empty")
	}

	var result []ManagedEnvironment

	if err := dbq.dbConnection.Model(&result).
		Where("me.clustercredentials_id = ?", clusterCredentialId).
		Where("ca.clusteraccess_user_id = ?", ownerId).
		Join("JOIN clusteraccess AS ca ON ca.clusteraccess_managed_environment_id = me.managedenvironment_id").
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving ManagedEnvironment: %v", err)
	}

	*managedEnvironments = result

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UncheckedGetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment) error {

	if err := validateQueryParamsEntity(managedEnvironment, dbq); err != nil {
		return err
	}

	if isEmpty(managedEnvironment.Managedenvironment_id) {
		return fmt.Errorf("managedenvironment_id is empty in GetManagedEnvironmentById")
	}

	var dbResults []ManagedEnvironment

	if err := dbq.dbConnection.Model(&dbResults).
		Where("me.managedenvironment_id = ?", managedEnvironment.Managedenvironment_id).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving ManagedEnvironment by id '%s': %v", managedEnvironment.Managedenvironment_id, err)
	}

	if len(dbResults) >= 2 {
		return fmt.Errorf("multiple results returned from GetManagedEnvironmentById")
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("error on retrieving GetGitopsEngineInstanceById")
	}

	*managedEnvironment = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment, ownerId string) error {

	if err := validateQueryParamsEntity(managedEnvironment, dbq); err != nil {
		return err
	}

	if isEmpty(managedEnvironment.Managedenvironment_id) {
		return fmt.Errorf("managedenvironment_id is empty in GetManagedEnvironmentById")
	}

	if isEmpty(ownerId) {
		return fmt.Errorf("ownerId is empty in GetManagedEnvironmentById")
	}

	var dbResults []ManagedEnvironment

	if err := dbq.dbConnection.Model(&dbResults).
		Where("me.managedenvironment_id = ?", managedEnvironment.Managedenvironment_id).
		Where("ca.clusteraccess_user_id = ?", ownerId).
		Join("JOIN clusteraccess AS ca ON ca.clusteraccess_managed_environment_id = me.managedenvironment_id").
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving ManagedEnvironment by id '%s': %v", managedEnvironment.Managedenvironment_id, err)
	}

	if len(dbResults) >= 2 {
		return fmt.Errorf("multiple results returned from GetManagedEnvironmentById")
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("error on retrieving GetGitopsEngineInstanceById")
	}

	*managedEnvironment = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteManagedEnvironmentById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	if isEmpty(ownerId) {
		return 0, fmt.Errorf("owner id is empty")
	}

	existingValue := ManagedEnvironment{Managedenvironment_id: id}
	err := dbq.GetManagedEnvironmentById(ctx, &existingValue, ownerId)
	if err != nil || existingValue.Managedenvironment_id != id {
		return 0, fmt.Errorf("unable to locate managed environment id, or access denied: %s", id)
	}

	deleteResult, err := dbq.dbConnection.Model(&existingValue).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

// This method does NOT check whether the user has access
func (dbq *PostgreSQLDatabaseQueries) UncheckedDeleteManagedEnvironmentById(ctx context.Context, id string) (int, error) {

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
