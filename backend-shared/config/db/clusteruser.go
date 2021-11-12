package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllClusterUsers(ctx context.Context) ([]ClusterUser, error) {
	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return nil, fmt.Errorf("unsafe call to ListAllClusterUsers")
	}

	var clusterUsers []ClusterUser
	err := dbq.dbConnection.Model(&clusterUsers).Context(ctx).Select()

	if err != nil {
		return nil, err
	}

	return clusterUsers, nil
}

func (dbq *PostgreSQLDatabaseQueries) AdminDeleteClusterUserById(ctx context.Context, id string) (int, error) {
	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
	}

	result := &ClusterUser{}

	deleteResult, err := dbq.dbConnection.Model(result).
		Where("clusteruser_id = ?", id).
		Context(ctx).
		Delete()

	if err != nil {
		return 0, fmt.Errorf("error on deleting cluster_user: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateClusterUser(ctx context.Context, obj *ClusterUser) error {

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

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting cluster user: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetClusterUserByUsername(ctx context.Context, userName string) (*ClusterUser, error) {

	// TODO: Add an index for this

	if err := validateQueryParams(userName, dbq); err != nil {
		return nil, err
	}

	var result []ClusterUser

	if err := dbq.dbConnection.Model(&result).
		Where("cu.user_name = ?", userName).
		Context(ctx).
		Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving GetClusterUserByUsername: %v", err)
	}

	if len(result) >= 2 {
		return nil, fmt.Errorf("multiple results returned from GetClusterUserByUsername")
	}

	if len(result) == 0 {
		return nil, NewResultNotFoundError("no results found for GetClusterUserByUsername")
	}

	return &result[0], nil
}

func (dbq *PostgreSQLDatabaseQueries) GetClusterUserById(ctx context.Context, id string) (*ClusterUser, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return nil, err
	}

	var result []ClusterUser

	if err := dbq.dbConnection.Model(&result).
		Where("cu.clusteruser_id = ?", id).
		Context(ctx).
		Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving GetClusterUserById: %v", err)
	}

	if len(result) >= 2 {
		return nil, fmt.Errorf("multiple results returned from GetClusterUserById")
	}

	if len(result) == 0 {
		return nil, NewResultNotFoundError("no results found for GetClusterUserById")
	}

	return &result[0], nil
}
