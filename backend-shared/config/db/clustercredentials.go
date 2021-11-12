package db

import (
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllClusterCredentials() ([]ClusterCredentials, error) {
	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return nil, fmt.Errorf("unsafe call to ListAllClusterCredentials")
	}

	var clusterCredentials []ClusterCredentials
	err := dbq.dbConnection.Model(&clusterCredentials).Select()

	if err != nil {
		return nil, err
	}

	return clusterCredentials, nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateClusterCredentials(obj *ClusterCredentials) error {

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if dbq.allowTestUuids {

		if isEmpty(obj.Clustercredentials_cred_id) {
			obj.Clustercredentials_cred_id = generateUuid()
		}

	} else {

		if !isEmpty(obj.Clustercredentials_cred_id) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.Clustercredentials_cred_id = generateUuid()
	}

	result, err := dbq.dbConnection.Model(obj).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting cluster credentials: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UnsafeGetClusterCredentialsById(id string) (*ClusterCredentials, error) {

	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return nil, fmt.Errorf("invalid pk")
	}

	result := &ClusterCredentials{
		Clustercredentials_cred_id: id,
	}
	if err := dbq.dbConnection.Model(result).WherePK().Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving ClusterCredentials: %v", err)
	}

	return result, nil
}

func (dbq *PostgreSQLDatabaseQueries) GetClusterCredentialsById(id string, ownerId string) (*ClusterCredentials, error) {

	if err := validateGenericEntity(id, dbq); err != nil {
		return nil, err
	}

	// A user should only be able to get cluster credentials if:
	// - they have access to a gitops engine instance on that cluster.
	// - they have access to a managed environment using those credentials
	accessibleByUser, err := dbq.isAccessibleByUser(id, ownerId)
	if err != nil {
		return nil, err
	}

	// No results
	if !accessibleByUser {
		return nil, NewResultNotFoundError("error on retrieving ClusterCredentials")
	}

	// Otherwise, the service is free to retrieve the credentials on behalf of the user, as it is
	// likely there is a valid reason for them doing so.

	var result []ClusterCredentials

	err = dbq.dbConnection.Model(&result).
		// owner id from cluster access must match the provided parameter
		Where("cc.clustercredentials_cred_id = ?", id).
		Select()

	if err != nil {
		return nil, err
	}

	if len(result) >= 2 {
		return nil, fmt.Errorf("multiple results returned from GetClusterCredentialsById")
	}

	if len(result) == 0 {
		return nil, NewResultNotFoundError("no results found for GetClusterCredentialsById")
	}

	return &(result[0]), nil

}

func (dbq *PostgreSQLDatabaseQueries) GetClusterCredentialsByHost(hostName string, ownerId string) ([]ClusterCredentials, error) {

	if err := validateGenericEntity(hostName, dbq); err != nil {
		return nil, err
	}

	var credsWithHostnameResults []ClusterCredentials

	err := dbq.dbConnection.Model(&credsWithHostnameResults).
		// owner id from cluster access must match the provided parameter
		Where("cc.host = ?", hostName).
		Select()

	if err != nil {
		return nil, err
	}

	if len(credsWithHostnameResults) == 0 {
		return nil, NewResultNotFoundError("no results found for GetClusterCredentialsByHost")
	}

	anyMatch := false
	for _, credsWithHostName := range credsWithHostnameResults {
		// A user should only be able to get cluster credentials if:
		// - they have access to a gitops engine instance on that cluster.
		// - they have access to a managed environment using those credentials
		accessibleByUser, err := dbq.isAccessibleByUser(credsWithHostName.Clustercredentials_cred_id, ownerId)
		if err != nil {
			return nil, err
		}

		if accessibleByUser {
			anyMatch = true
			break
		}

	}

	// No results
	if !anyMatch {
		return nil, NewResultNotFoundError("error on retrieving ClusterCredentials")
	}

	// Otherwise, the service is free to retrieve the credentials on behalf of the user, as it is
	// likely there is a valid reason for them doing so.

	return credsWithHostnameResults, nil

}

func (dbq *PostgreSQLDatabaseQueries) isAccessibleByUser(clusterCredsId string, ownerId string) (bool, error) {

	// A user should only be able to get cluster credentials if:
	// - they have access to a gitops engine instance on that cluster.
	// - they have access to a managed environment using those credentials
	accessibleByUser := false

	// Locate all managedEnvironments using this credential id
	managedEnvironments := &[]ManagedEnvironment{}
	if err := dbq.dbConnection.Model(managedEnvironments).Where("me.clustercredentials_id = ?", clusterCredsId).Select(); err != nil {
		return false, fmt.Errorf("unable to retrieve managedenvironments: %v", err)
	}

	// Determine if any of those manageEnvironments are accessible by the user

	for _, managedEnvironment := range *managedEnvironments {
		result, err := dbq.GetManagedEnvironmentById(managedEnvironment.Managedenvironment_id, ownerId)
		if err != nil {

			if IsResultNotFoundError(err) {
				continue
			}

			return false, err
		}
		if result != nil {
			// At least on of the ManagedEnvironments can be retrieved with this user,
			// so can handle the request.
			accessibleByUser = true
			break
		}
	}

	if !accessibleByUser {

		// Next, see if any of the engine clusters are being used by the user
		engineClustersUsingCredential := []GitopsEngineCluster{}

		// Retrieve all GitopsEngineClusters that reference this credential
		err := dbq.dbConnection.Model(&engineClustersUsingCredential).
			Where("gitops_engine_cluster.clustercredentials_id = ?", clusterCredsId).Select()
		if err != nil {
			return false, fmt.Errorf("unable to retrieve GitopsEngineClusters that reference credential: %v", err)
		}

		// For each engine cluster using this credential, locate an instance that is accessible by the owner
		for _, engineCluster := range engineClustersUsingCredential {

			res, err := dbq.ListAllGitopsEngineInstancesByGitopsEngineCluster(engineCluster.Gitopsenginecluster_id, ownerId)
			if err != nil {
				return false, err
			}

			// If at least one accessible exists
			if len(res) > 0 {
				accessibleByUser = true
			}
		}
	}

	return accessibleByUser, nil

}

func (dbq *PostgreSQLDatabaseQueries) AdminDeleteClusterCredentialsById(id string) (int, error) {

	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
	}

	result := &ClusterCredentials{
		Clustercredentials_cred_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
