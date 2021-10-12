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

	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return nil, fmt.Errorf("invalid pk")
	}

	// A user should only be able to get cluster credentials if:
	// - they have access to a gitops engine instance on that cluster.
	// - they have access to a managed environment using those credentials
	accessibleByUser := false
	{
		// Locate all managedEnvironments using this credential id
		managedEnvironments := &[]ManagedEnvironment{}
		if err := dbq.dbConnection.Model(managedEnvironments).Where("me.clustercredentials_id = ?", id).Select(); err != nil {
			return nil, fmt.Errorf("unable to retrieve managedenvironments: %v", err)
		}

		// Determine if any of those manageEnvironments are accessible by the user

		for _, managedEnvironment := range *managedEnvironments {
			result, err := dbq.GetManagedEnvironmentById(managedEnvironment.Managedenvironment_id, ownerId)
			if err != nil {

				if IsResultNotFoundError(err) {
					continue
				}

				return nil, err
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
				Where("gitopsenginecluster.clustercredentials_id = ?", id).Select()
			if err != nil {
				return nil, err
			}

			// For each engine cluster using this credential, locate an instance that is accessible by the owner
			for _, engineCluster := range engineClustersUsingCredential {

				res, err := dbq.ListAllGitopsEngineInstancesByGitopsEngineCluster(engineCluster.Gitopsenginecluster_id, ownerId)
				if err != nil {
					return nil, err
				}

				// If at least one accessible exists
				if len(res) > 0 {
					accessibleByUser = true
				}
			}

		}

	}

	// No results
	if !accessibleByUser {
		return nil, NewResultNotFoundError("error on retrieving ClusterCredentials")
	}

	// Otherwise, the service is free to retrieve the credentials on behalf of the user, as it is
	// likely there is a valid reason for them doing so.

	result := &ClusterCredentials{
		Clustercredentials_cred_id: id,
	}
	if err := dbq.dbConnection.Model(result).WherePK().Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving ClusterCredentials: %v", err)
	}

	return result, nil
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
