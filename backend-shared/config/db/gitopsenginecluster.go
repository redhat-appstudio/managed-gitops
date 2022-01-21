package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UncheckedGetGitopsEngineClusterById(ctx context.Context, gitopsEngineCluster *GitopsEngineCluster) error {

	if err := validateQueryParamsEntity(gitopsEngineCluster, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UncheckedGetGitopsEngineClusterById", "Gitopsenginecluster_id", gitopsEngineCluster.Gitopsenginecluster_id); err != nil {
		return err
	}

	var dbResultEngineClusters []GitopsEngineCluster
	if err := dbq.dbConnection.Model(&dbResultEngineClusters).
		Where("gitopsenginecluster_id = ?", gitopsEngineCluster.Gitopsenginecluster_id).
		Context(ctx).
		Select(); err != nil {
		return fmt.Errorf("error on retrieving GitopsEngineCluster '%s': %v", gitopsEngineCluster.Gitopsenginecluster_id, err)
	}

	if len(dbResultEngineClusters) == 0 {
		return NewResultNotFoundError(
			fmt.Sprintf("no engine clusters was found with id '%s'", gitopsEngineCluster.Gitopsenginecluster_id))
	}

	if len(dbResultEngineClusters) > 1 {
		return fmt.Errorf("unexpected number of dbResultEngineClusters")
	}

	*gitopsEngineCluster = dbResultEngineClusters[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetGitopsEngineClusterById(ctx context.Context, gitopsEngineCluster *GitopsEngineCluster, ownerId string) error {

	if err := validateQueryParamsEntity(gitopsEngineCluster, dbq); err != nil {
		return err
	}

	if isEmpty(gitopsEngineCluster.Gitopsenginecluster_id) {
		return fmt.Errorf("invalid pk in GetGitopsEngineClusterById")
	}

	if isEmpty(ownerId) {
		return fmt.Errorf("invalid owner in GetGitopsEngineClusterById")
	}

	// Return engine instances that are owned by 'ownerid', and are running on cluster 'id'
	var dbResultGitopsEngineInstances []GitopsEngineInstance
	if err := dbq.ListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx, gitopsEngineCluster.Gitopsenginecluster_id, ownerId, &dbResultGitopsEngineInstances); err != nil {
		return NewResultNotFoundError(
			fmt.Sprintf("unable to list engine instances for engine cluster '%s' %v", gitopsEngineCluster.Gitopsenginecluster_id, err))
	}

	// For security reasons, there should be at least one gitops engine instance that is running on the cluster, that
	// this user has access to.
	// - If not, the user should not be able to retrieve the engine instance.
	if len(dbResultGitopsEngineInstances) == 0 {
		return NewResultNotFoundError(
			fmt.Sprintf("no gitops engine clusters were found that had an engine instance owned by '%s'", ownerId))
	}

	var dbResultEngineClusters []GitopsEngineCluster
	if err := dbq.dbConnection.Model(&dbResultEngineClusters).
		Where("gitopsenginecluster_id = ?", gitopsEngineCluster.Gitopsenginecluster_id).
		Context(ctx).
		Select(); err != nil {
		return fmt.Errorf("error on retrieving GitopsEngineCluster '%s': %v", gitopsEngineCluster.Gitopsenginecluster_id, err)
	}

	if len(dbResultEngineClusters) == 0 {
		return NewResultNotFoundError(
			fmt.Sprintf("no engine clusters was found with id '%s'", gitopsEngineCluster.Gitopsenginecluster_id))
	}

	if len(dbResultEngineClusters) > 1 {
		return fmt.Errorf("unexpected number of dbResultEngineClusters")
	}

	*gitopsEngineCluster = dbResultEngineClusters[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) ListGitopsEngineClusterByCredentialId(ctx context.Context, credentialId string, engineClustersParam *[]GitopsEngineCluster, ownerId string) error {

	if err := validateQueryParams(credentialId, dbq); err != nil {
		return err
	}

	if isEmpty(ownerId) {
		return fmt.Errorf("invalid owner in GetGitopsEngineClusterByCredentialId")
	}

	// Locate GitopsEngineClusters that reference the specified credentials
	var dbGitopsEngineClustersWithCreds []GitopsEngineCluster
	if err := dbq.dbConnection.Model(&dbGitopsEngineClustersWithCreds).
		Where("gitops_engine_cluster.clustercredentials_id = ?", credentialId).
		Context(ctx).
		Select(); err != nil {
		// TODO: GITOPS-1702 - PERF -  Add an index for this function, if it's actually used for anything

		return fmt.Errorf("error on retrieving GetGitopsEngineClusterByCredentialId: %v", err)
	}

	if len(dbGitopsEngineClustersWithCreds) == 0 {
		*engineClustersParam = dbGitopsEngineClustersWithCreds
		return nil
	}

	// Next, filter the credentials based on whether the user has a managed environment that uses them
	var res []GitopsEngineCluster
	for _, gitopsEngineCluster := range dbGitopsEngineClustersWithCreds {

		// Return engine instances that are owned by 'ownerid', and are running on cluster 'id'
		var dbEngineInstances []GitopsEngineInstance
		if err := dbq.ListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx, gitopsEngineCluster.Gitopsenginecluster_id, ownerId, &dbEngineInstances); err != nil {
			return fmt.Errorf("unable to list engine instance for '%s', owner '%s', error: %v", gitopsEngineCluster.Gitopsenginecluster_id, ownerId, err)
		}

		// For security reasons, there should be at least one gitops engine instance that is running on the cluster, that
		// this user has access to.
		// - If not, the user should not be able to retrieve the engine instance.
		if len(dbEngineInstances) > 0 {
			res = append(res, gitopsEngineCluster)
		}
	}

	*engineClustersParam = res

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateGitopsEngineCluster(ctx context.Context, obj *GitopsEngineCluster) error {

	if dbq.allowTestUuids {
		if isEmpty(obj.Gitopsenginecluster_id) {
			obj.Gitopsenginecluster_id = generateUuid()
		}
	} else {
		if !isEmpty(obj.Gitopsenginecluster_id) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.Gitopsenginecluster_id = generateUuid()
	}

	if err := validateQueryParams(obj.Gitopsenginecluster_id, dbq); err != nil {
		return err
	}

	if isEmpty(obj.Clustercredentials_id) {
		return fmt.Errorf("cluster credentials field should not be empty")
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting engine cluster: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllGitopsEngineClusters(ctx context.Context, gitopsEngineClusters *[]GitopsEngineCluster) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	err := dbq.dbConnection.Model(gitopsEngineClusters).Context(ctx).Select()
	if err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UncheckedDeleteGitopsEngineClusterById(ctx context.Context, id string) (int, error) {

	if err := validateUnsafeQueryParams(id, dbq); err != nil {
		return 0, err
	}

	result := &GitopsEngineCluster{
		Gitopsenginecluster_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting gitops engine: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
