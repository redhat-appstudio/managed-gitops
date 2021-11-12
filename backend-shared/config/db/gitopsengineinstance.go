package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllGitopsEngineInstances(ctx context.Context) ([]GitopsEngineInstance, error) {
	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return nil, fmt.Errorf("unsafe call to ListAllGitopsEngineInstances")
	}

	var gitopsEngineInstances []GitopsEngineInstance
	err := dbq.dbConnection.Model(&gitopsEngineInstances).Context(ctx).Select()

	if err != nil {
		return nil, err
	}

	return gitopsEngineInstances, nil
}

func (dbq *PostgreSQLDatabaseQueries) ListAllGitopsEngineInstancesByGitopsEngineCluster(ctx context.Context, engineClusterId string, ownerId string) ([]GitopsEngineInstance, error) {

	if err := validateQueryParams(engineClusterId, dbq); err != nil {
		return nil, err
	}

	if isEmpty(ownerId) {
		return nil, fmt.Errorf("engine instance owner id is nil")
	}

	var gitopsEngineInstances []GitopsEngineInstance

	err := dbq.dbConnection.Model(&gitopsEngineInstances).
		// gitopsEngineId (of engine instance) must match the provided parameter
		Where("gei.enginecluster_id = ?", engineClusterId).
		// owner id from cluster access must match the provided parameter
		Where("ca.clusteraccess_user_id = ?", ownerId).
		// join on the PK of GitOpsEngineInstance
		Join("JOIN ClusterAccess as ca ON ca.clusteraccess_gitops_engine_instance_id = gei.gitopsengineinstance_id").
		Context(ctx).
		Select()

	if err != nil {
		return nil, err
	}

	return gitopsEngineInstances, nil
}

func (dbq *PostgreSQLDatabaseQueries) GetGitopsEngineInstanceById(ctx context.Context, id string, ownerId string) (*GitopsEngineInstance, error) {

	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return nil, fmt.Errorf("invalid pk")
	}

	if isEmpty(id) {
		return nil, fmt.Errorf("invalid ownerId")
	}

	var res []GitopsEngineInstance

	if err := dbq.dbConnection.Model(&res).
		Where("gei.Gitopsengineinstance_id = ?", id).
		Where("ca.clusteraccess_user_id = ?", ownerId).
		Join("JOIN clusteraccess AS ca ON ca.clusteraccess_gitops_engine_instance_id = gei.gitopsengineinstance_id").
		Context(ctx).
		Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving GetGitopsEngineInstanceById: %v", err)
	}

	if len(res) >= 2 {
		return nil, fmt.Errorf("multiple results returned from GetGitopsEngineInstanceById")
	}

	if len(res) == 0 {
		return nil, NewResultNotFoundError("no results found for GetGitopsEngineInstanceById")
	}

	return &res[0], nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateGitopsEngineInstance(ctx context.Context, obj *GitopsEngineInstance) error {

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if dbq.allowTestUuids {
		if isEmpty(obj.Gitopsengineinstance_id) {
			obj.Gitopsengineinstance_id = generateUuid()
		}
	} else {
		if !isEmpty(obj.Gitopsengineinstance_id) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.Gitopsengineinstance_id = generateUuid()
	}

	if isEmpty(obj.EngineCluster_id) {
		return fmt.Errorf("engine cluster id should not be empty")
	}

	if isEmpty(obj.Namespace_name) {
		return fmt.Errorf("namespace name should not be empty")
	}

	if isEmpty(obj.Namespace_uid) {
		return fmt.Errorf("namespace uid should not be empty")
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting gitops engine instance: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) DeleteGitopsEngineInstanceById(ctx context.Context, id string, ownerId string) (int, error) {

	return dbq.internalDeleteGitopsEngineInstanceById(ctx, id, ownerId, false)

}

func (dbq *PostgreSQLDatabaseQueries) UnsafeDeleteGitopsEngineInstanceById(ctx context.Context, id string) (int, error) {

	return dbq.internalDeleteGitopsEngineInstanceById(ctx, id, "", true)

}

func (dbq *PostgreSQLDatabaseQueries) internalDeleteGitopsEngineInstanceById(ctx context.Context, id string, ownerId string, allowUnsafe bool) (int, error) {
	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
	}

	if !allowUnsafe {

		if isEmpty(ownerId) {
			return 0, fmt.Errorf("owner id is empty")
		}

		// If we are able to retrieve the engine instance with the ownerId, then it is reasonable to
		// assume the a valid purpose for deleting the value on behalf of the user.
		existingValue, err := dbq.GetGitopsEngineInstanceById(ctx, id, ownerId)
		if err != nil || existingValue == nil || existingValue.Gitopsengineinstance_id != id {
			return 0, fmt.Errorf("unable to locate gitops engine instance id, or access denied: '%s', %v", id, err)
		}
	}

	result := &GitopsEngineInstance{
		Gitopsengineinstance_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
