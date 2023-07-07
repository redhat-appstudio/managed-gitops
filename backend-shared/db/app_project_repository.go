package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllAppProjectRepositories(ctx context.Context, appRepositories *[]AppProjectRepository) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(appRepositories).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateAppProjectRepository(ctx context.Context, obj *AppProjectRepository) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if IsEmpty(obj.AppprojectRepositoryID) {
			obj.AppprojectRepositoryID = generateUuid()
		}
	} else {
		if !IsEmpty(obj.AppprojectRepositoryID) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.AppprojectRepositoryID = generateUuid()
	}

	if err := isEmptyValues("CreateAppProjectRepository",
		"clusteruser_id", obj.Clusteruser_id,
		"repo_url", obj.RepoURL); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting appProjectRepository: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

// GetAppProjectRepositoryByClusterUserAndRepoURL retrieves AppProjectRepository by unique constraint i.e, clusteruser_id and repo_url.
func (dbq *PostgreSQLDatabaseQueries) GetAppProjectRepositoryByClusterUserAndRepoURL(ctx context.Context, obj *AppProjectRepository) error {
	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	var results []AppProjectRepository

	if err := dbq.dbConnection.Model(&results).
		Where("clusteruser_id = ? AND repo_url = ?", obj.Clusteruser_id, obj.RepoURL).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error retrieving AppProjectRepository: %v", err)
	}

	if len(results) == 0 {
		return NewResultNotFoundError(fmt.Sprintf("AppProjectRepository '%s:%s'", obj.Clusteruser_id, obj.RepoURL))
	}

	if len(results) > 1 {
		return fmt.Errorf("multiple results found retrieving AppProjectRepository: %v:%v", obj.Clusteruser_id, obj.RepoURL)
	}

	*obj = results[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) ListAppProjectRepositoryByClusterUserId(ctx context.Context,
	clusteruser_id string, appProjectRepositories *[]AppProjectRepository) error {

	if err := validateQueryParams(clusteruser_id, dbq); err != nil {
		return err
	}
	// Retrieve all appProjectRepository which are targeting this clusteruser_id
	err := dbq.dbConnection.Model(appProjectRepositories).Context(ctx).Where("clusteruser_id = ?", clusteruser_id).Select()
	if err != nil {
		return fmt.Errorf("unable to retrieve appProjectRepository with clusteruser_id: %v", err)
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) UpdateAppProjectRepository(ctx context.Context, obj *AppProjectRepository) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UpdateAppProjectRepository",
		"appproject_repository_id", obj.AppprojectRepositoryID,
		"clusteruser_id", obj.Clusteruser_id,
		"repositorycredentials_id", obj.RepositorycredentialsID,
		"repo_url", obj.RepoURL); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).WherePK().Context(ctx).Update()
	if err != nil {
		return fmt.Errorf("error on updating appProjectRepository %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) DeleteAppProjectRepositoryByAppProjectRepositoryID(ctx context.Context, obj *AppProjectRepository) (int, error) {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return 0, err
	}

	if err := isEmptyValues("DeleteAppProjectRepositoryByAppProjectRepositoryID",
		"appprojectRepositoryID", obj.AppprojectRepositoryID,
	); err != nil {
		return 0, err
	}

	deleteResult, err := dbq.dbConnection.Model(obj).
		Where("appproject_repository_id = ?", obj.AppprojectRepositoryID).
		Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting AppProjectRepository by primary key: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteAppProjectRepositoryByClusterUserAndRepoURL(ctx context.Context, obj *AppProjectRepository) (int, error) {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return 0, err
	}

	if err := isEmptyValues("DeleteAppProjectRepositoryByClusterUserAndRepoURL",
		"clusteruser_id", obj.Clusteruser_id,
		"repo_url", obj.RepoURL,
	); err != nil {
		return 0, err
	}

	deleteResult, err := dbq.dbConnection.Model(obj).
		Where("clusteruser_id = ? AND repo_url = ?", obj.Clusteruser_id, obj.RepoURL).
		Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting AppProjectRepository based on clusteruser_id and repo_url: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) CountAppProjectRepositoryByClusterUserID(ctx context.Context, obj *AppProjectRepository) (int, error) {

	count, err := dbq.dbConnection.Model(obj).Where("clusteruser_id = ?", obj.Clusteruser_id).Count()
	if err != nil {
		return 0, fmt.Errorf("error on counting total number of AppProjectRepository exists for the user: %w", err)
	}

	return count, nil
}

// GetAsLogKeyValues returns an []interface that can be passed to log.Info(...).
// e.g. log.Info("Creating database resource", obj.GetAsLogKeyValues()...)
func (obj *AppProjectRepository) GetAsLogKeyValues() []interface{} {
	if obj == nil {
		return []interface{}{}
	}

	return []interface{}{"appproject_repository_id", obj.AppprojectRepositoryID,
		"clusteruser_id", obj.Clusteruser_id,
		"repositorycredentials_id", obj.RepositorycredentialsID,
		"repo_url", obj.RepoURL}
}
