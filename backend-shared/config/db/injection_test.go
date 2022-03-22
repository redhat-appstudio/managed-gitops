package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitopsEngineInstanceWrongInput(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-0",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	assert.NoError(t, err)
	{
		managedEnvironment := ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-0",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env",
		}
		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		if !assert.NoError(t, err) {
			return
		}

		gitopsEngineCluster := GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-0",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		if !assert.NoError(t, err) {
			return
		}

		gitopsEngineInstance := GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id-0",
			Namespace_name:          "test'fake'namespace",
			Namespace_uid:           "test-fake-namespace-0",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}
		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
		if !assert.NoError(t, err) {
			return
		}
		retrieveGitopsEngineInstance := &GitopsEngineInstance{
			Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
		}
		err = dbq.GetGitopsEngineInstanceById(ctx, retrieveGitopsEngineInstance)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, gitopsEngineInstance.Namespace_name, retrieveGitopsEngineInstance.Namespace_name)
	}
	assert.NoError(t, err)
}

func TestClusterCredentialWrongInput(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id: "test-cluster-creds-input",
		Host:                       "host'sInput'",
	}

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	if !assert.NoError(t, err) {
		return
	}
	retrievedClusterCredentials := &ClusterCredentials{
		Clustercredentials_cred_id: clusterCredentials.Clustercredentials_cred_id,
	}
	err = dbq.GetClusterCredentialsById(ctx, retrievedClusterCredentials)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, clusterCredentials.Host, retrievedClusterCredentials.Host)
	assert.NoError(t, err)
}

func TestManagedEnviromentWrongInput(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-1",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	assert.NoError(t, err)
	{
		managedEnvironment := ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-1",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "test'env",
		}
		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		if !assert.NoError(t, err) {
			return
		}
		retrieveManagedEnv := &ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-1",
		}
		err = dbq.GetManagedEnvironmentById(ctx, retrieveManagedEnv)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, managedEnvironment.Name, retrieveManagedEnv.Name)
	}
	assert.NoError(t, err)
}

func TestClusterUserWrongInput(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	user := &ClusterUser{
		Clusteruser_id: "test-user-id",
		User_name:      "samyak'scluster",
	}
	err = dbq.CreateClusterUser(ctx, user)
	if !assert.NoError(t, err) {
		return
	}

	retrieveUser := &ClusterUser{
		User_name: "samyak'scluster",
	}
	err = dbq.GetClusterUserByUsername(ctx, retrieveUser)
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, err)
}

func TestApplicationWrongInput(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	var clusterUser = &ClusterUser{
		Clusteruser_id: "test-user-wrong-application",
		User_name:      "test-user-wrong-application",
	}
	err = dbq.CreateClusterUser(ctx, clusterUser)
	if !assert.NoError(t, err) {
		return
	}

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-5",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	managedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-5",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env",
	}

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-5",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-5",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	clusterAccess := ClusterAccess{
		Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
	}

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.CreateClusterAccess(ctx, &clusterAccess)
	if !assert.NoError(t, err) {
		return
	}

	application := &Application{
		Application_id:          "test-my-application-5",
		Name:                    "test'application",
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
	if !assert.Equal(t, application.Name, retrievedApplication.Name) {
		return
	}
	assert.NoError(t, err)
}
