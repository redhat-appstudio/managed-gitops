package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllAppProjectManagedEnvironment(ctx context.Context, appProjectManagedEnv *[]AppProjectManagedEnvironment) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(appProjectManagedEnv).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateAppProjectManagedEnvironment(ctx context.Context, obj *AppProjectManagedEnvironment) error {

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if IsEmpty(obj.AppProjectManagedEnvironmentID) {
			obj.AppProjectManagedEnvironmentID = generateUuid()
		}
	} else {
		if !IsEmpty(obj.AppProjectManagedEnvironmentID) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.AppProjectManagedEnvironmentID = generateUuid()
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

func (dbq *PostgreSQLDatabaseQueries) UpdateAppProjectManagedEnvironment(ctx context.Context, obj *AppProjectManagedEnvironment) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UpdateAppProjectManagedEnvironment",
		"managed_environment_id", obj.Managed_environment_id); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).WherePK().Context(ctx).Update()
	if err != nil {
		return fmt.Errorf("error on updating appProjectManagedEnv %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) GetAppProjectManagedEnvironmentById(ctx context.Context, obj *AppProjectManagedEnvironment) error {

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

// ListAppProjectManagedEnvironmentByClusterUserId returns a list of all AppProjectManagedEnvironment that reference the specified cluster_user_id row
func (dbq *PostgreSQLDatabaseQueries) ListAppProjectManagedEnvironmentByClusterUserId(ctx context.Context,
	cluster_user_id string, appProjectManagedEnv []AppProjectManagedEnvironment) ([]AppProjectManagedEnvironment, error) {

	if err := validateQueryParams(cluster_user_id, dbq); err != nil {
		return nil, err
	}
	// Retrieve all appProjectManagedEnv which are targeting this cluster_user_id
	err := dbq.dbConnection.Model(appProjectManagedEnv).Context(ctx).Where("cluster_user_id = ?", cluster_user_id).Select()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve appProjectManagedEnv with cluster_user_id: %v", err)
	}

	return appProjectManagedEnv, nil

}

func (dbq *PostgreSQLDatabaseQueries) DeleteAppProjectManagedEnvironmentByClusterUserId(ctx context.Context, obj *AppProjectManagedEnvironment) (int, error) {
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

// GetAsLogKeyValues returns an []interface that can be passed to log.Info(...).
// e.g. log.Info("Creating database resource", obj.GetAsLogKeyValues()...)
func (obj *AppProjectManagedEnvironment) GetAsLogKeyValues() []interface{} {
	if obj == nil {
		return []interface{}{}
	}

	return []interface{}{"app_project_managedenv_id", obj.AppProjectManagedEnvironmentID,
		"cluster_user_id", obj.Clusteruser_id,
		"managed_environment_id", obj.Managed_environment_id}
}
