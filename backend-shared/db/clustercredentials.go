package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllClusterCredentials(ctx context.Context, clusterCredentials *[]ClusterCredentials) error {
	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return fmt.Errorf("unsafe call to ListAllClusterCredentials")
	}

	if err := dbq.dbConnection.Model(clusterCredentials).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateClusterCredentials(ctx context.Context, obj *ClusterCredentials) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {

		if IsEmpty(obj.Clustercredentials_cred_id) {
			obj.Clustercredentials_cred_id = generateUuid()
		}

	} else {

		if !IsEmpty(obj.Clustercredentials_cred_id) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.Clustercredentials_cred_id = generateUuid()
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting cluster credentials: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetClusterCredentialsById(ctx context.Context, clusterCreds *ClusterCredentials) error {

	if err := validateQueryParamsEntity(clusterCreds, dbq); err != nil {
		return err
	}

	var dbResults []ClusterCredentials
	if err := dbq.dbConnection.Model(&dbResults).
		Where("clustercredentials_cred_id = ?", clusterCreds.Clustercredentials_cred_id).Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving ClusterCredentials: %v", err)
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("No results found for GetClusterCredentialsById")
	}

	if len(dbResults) > 1 {
		return fmt.Errorf("unexpected multiple results found in UnsafeGetClusterCredentialsById")
	}

	*clusterCreds = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CheckedGetClusterCredentialsById(ctx context.Context, clusterCredentials *ClusterCredentials, ownerId string) error {

	if err := validateQueryParamsEntity(clusterCredentials, dbq); err != nil {
		return err
	}

	// A user should only be able to get cluster credentials if:
	// - they have access to a gitops engine instance on that cluster.
	// - they have access to a managed environment using those credentials
	accessibleByUser, err := dbq.isAccessibleByUser(ctx, clusterCredentials.Clustercredentials_cred_id, ownerId)
	if err != nil {
		return err
	}

	// No results
	if !accessibleByUser {
		return NewResultNotFoundError("no accessible results")
	}

	// Otherwise, the service is free to retrieve the credentials on behalf of the user, as it is
	// likely there is a valid reason for them doing so.

	var dbResults []ClusterCredentials

	if err = dbq.dbConnection.Model(&dbResults).
		// owner id from cluster access must match the provided parameter
		Where("cc.clustercredentials_cred_id = ?", clusterCredentials.Clustercredentials_cred_id).
		Context(ctx).
		Select(); err != nil {
		return err
	}

	if len(dbResults) >= 2 {
		return fmt.Errorf("multiple results returned from GetClusterCredentialsById")
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("no results found for GetClusterCredentialsById")
	}

	*clusterCredentials = dbResults[0]

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) CheckedListClusterCredentialsByHost(ctx context.Context, hostName string, clusterCredentials *[]ClusterCredentials, ownerId string) error {

	if err := validateQueryParams(hostName, dbq); err != nil {
		return err
	}

	var dbResultCredsWithHostnameResults []ClusterCredentials

	if err := dbq.dbConnection.Model(&dbResultCredsWithHostnameResults).
		// owner id from cluster access must match the provided parameter
		Where("cc.host = ?", hostName).
		Context(ctx).
		Select(); err != nil {

		return err
	}

	if len(dbResultCredsWithHostnameResults) == 0 {
		*clusterCredentials = []ClusterCredentials{}
		return nil
	}

	var matchingClusterCreds []ClusterCredentials

	for idx, credsWithHostName := range dbResultCredsWithHostnameResults {
		// A user should only be able to get cluster credentials if:
		// - they have access to a gitops engine instance on that cluster.
		// - they have access to a managed environment using those credentials
		accessibleByUser, err := dbq.isAccessibleByUser(ctx, credsWithHostName.Clustercredentials_cred_id, ownerId)
		if err != nil {
			return err
		}

		if accessibleByUser {
			matchingClusterCreds = append(matchingClusterCreds, dbResultCredsWithHostnameResults[idx])
		}

	}
	// Otherwise, the service is free to retrieve the credentials on behalf of the user, as it is
	// likely there is a valid reason for them doing so.

	*clusterCredentials = matchingClusterCreds

	return nil

}

// A user should only be able to get cluster credentials if:
// - they have access to a gitops engine instance on that cluster.
// - they have access to a managed environment using those credentials
func (dbq *PostgreSQLDatabaseQueries) isAccessibleByUser(ctx context.Context, clusterCredsId string, ownerId string) (bool, error) {

	accessibleByUser := false

	// Locate all managedEnvironments using this credential id
	managedEnvironments := &[]ManagedEnvironment{}
	if err := dbq.dbConnection.Model(managedEnvironments).
		Where("me.clustercredentials_id = ?", clusterCredsId).
		Context(ctx).
		Select(); err != nil {
		return false, fmt.Errorf("unable to retrieve managedenvironments: %v", err)
	}

	// Determine if any of those manageEnvironments are accessible by the user
	for _, managedEnvironment := range *managedEnvironments {
		dbManagedEnv := ManagedEnvironment{Managedenvironment_id: managedEnvironment.Managedenvironment_id}
		err := dbq.CheckedGetManagedEnvironmentById(ctx, &dbManagedEnv, ownerId)
		if err != nil {

			if IsResultNotFoundError(err) {
				continue
			}

			return false, err
		}

		// At least on of the ManagedEnvironments can be retrieved with this user,
		// so can handle the request.
		accessibleByUser = true
		break
	}

	if !accessibleByUser {

		// Next, see if any of the engine clusters are being used by the user
		engineClustersUsingCredential := []GitopsEngineCluster{}

		// Retrieve all GitopsEngineClusters that reference this credential
		err := dbq.dbConnection.Model(&engineClustersUsingCredential).
			Where("gitops_engine_cluster.clustercredentials_id = ?", clusterCredsId).Context(ctx).Select()
		if err != nil {
			return false, fmt.Errorf("unable to retrieve GitopsEngineClusters that reference credential: %v", err)
		}

		// For each engine cluster using this credential, locate an engine instance that is accessible by the owner
		for _, engineCluster := range engineClustersUsingCredential {

			var gitopsEngineInstances []GitopsEngineInstance
			if err := dbq.CheckedListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx, engineCluster.Gitopsenginecluster_id, ownerId, &gitopsEngineInstances); err != nil {
				return false, err
			}

			// If at least one accessible instance exists
			if len(gitopsEngineInstances) > 0 {
				accessibleByUser = true
				break
			}
		}
	}

	return accessibleByUser, nil

}

func (dbq *PostgreSQLDatabaseQueries) DeleteClusterCredentialsById(ctx context.Context, id string) (int, error) {

	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if IsEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
	}

	result := &ClusterCredentials{
		Clustercredentials_cred_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (obj *ClusterCredentials) Dispose(ctx context.Context, dbq DatabaseQueries) error {
	if dbq == nil {
		return fmt.Errorf("missing database interface in ClusterCredentials dispose")
	}

	_, err := dbq.DeleteClusterCredentialsById(ctx, obj.Clustercredentials_cred_id)
	return err
}

// GetAsLogKeyValues returns an []interface that can be passed to log.Info(...).
// e.g. log.Info("Creating database resource", obj.GetAsLogKeyValues()...)
func (obj *ClusterCredentials) GetAsLogKeyValues() []interface{} {
	if obj == nil {
		return []interface{}{}
	}

	// We avoid logging the bearer_token or kube_config, as these container sensitive user data.
	return []interface{}{"host", obj.Host, "kube-config-length", len(obj.Kube_config),
		"kube-config-context", len(obj.Kube_config_context), "serviceaccount_ns", obj.Serviceaccount_ns,
		"serviceaccount-bearer-token-length", len(obj.Serviceaccount_bearer_token)}
}
