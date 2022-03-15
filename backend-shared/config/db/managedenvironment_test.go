package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateManagedEnvironment(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-3",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	assert.NoError(t, err)

	managedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-3",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env101",
	}
	post := dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
	if !assert.NoError(t, post) {
		return
	}

	getmanagedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-3",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env101",
	}
	get := dbq.GetManagedEnvironmentById(ctx, &getmanagedEnvironment)
	if !assert.NoError(t, get) {
		return
	}
	assert.Equal(t, managedEnvironment, getmanagedEnvironment)

}

func TestGetManagedEnvironmentById(t *testing.T) {

	testSetup(t)
	defer testTeardown(t)

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-4",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	assert.NoError(t, err)

	managedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-4",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env101",
	}
	post := dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
	if !assert.NoError(t, post) {
		return
	}

	getmanagedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-4",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env101",
	}
	get := dbq.GetManagedEnvironmentById(ctx, &getmanagedEnvironment)
	if !assert.NoError(t, get) {
		return
	}
	assert.Equal(t, managedEnvironment, getmanagedEnvironment)

	//check for inexistent primary key

	managedEnvironmentNotExist := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-4-not-exist",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env101-not-exist",
	}

	err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentNotExist)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

}

func TestDeleteManagedEnvironmentById(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-5",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	assert.NoError(t, err)

	managedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-5",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env101",
	}
	post := dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
	if !assert.NoError(t, post) {
		return
	}

	rowsAffected, err := dbq.DeleteManagedEnvironmentById(ctx, managedEnvironment.Managedenvironment_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteClusterCredentialsById(ctx, clusterCredentials.Clustercredentials_cred_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironment)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

	err = dbq.GetClusterCredentialsById(ctx, &clusterCredentials)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

}

func TestListManagedEnvironmentForClusterCredentialsAndOwnerId(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)

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
		Clustercredentials_cred_id:  "test-cluster-creds-test-6",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	managedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-6",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env101",
	}

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-6",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-6",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	clusterAccess := ClusterAccess{
		Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
	}

	var managedEnvironments []ManagedEnvironment

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

	err = dbq.ListManagedEnvironmentForClusterCredentialsAndOwnerId(ctx, clusterCredentials.Clustercredentials_cred_id, clusterAccess.Clusteraccess_user_id, &managedEnvironments)
	assert.NoError(t, err)

	assert.Equal(t, managedEnvironments[0], managedEnvironment)
	assert.Equal(t, len(managedEnvironments), 1)
}
