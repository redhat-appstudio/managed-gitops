package db

import (
	"context"
	"fmt"
	"time"
)

func (dbq *PostgreSQLDatabaseQueries) CreateManagedEnvironment(ctx context.Context, obj *ManagedEnvironment) error {

	if err := validateQueryParams(obj.Clustercredentials_id, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if IsEmpty(obj.Managedenvironment_id) {
			obj.Managedenvironment_id = generateUuid()
		}
	} else {
		if !IsEmpty(obj.Managedenvironment_id) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.Managedenvironment_id = generateUuid()
	}

	if IsEmpty(obj.Name) {
		return fmt.Errorf("managed environment name field should not be empty")
	}

	obj.Created_on = time.Now()

	if err := validateFieldLength(obj); err != nil {
		return err
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

	if IsEmpty(ownerId) {
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

func (dbq *PostgreSQLDatabaseQueries) GetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment) error {

	if err := validateQueryParamsEntity(managedEnvironment, dbq); err != nil {
		return err
	}

	if IsEmpty(managedEnvironment.Managedenvironment_id) {
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
		return NewResultNotFoundError("error on retrieving GetManagedEnvironmentById")
	}

	*managedEnvironment = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CheckedGetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment, ownerId string) error {

	if err := validateQueryParamsEntity(managedEnvironment, dbq); err != nil {
		return err
	}

	if IsEmpty(managedEnvironment.Managedenvironment_id) {
		return fmt.Errorf("managedenvironment_id is empty in GetManagedEnvironmentById")
	}

	if IsEmpty(ownerId) {
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

func (dbq *PostgreSQLDatabaseQueries) CheckedDeleteManagedEnvironmentById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	if IsEmpty(ownerId) {
		return 0, fmt.Errorf("owner id is empty")
	}

	existingValue := ManagedEnvironment{Managedenvironment_id: id}
	err := dbq.CheckedGetManagedEnvironmentById(ctx, &existingValue, ownerId)
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
func (dbq *PostgreSQLDatabaseQueries) DeleteManagedEnvironmentById(ctx context.Context, id string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
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

func (dbq *PostgreSQLDatabaseQueries) UpdateManagedEnvironment(ctx context.Context, obj *ManagedEnvironment) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UpdateManagedEnvironment",
		"Clustercredentials_id", obj.Clustercredentials_id); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).WherePK().Context(ctx).Update()
	if err != nil {
		return fmt.Errorf("error on updating operation: %v, %v", err, obj.Managedenvironment_id)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d, %v", result.RowsAffected(), obj.Managedenvironment_id)
	}

	return nil

}

func (obj *ManagedEnvironment) Dispose(ctx context.Context, dbq DatabaseQueries) error {
	if dbq == nil {
		return fmt.Errorf("missing database interface in ManagedEnvironment dispose")
	}

	_, err := dbq.DeleteManagedEnvironmentById(ctx, obj.Managedenvironment_id)
	return err
}

// GetAsLogKeyValues returns an []interface that can be passed to log.Info(...).
// e.g. log.Info("Creating database resource", obj.GetAsLogKeyValues()...)
func (obj *ManagedEnvironment) GetAsLogKeyValues() []interface{} {
	if obj == nil {
		return []interface{}{}
	}

	return []interface{}{"clusterCredentialsID", obj.Clustercredentials_id,
		"managedEnvironmentID", obj.Managedenvironment_id,
		"managedEnvironmentName", obj.Name}
}
