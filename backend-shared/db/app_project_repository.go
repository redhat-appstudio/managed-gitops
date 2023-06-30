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

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if IsEmpty(obj.AppProjectRepositoryID) {
			obj.AppProjectRepositoryID = generateUuid()
		}
	} else {
		if !IsEmpty(obj.AppProjectRepositoryID) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.AppProjectRepositoryID = generateUuid()
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

// GetAppProjectRepositoryByUniqueConstraint retrieves AppProjectRepository by unique constraint i.e, clusteruser_id and repo_url.
func (dbq *PostgreSQLDatabaseQueries) GetAppProjectRepositoryByUniqueConstraint(ctx context.Context, obj *AppProjectRepository) error {
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

func (dbq *PostgreSQLDatabaseQueries) DeleteAppProjectRepositoryByRepoCredId(ctx context.Context, obj *AppProjectRepository) (int, error) {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return 0, err
	}

	if err := isEmptyValues("DeleteAppProjectRepositoryByRepoCredId",
		"repositorycredentials_id", obj.RepositorycredentialsID,
	); err != nil {
		return 0, err
	}

	deleteResult, err := dbq.dbConnection.Model(obj).
		Where("repositorycredentials_id = ?", obj.RepositorycredentialsID).
		Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting AppProjectRepository: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

// GetAsLogKeyValues returns an []interface that can be passed to log.Info(...).
// e.g. log.Info("Creating database resource", obj.GetAsLogKeyValues()...)
func (obj *AppProjectRepository) GetAsLogKeyValues() []interface{} {
	if obj == nil {
		return []interface{}{}
	}

	return []interface{}{"app_project_repository_id", obj.AppProjectRepositoryID,
		"clusteruser_id", obj.Clusteruser_id,
		"repositorycredentials_id", obj.RepositorycredentialsID,
		"repo_url", obj.RepoURL}
}
