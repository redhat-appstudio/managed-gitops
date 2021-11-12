package db

import (
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllClusterAccess() ([]ClusterAccess, error) {

	if !dbq.allowUnsafe {
		return nil, fmt.Errorf("unsafe call to ListAllClusterAccess")
	}

	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}
	var clusterAccess []ClusterAccess
	err := dbq.dbConnection.Model(&clusterAccess).Select()

	if err != nil {
		return nil, err
	}

	return clusterAccess, nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateClusterAccess(obj *ClusterAccess) error {

	if err := validateGenericEntity(obj.Clusteraccess_gitops_engine_instance_id, dbq); err != nil {
		return err
	}

	if isEmpty(obj.Clusteraccess_managed_environment_id) {
		return fmt.Errorf("primary key environment id should not be empty")
	}

	if isEmpty(obj.Clusteraccess_user_id) {
		return fmt.Errorf("primary key user_id should not be empty")
	}

	result, err := dbq.dbConnection.Model(obj).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting cluster access: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteClusterAccessById(userId string, managedEnvironmentId string, gitopsEngineInstanceId string) (int, error) {

	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if isEmpty(userId) {
		return 0, fmt.Errorf("primary key is empty")
	}

	if isEmpty(managedEnvironmentId) {
		return 0, fmt.Errorf("primary key is empty")
	}

	if isEmpty(gitopsEngineInstanceId) {
		return 0, fmt.Errorf("primary key is empty")
	}

	result := &ClusterAccess{}

	deleteResult, err := dbq.dbConnection.Model(result).
		Where("clusteraccess_user_id = ?", userId).
		Where("clusteraccess_managed_environment_id = ?", managedEnvironmentId).
		Where("clusteraccess_gitops_engine_instance_id = ?", gitopsEngineInstanceId).
		Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
