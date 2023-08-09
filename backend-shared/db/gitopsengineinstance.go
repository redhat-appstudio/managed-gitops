package db

import (
	"context"
	"fmt"

	"github.com/go-pg/pg/v10"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllGitopsEngineInstances(ctx context.Context, gitopsEngineInstances *[]GitopsEngineInstance) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(gitopsEngineInstances).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

// ListGitopsEngineInstancesForCluster lists the GitOpsEngineInstances that are on the given GitOpsEngineCluster
func (dbq *PostgreSQLDatabaseQueries) ListGitopsEngineInstancesForCluster(ctx context.Context, gitopsEngineCluster GitopsEngineCluster, gitopsEngineInstances *[]GitopsEngineInstance) error {

	if err := validateQueryParamsEntity(gitopsEngineInstances, dbq); err != nil {
		return err
	}

	if IsEmpty(gitopsEngineCluster.Gitopsenginecluster_id) {
		return fmt.Errorf("GitOpsEngineCluster parameter has nil value, when attempting to list corresponding GitOpsEngineInstances")
	}

	if err := dbq.dbConnection.Model(gitopsEngineInstances).Context(ctx).Where("gei.enginecluster_id = ?", gitopsEngineCluster.Gitopsenginecluster_id).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CheckedListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx context.Context, engineClusterId string, ownerId string, gitopsEngineInstancesParam *[]GitopsEngineInstance) error {

	if err := validateQueryParams(engineClusterId, dbq); err != nil {
		return err
	}

	if IsEmpty(ownerId) {
		return fmt.Errorf("engine instance owner id is nil")
	}

	var dbGitopsEngineInstances []GitopsEngineInstance

	if err := dbq.dbConnection.Model(&dbGitopsEngineInstances).
		// gitopsEngineId (of engine instance) must match the provided parameter
		Where("gei.enginecluster_id = ?", engineClusterId).
		// owner id from cluster access must match the provided parameter
		Where("ca.clusteraccess_user_id = ?", ownerId).
		// join on the PK of GitOpsEngineInstance
		Join("JOIN ClusterAccess as ca ON ca.clusteraccess_gitops_engine_instance_id = gei.gitopsengineinstance_id").
		Context(ctx).
		Select(); err != nil {
		return err
	}

	*gitopsEngineInstancesParam = dbGitopsEngineInstances

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetGitopsEngineInstanceById(ctx context.Context, engineInstanceParam *GitopsEngineInstance) error {

	if err := validateQueryParamsEntity(engineInstanceParam, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("GetGitopsEngineInstanceById",
		"Gitopsengineinstance_id", engineInstanceParam.Gitopsengineinstance_id); err != nil {
		return err
	}

	var res []GitopsEngineInstance

	if err := dbq.dbConnection.Model(&res).
		Where("gei.Gitopsengineinstance_id = ?", engineInstanceParam.Gitopsengineinstance_id).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving GetGitopsEngineInstanceById: %v", err)
	}

	if len(res) >= 2 {
		return fmt.Errorf("multiple results returned from GetGitopsEngineInstanceById")
	}

	if len(res) == 0 {
		return NewResultNotFoundError("no results found for GetGitopsEngineInstanceById")
	}

	*engineInstanceParam = res[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CheckedGetGitopsEngineInstanceById(ctx context.Context, engineInstanceParam *GitopsEngineInstance, ownerId string) error {

	if err := validateQueryParamsEntity(engineInstanceParam, dbq); err != nil {
		return err
	}

	if IsEmpty(engineInstanceParam.Gitopsengineinstance_id) {
		return fmt.Errorf("invalid pk")
	}

	if IsEmpty(ownerId) {
		return fmt.Errorf("invalid ownerId")
	}

	var res []GitopsEngineInstance

	if err := dbq.dbConnection.Model(&res).
		Where("gei.Gitopsengineinstance_id = ?", engineInstanceParam.Gitopsengineinstance_id).
		Where("ca.clusteraccess_user_id = ?", ownerId).
		Join("JOIN clusteraccess AS ca ON ca.clusteraccess_gitops_engine_instance_id = gei.gitopsengineinstance_id").
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving GetGitopsEngineInstanceById: %v", err)
	}

	if len(res) >= 2 {
		return fmt.Errorf("multiple results returned from GetGitopsEngineInstanceById")
	}

	if len(res) == 0 {
		return NewResultNotFoundError("no results found for GetGitopsEngineInstanceById")
	}

	*engineInstanceParam = res[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateGitopsEngineInstance(ctx context.Context, obj *GitopsEngineInstance) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if IsEmpty(obj.Gitopsengineinstance_id) {
			obj.Gitopsengineinstance_id = generateUuid()
		}
	} else {
		if !IsEmpty(obj.Gitopsengineinstance_id) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.Gitopsengineinstance_id = generateUuid()
	}

	if IsEmpty(obj.EngineCluster_id) {
		return fmt.Errorf("engine cluster id should not be empty")
	}

	if IsEmpty(obj.Namespace_name) {
		return fmt.Errorf("namespace name should not be empty")
	}

	if IsEmpty(obj.Namespace_uid) {
		return fmt.Errorf("namespace uid should not be empty")
	}

	if err := validateFieldLength(obj); err != nil {
		return err
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

func (dbq *PostgreSQLDatabaseQueries) CheckedDeleteGitopsEngineInstanceById(ctx context.Context, id string, ownerId string) (int, error) {

	return dbq.internalDeleteGitopsEngineInstanceById(ctx, id, ownerId, false)

}

func (dbq *PostgreSQLDatabaseQueries) DeleteGitopsEngineInstanceById(ctx context.Context, id string) (int, error) {

	return dbq.internalDeleteGitopsEngineInstanceById(ctx, id, "", true)

}

func (dbq *PostgreSQLDatabaseQueries) internalDeleteGitopsEngineInstanceById(ctx context.Context, id string, ownerId string, allowUnsafe bool) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	if !allowUnsafe {

		if IsEmpty(ownerId) {
			return 0, fmt.Errorf("owner id is empty")
		}

		// If we are able to retrieve the engine instance with the ownerId, then it is reasonable to
		// assume the a valid purpose for deleting the value on behalf of the user.
		existingValue := GitopsEngineInstance{Gitopsengineinstance_id: id}
		err := dbq.CheckedGetGitopsEngineInstanceById(ctx, &existingValue, ownerId)
		if err != nil || existingValue.Gitopsengineinstance_id != id {
			return 0, fmt.Errorf("unable to locate gitops engine instance id, or access denied: '%s', %v", id, err)
		}
	}

	result := &GitopsEngineInstance{
		Gitopsengineinstance_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Context(ctx).Delete()
	if err != nil {
		pgErr := err.(pg.Error)
		return 0, fmt.Errorf("error on deleting operation: %v\nPGError:%v\n ", err, pgErr)
	}

	return deleteResult.RowsAffected(), nil
}

func (obj *GitopsEngineInstance) Dispose(ctx context.Context, dbq DatabaseQueries) error {
	if dbq == nil {
		return fmt.Errorf("missing database interface in GitopsEngineInstance dispose")
	}

	_, err := dbq.DeleteGitopsEngineInstanceById(ctx, obj.Gitopsengineinstance_id)
	return err
}
