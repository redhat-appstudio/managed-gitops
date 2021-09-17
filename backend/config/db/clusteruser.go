package db

import (
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllClusterUsers() ([]ClusterUser, error) {
	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return nil, fmt.Errorf("unsafe call to ListAllClusterUsers")
	}

	var clusterUsers []ClusterUser
	err := dbq.dbConnection.Model(&clusterUsers).Select()

	if err != nil {
		return nil, err
	}

	return clusterUsers, nil
}

func (dbq *PostgreSQLDatabaseQueries) AdminDeleteClusterUserById(id string) (int, error) {
	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
	}

	result := &ClusterUser{}

	deleteResult, err := dbq.dbConnection.Model(result).
		Where("clusteruser_id = ?", id).
		Delete()

	if err != nil {
		return 0, fmt.Errorf("error on deleting cluster_user: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateClusterUser(obj *ClusterUser) error {

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if dbq.allowTestUuids {
		if isEmpty(obj.Clusteruser_id) {
			obj.Clusteruser_id = generateUuid()
		}
	} else {
		if !isEmpty(obj.Clusteruser_id) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.Clusteruser_id = generateUuid()
	}

	// State

	if isEmpty(obj.User_name) {
		return fmt.Errorf("user name should not be empty")
	}

	result, err := dbq.dbConnection.Model(obj).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting cluster user: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetClusterUserById(id string) (*ClusterUser, error) {

	// find all managed environments a user has access to
	// - select on clusteraccess by userid

	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return nil, fmt.Errorf("invalid pk")
	}

	result := &ClusterUser{Clusteruser_id: id}

	if err := dbq.dbConnection.Model(result).WherePK().Select(); err != nil {
		return nil, fmt.Errorf("error on retrieving ManagedEnvironment: %v", err)
	}

	return result, nil
}
