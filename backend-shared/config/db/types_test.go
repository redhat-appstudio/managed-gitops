package db

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Ensure that the we are able to select on all the fields of the database.
func TestSelectOnAllTables(t *testing.T) {

	SetupforTestingDB(t)
	defer TestTeardown(t)
	ctx := context.Background()

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	var applicationStates []ApplicationState
	err = dbq.UnsafeListAllApplicationStates(ctx, &applicationStates)
	assert.NoError(t, err)

	var applications []Application
	err = dbq.UnsafeListAllApplications(ctx, &applications)
	assert.NoError(t, err)

	var clusterAccess []ClusterAccess
	err = dbq.UnsafeListAllClusterAccess(ctx, &clusterAccess)
	assert.NoError(t, err)

	var clusterCredentials []ClusterCredentials
	err = dbq.UnsafeListAllClusterCredentials(ctx, &clusterCredentials)
	assert.NoError(t, err)

	var clusterUsers []ClusterUser
	err = dbq.UnsafeListAllClusterUsers(ctx, &clusterUsers)
	assert.NoError(t, err)

	var engineClusters []GitopsEngineCluster
	err = dbq.UnsafeListAllGitopsEngineClusters(ctx, &engineClusters)
	assert.NoError(t, err)

	var engineInstances []GitopsEngineInstance
	err = dbq.UnsafeListAllGitopsEngineInstances(ctx, &engineInstances)
	assert.NoError(t, err)

	var managedEnvironments []ManagedEnvironment
	err = dbq.UnsafeListAllManagedEnvironments(ctx, &managedEnvironments)
	assert.NoError(t, err)

	var operations []Operation
	err = dbq.UnsafeListAllOperations(ctx, &operations)
	assert.NoError(t, err)

}

func TestCreateApplication(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()
	_, managedEnvironment, _, gitopsEngineInstance, clusterAccess, err := CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}

	application := &Application{
		Application_id:          "test-my-application",
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	err = dbq.CheckedCreateApplication(ctx, application, clusterAccess.Clusteraccess_user_id)
	if !assert.NoError(t, err) {
		return
	}

	retrievedApplication := Application{Application_id: application.Application_id}

	err = dbq.GetApplicationById(ctx, &retrievedApplication)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, application.Application_id, retrievedApplication.Application_id) {
		return
	}

	rowsAffected, err := dbq.CheckedDeleteApplicationById(ctx, application.Application_id, clusterAccess.Clusteraccess_user_id)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, rowsAffected, 1) {
		return
	}

	retrievedApplication = Application{Application_id: application.Application_id}
	err = dbq.GetApplicationById(ctx, &retrievedApplication)
	if !assert.Error(t, err) {
		return
	}

}

func TestDeploymentToApplicationMapping(t *testing.T) {

	// TODO: GITOPSRVCE-67 - DEBT - Finish filling this in

	SetupforTestingDB(t)
	defer TestTeardown(t)
	ctx := context.Background()

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	mapping := DeploymentToApplicationMapping{}
	err = dbq.CheckedGetDeploymentToApplicationMappingByDeplId(ctx, &mapping, "")
	fmt.Println(err, mapping)

}

func TestGitopsEngineInstanceAndCluster(t *testing.T) {

	SetupforTestingDB(t)
	defer TestTeardown(t)

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()
	ctx := context.Background()

	_, _, gitopsEngineCluster, gitopsEngineInstance, clusterAccess, err := CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}

	retrievedGitopsEngineCluster := &GitopsEngineCluster{Gitopsenginecluster_id: gitopsEngineCluster.Gitopsenginecluster_id}
	if err = dbq.CheckedGetGitopsEngineClusterById(ctx, retrievedGitopsEngineCluster, testClusterUser.Clusteruser_id); !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, &gitopsEngineCluster, &retrievedGitopsEngineCluster) {
		return
	}

	rowsAffected, err := dbq.DeleteClusterAccessById(ctx, clusterAccess.Clusteraccess_user_id, clusterAccess.Clusteraccess_managed_environment_id, clusterAccess.Clusteraccess_gitops_engine_instance_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	// get should return not found, after the delete
	gitopsEngineInstance = &GitopsEngineInstance{Gitopsengineinstance_id: gitopsEngineCluster.Gitopsenginecluster_id}
	if err = dbq.CheckedGetGitopsEngineInstanceById(ctx, gitopsEngineInstance, testClusterUser.Clusteruser_id); !assert.Error(t, err) {
		return
	}
	assert.True(t, IsResultNotFoundError(err))

	rowsAffected, err = dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineCluster.Gitopsenginecluster_id)
	assert.Equal(t, rowsAffected, 1)
	assert.NoError(t, err)

	retrievedGitopsEngineCluster = &GitopsEngineCluster{Gitopsenginecluster_id: gitopsEngineCluster.Gitopsenginecluster_id}
	err = dbq.CheckedGetGitopsEngineClusterById(ctx, retrievedGitopsEngineCluster, testClusterUser.Clusteruser_id)
	assert.Error(t, err)
	assert.True(t, IsResultNotFoundError(err))
}

func TestManagedEnvironment(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	ctx := context.Background()

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	_, managedEnvironment, _, _, clusterAccess, err := CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}

	result := ManagedEnvironment{Managedenvironment_id: managedEnvironment.Managedenvironment_id}
	err = dbq.CheckedGetManagedEnvironmentById(ctx, &result, testClusterUser.Clusteruser_id)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, managedEnvironment.Name, result.Name)

	result = ManagedEnvironment{Managedenvironment_id: managedEnvironment.Managedenvironment_id}
	err = dbq.CheckedGetManagedEnvironmentById(ctx, &result, "another-user")
	assert.NotNil(t, err)
	// deleting from another user should fail
	assert.True(t, IsResultNotFoundError(err))

	rowsAffected, err := dbq.DeleteClusterAccessById(ctx, clusterAccess.Clusteraccess_user_id, clusterAccess.Clusteraccess_managed_environment_id, clusterAccess.Clusteraccess_gitops_engine_instance_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteManagedEnvironmentById(ctx, managedEnvironment.Managedenvironment_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	result = ManagedEnvironment{Managedenvironment_id: managedEnvironment.Managedenvironment_id}
	err = dbq.CheckedGetManagedEnvironmentById(ctx, &result, testClusterUser.Clusteruser_id)
	assert.NotNil(t, err)
	assert.True(t, IsResultNotFoundError(err))

}

func TestOperation(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()
	ctx := context.Background()
	_, _, _, gitopsEngineInstance, _, err := CreateSampleData(dbq)
	if !assert.NoError(t, err) {
		return
	}

	operation := &Operation{
		Operation_id:            "test-operation",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "fake resource id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.CreateOperation(ctx, operation, operation.Operation_owner_user_id)
	assert.NoError(t, err)

	result := Operation{Operation_id: operation.Operation_id}
	err = dbq.CheckedGetOperationById(ctx, &result, operation.Operation_owner_user_id)
	assert.NoError(t, err)
	assert.Equal(t, result.Operation_id, operation.Operation_id)

	result = Operation{Operation_id: operation.Operation_id}
	err = dbq.CheckedGetOperationById(ctx, &result, "another-user")
	if !assert.Error(t, err) {
		return
	}
	assert.True(t, IsResultNotFoundError(err))
	rowsAffected, _ := dbq.CheckedDeleteOperationById(ctx, operation.Operation_id, "another-user")
	assert.Equal(t, rowsAffected, 0)

	rowsAffected, err = dbq.CheckedDeleteOperationById(ctx, operation.Operation_id, operation.Operation_owner_user_id)
	assert.Equal(t, rowsAffected, 1)
	assert.NoError(t, err)
}

func TestClusterUser(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	ctx := context.Background()

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	clusterUser := ClusterUser{
		Clusteruser_id: "test-my-cluster-user-2",
		User_name:      "cluster-mccluster",
	}
	err = dbq.CreateClusterUser(ctx, &clusterUser)
	assert.NoError(t, err)

	retrievedClusterUser := ClusterUser{Clusteruser_id: clusterUser.Clusteruser_id}
	err = dbq.GetClusterUserById(ctx, &retrievedClusterUser)
	assert.NoError(t, err)
	assert.Equal(t, clusterUser.User_name, retrievedClusterUser.User_name)

	rowsAffected, err := dbq.DeleteClusterUserById(ctx, clusterUser.Clusteruser_id)
	assert.Equal(t, rowsAffected, 1)
	assert.NoError(t, err)

	retrievedClusterUser = ClusterUser{Clusteruser_id: clusterUser.Clusteruser_id}
	if err = dbq.GetClusterUserById(ctx, &retrievedClusterUser); !assert.Error(t, err) {
		return
	}
	assert.True(t, IsResultNotFoundError(err), err)

	retrievedClusterUser = ClusterUser{Clusteruser_id: "does-not-exist"}
	if err = dbq.GetClusterUserById(ctx, &retrievedClusterUser); !assert.Error(t, err) {
		return
	}
	assert.True(t, IsResultNotFoundError(err))

}

func TestClusterCredentials(t *testing.T) {

	SetupforTestingDB(t)
	defer TestTeardown(t)

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()
	ctx := context.Background()

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	assert.NoError(t, err)

	var gitopsEngineCluster GitopsEngineCluster
	var gitopsEngineInstance GitopsEngineInstance
	var clusterAccess ClusterAccess
	var managedEnvironment ManagedEnvironment

	// Create managed environment, and cluster access, so the non-unsafe get works below
	{
		managedEnvironment = ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-914",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env",
		}
		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		if !assert.NoError(t, err) {
			return
		}

		gitopsEngineCluster = GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-914",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		if !assert.NoError(t, err) {
			return
		}

		gitopsEngineInstance = GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id",
			Namespace_name:          "test-fake-namespace",
			Namespace_uid:           "test-fake-namespace-914",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}
		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
		if !assert.NoError(t, err) {
			return
		}

		clusterAccess = ClusterAccess{
			Clusteraccess_user_id:                   testClusterUser.Clusteruser_id,
			Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
			Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
		}

		err = dbq.CreateClusterAccess(ctx, &clusterAccess)
		if !assert.NoError(t, err) {
			return
		}
	}

	retrievedClusterCredentials := &ClusterCredentials{
		Clustercredentials_cred_id: clusterCredentials.Clustercredentials_cred_id,
	}
	err = dbq.GetClusterCredentialsById(ctx, retrievedClusterCredentials)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, clusterCredentials.Host, retrievedClusterCredentials.Host)
	assert.Equal(t, clusterCredentials.Kube_config, retrievedClusterCredentials.Kube_config)
	assert.Equal(t, clusterCredentials.Kube_config_context, retrievedClusterCredentials.Kube_config_context)

	retrievedClusterCredentials = &ClusterCredentials{
		Clustercredentials_cred_id: clusterCredentials.Clustercredentials_cred_id,
	}
	err = dbq.CheckedGetClusterCredentialsById(ctx, retrievedClusterCredentials, testClusterUser.Clusteruser_id)
	if !assert.NoError(t, err) ||
		!assert.NotNil(t, retrievedClusterCredentials) {
		return
	}

	assert.Equal(t, clusterCredentials.Host, retrievedClusterCredentials.Host)
	assert.Equal(t, clusterCredentials.Kube_config, retrievedClusterCredentials.Kube_config)
	assert.Equal(t, clusterCredentials.Kube_config_context, retrievedClusterCredentials.Kube_config_context)

	rowsAffected, err := dbq.DeleteClusterAccessById(ctx, clusterAccess.Clusteraccess_user_id, clusterAccess.Clusteraccess_managed_environment_id, clusterAccess.Clusteraccess_gitops_engine_instance_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineCluster.Gitopsenginecluster_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteManagedEnvironmentById(ctx, managedEnvironment.Managedenvironment_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteClusterCredentialsById(ctx, clusterCredentials.Clustercredentials_cred_id)
	// add delete options for other tables table as well!
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	retrievedClusterCredentials = &ClusterCredentials{
		Clustercredentials_cred_id: clusterCredentials.Clustercredentials_cred_id,
	}
	err = dbq.GetClusterCredentialsById(ctx, retrievedClusterCredentials)
	if !assert.Error(t, err) {
		return
	}
	assert.True(t, IsResultNotFoundError(err))

}
