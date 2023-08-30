package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllAppProjectManagedEnvironments(ctx context.Context, appProjectManagedEnv *[]AppProjectManagedEnvironment) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(appProjectManagedEnv).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateAppProjectManagedEnvironment(ctx context.Context, obj *AppProjectManagedEnvironment) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if IsEmpty(obj.AppprojectManagedenvID) {
			obj.AppprojectManagedenvID = generateUuid()
		}
	} else {
		if !IsEmpty(obj.AppprojectManagedenvID) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.AppprojectManagedenvID = generateUuid()
	}

	if err := isEmptyValues("CreateAppProjectManagedEnvironment",
		"clusteruser_id", obj.Clusteruser_id,
		"managed_environment_id", obj.Managed_environment_id); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting appProjectManagedEnv: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetAppProjectManagedEnvironmentByManagedEnvId(ctx context.Context, obj *AppProjectManagedEnvironment) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if IsEmpty(obj.Managed_environment_id) {
		return fmt.Errorf("managed_environment_id is nil")
	}

	var results []AppProjectManagedEnvironment

	if err := dbq.dbConnection.Model(&results).
		Where("managed_environment_id = ?", obj.Managed_environment_id).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving appProjectManagedenv: %v", err)
	}

	if len(results) == 0 {
		return NewResultNotFoundError(fmt.Sprintf("AppProjectManagedEnvironment '%s'", obj.Managed_environment_id))
	}

	if len(results) > 1 {
		return fmt.Errorf("multiple results found on retrieving appProjectManagedenv: %v", obj.Managed_environment_id)
	}

	*obj = results[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) ListAppProjectManagedEnvironmentByClusterUserId(ctx context.Context,
	clusteruser_id string, appProjectManagedEnvs *[]AppProjectManagedEnvironment) error {

	if err := validateQueryParams(clusteruser_id, dbq); err != nil {
		return err
	}
	// Retrieve all appProjectManagedEnvs which are targeting this clusteruser_id
	err := dbq.dbConnection.Model(appProjectManagedEnvs).Context(ctx).Where("clusteruser_id = ?", clusteruser_id).Select()
	if err != nil {
		return fmt.Errorf("unable to retrieve appProjectManagedEnvs with clusteruser_id: %v", err)
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) DeleteAppProjectManagedEnvironmentByManagedEnvId(ctx context.Context, obj *AppProjectManagedEnvironment) (int, error) {
	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return 0, err
	}

	if err := isEmptyValues("DeleteAppProjectManagedEnvironmentByClusterUserId",
		"managed_environment_id", obj.Managed_environment_id,
	); err != nil {
		return 0, err
	}

	deleteResult, err := dbq.dbConnection.Model(obj).
		Where("managed_environment_id = ?", obj.Managed_environment_id).
		Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting appProjectManagedEnvironment: %v", err)
	}

	return deleteResult.RowsAffected(), nil

}

func (dbq *PostgreSQLDatabaseQueries) CountAppProjectManagedEnvironmentByClusterUserID(ctx context.Context, obj *AppProjectManagedEnvironment) (int, error) {

	count, err := dbq.dbConnection.Model(obj).Where("clusteruser_id = ?", obj.Clusteruser_id).Count()
	if err != nil {
		return 0, fmt.Errorf("error on counting total number of AppProjectManagedEnvironment exists for the user: %w", err)
	}

	return count, nil
}

// GetAsLogKeyValues returns an []interface that can be passed to log.Info(...).
// e.g. log.Info("Creating database resource", obj.GetAsLogKeyValues()...)
func (obj *AppProjectManagedEnvironment) GetAsLogKeyValues() []interface{} {
	if obj == nil {
		return []interface{}{}
	}

	return []interface{}{"appproject_managedenv_id", obj.AppprojectManagedenvID,
		"clusteruser_id", obj.Clusteruser_id,
		"managed_environment_id", obj.Managed_environment_id}
}
