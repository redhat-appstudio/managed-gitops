package db

import "fmt"

func (dbq *PostgreSQLDatabaseQueries) GetGitopsEngineClusterById(id string, ownerId string) (*GitopsEngineCluster, error) {

	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return nil, fmt.Errorf("invalid pk")
	}

	if isEmpty(ownerId) {
		return nil, fmt.Errorf("invalid owner")
	}

	// Return engine instances that are owned by 'ownerid', and are running on cluster 'id'
	engineInstances, err := dbq.ListAllGitopsEngineInstancesByGitopsEngineCluster(id, ownerId)
	if err != nil {
		return nil, NewResultNotFoundError(fmt.Sprintf("unable to list engine instances for engine cluster '%s' %v", id, err))
	}

	// For security reasons, there should be at least one gitops engine instance that is running on the cluster, that
	// this user has access to.
	// - If not, the user should not be able to retrieve the engine instance.
	if len(engineInstances) == 0 {
		return nil, NewResultNotFoundError(
			fmt.Sprintf("no gitops engine clusters were found that had an engine instance owned by '%s'", ownerId))
	}

	result := &GitopsEngineCluster{
		Gitopsenginecluster_id: id,
	}

	if err := dbq.dbConnection.Model(result).WherePK().Select(); err != nil {
		return nil, fmt.Errorf("error on retrieving GitopsEngineCluster: %v", err)
	}

	return result, nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateGitopsEngineCluster(obj *GitopsEngineCluster) error {

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

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

	if isEmpty(obj.Clustercredentials_id) {
		return fmt.Errorf("cluster credentials field should not be empty")
	}

	result, err := dbq.dbConnection.Model(obj).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting engine cluster: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllGitopsEngineClusters() ([]GitopsEngineCluster, error) {

	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return nil, fmt.Errorf("unsafe call to ListAllGitopsEngineClusters")
	}

	var gitopsEngineClusters []GitopsEngineCluster

	err := dbq.dbConnection.Model(&gitopsEngineClusters).Select()
	if err != nil {
		return nil, err
	}

	return gitopsEngineClusters, nil
}

func (dbq *PostgreSQLDatabaseQueries) AdminDeleteGitopsEngineClusterById(id string) (int, error) {
	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return 0, fmt.Errorf("unsafe call to DeleteGitopsEngineClusterById")
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
	}

	result := &GitopsEngineCluster{
		Gitopsenginecluster_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting gitops engine: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
