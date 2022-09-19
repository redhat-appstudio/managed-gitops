package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllClusterAccess(ctx context.Context, clusterAccess *[]ClusterAccess) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(clusterAccess).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetClusterAccessByPrimaryKey(ctx context.Context, obj *ClusterAccess) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("GetClusterAccessByPrimaryKey",
		"Clusteraccess_gitops_engine_instance_id", obj.Clusteraccess_gitops_engine_instance_id,
		"Clusteraccess_managed_environment_id", obj.Clusteraccess_managed_environment_id,
		"Clusteraccess_user_id", obj.Clusteraccess_user_id); err != nil {
		return err
	}

	var dbResults []ClusterAccess

	err := dbq.dbConnection.Model(&dbResults).
		Where("clusteraccess_user_id = ?", obj.Clusteraccess_user_id).
		Where("clusteraccess_managed_environment_id = ?", obj.Clusteraccess_managed_environment_id).
		Where("clusteraccess_gitops_engine_instance_id = ?", obj.Clusteraccess_gitops_engine_instance_id).
		Context(ctx).Select()

	if err != nil {
		return fmt.Errorf("unable to retrieve ClusterAccess in GetClusterAccessByPrimaryKey: %v", err)
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("No results for ClusterAccess")
	}

	if len(dbResults) != 1 {
		return fmt.Errorf("unexpected number of results for GetClusterAccessByPrimaryKey")
	}

	*obj = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateClusterAccess(ctx context.Context, obj *ClusterAccess) error {

	if err := validateQueryParams(obj.Clusteraccess_gitops_engine_instance_id, dbq); err != nil {
		return err
	}

	if IsEmpty(obj.Clusteraccess_managed_environment_id) {
		return fmt.Errorf("primary key environment id should not be empty")
	}

	if IsEmpty(obj.Clusteraccess_user_id) {
		return fmt.Errorf("primary key user_id should not be empty")
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting cluster access: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteClusterAccessById(ctx context.Context, userId string, managedEnvironmentId string, gitopsEngineInstanceId string) (int, error) {

	if err := validateQueryParams(userId, dbq); err != nil {
		return 0, err
	}

	if IsEmpty(managedEnvironmentId) {
		return 0, fmt.Errorf("primary key is empty")
	}

	if IsEmpty(gitopsEngineInstanceId) {
		return 0, fmt.Errorf("primary key is empty")
	}

	result := &ClusterAccess{}

	deleteResult, err := dbq.dbConnection.Model(result).
		Where("clusteraccess_user_id = ?", userId).
		Where("clusteraccess_managed_environment_id = ?", managedEnvironmentId).
		Where("clusteraccess_gitops_engine_instance_id = ?", gitopsEngineInstanceId).
		Context(ctx).
		Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) ListClusterAccessesByManagedEnvironmentID(ctx context.Context, managedEnvironmentID string, clusterAccesses *[]ClusterAccess) error {

	if err := validateQueryParamsEntity(clusterAccesses, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("ListClusterAccessByManagedEnvironmentID",
		"managedEnvironmentID", managedEnvironmentID); err != nil {
		return err
	}

	var dbResults []ClusterAccess

	// TODO: GITOPSRVCE-68 - PERF - Add index for this

	if err := dbq.dbConnection.Model(&dbResults).
		Where("clusteraccess_managed_environment_id = ?", managedEnvironmentID).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving ListOperationsByResourceIdAndTypeAndOwnerId: %v", err)
	}

	*clusterAccesses = dbResults

	return nil
}

func (obj *ClusterAccess) Dispose(ctx context.Context, dbq DatabaseQueries) error {
	if dbq == nil {
		return fmt.Errorf("missing database interface in ClusterAccess dispose")
	}

	_, err := dbq.DeleteClusterAccessById(ctx, obj.Clusteraccess_user_id, obj.Clusteraccess_managed_environment_id, obj.Clusteraccess_gitops_engine_instance_id)
	return err
}

// GetAsLogKeyValues returns an []interface that can be passed to log.Info(...).
// e.g. log.Info("Creating database resource", obj.GetAsLogKeyValues()...)
func (obj *ClusterAccess) GetAsLogKeyValues() []interface{} {
	if obj == nil {
		return []interface{}{}
	}

	return []interface{}{"engineInstanceID", obj.Clusteraccess_gitops_engine_instance_id,
		"managedEnvironmentID", obj.Clusteraccess_managed_environment_id,
		"userID", obj.Clusteraccess_user_id}
}
