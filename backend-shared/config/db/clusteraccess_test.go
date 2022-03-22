package db

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClusterAccessFunctions(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	var clusterUser = &ClusterUser{
		Clusteruser_id: "test-user-application",
		User_name:      "test-user-application",
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
	fetchRow := &ClusterAccess{Clusteraccess_user_id: clusterAccess.Clusteraccess_user_id,
		Clusteraccess_managed_environment_id:    clusterAccess.Clusteraccess_managed_environment_id,
		Clusteraccess_gitops_engine_instance_id: clusterAccess.Clusteraccess_gitops_engine_instance_id}
	err = dbq.GetClusterAccessByPrimaryKey(ctx, fetchRow)
	assert.NoError(t, err)
	assert.ObjectsAreEqualValues(fetchRow, clusterAccess)

	affectedRows, err := dbq.DeleteClusterAccessById(ctx, fetchRow.Clusteraccess_user_id, fetchRow.Clusteraccess_managed_environment_id, fetchRow.Clusteraccess_gitops_engine_instance_id)
	assert.NoError(t, err)
	assert.True(t, affectedRows == 1)

	err = dbq.GetClusterAccessByPrimaryKey(ctx, fetchRow)
	assert.True(t, IsResultNotFoundError(err))

	// Set the invalid value
	clusterAccess.Clusteraccess_user_id = strings.Repeat("abc", 100)
	err = dbq.CreateClusterAccess(ctx, &clusterAccess)
	assert.True(t, isMaxLengthError(err))
}
