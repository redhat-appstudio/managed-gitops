package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var timestamp = time.Date(2022, time.March, 11, 12, 3, 49, 514935000, time.UTC)

func TestGetOperationById(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterUser := ClusterUser{
		Clusteruser_id: "test-user-application-1",
		User_name:      "test-user-application",
	}

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-1",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	managedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-1",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env",
	}

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-1",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-1",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	operationput := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.CreateClusterUser(ctx, &clusterUser)
	if !assert.NoError(t, err) {
		return
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

	err = dbq.CreateOperation(ctx, &operationput, operationput.Operation_owner_user_id)
	if !assert.NoError(t, err) {
		return
	}

	operationget := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.GetOperationById(ctx, &operationget)
	if !assert.NoError(t, err) {
		return
	}

	assert.IsType(t, timestamp, operationget.Created_on)
	assert.IsType(t, timestamp, operationget.Last_state_update)
	operationget.Created_on = operationput.Created_on
	operationget.Last_state_update = operationput.Last_state_update
	assert.Equal(t, operationput, operationget)

	operationNotExist := Operation{
		Operation_id:            "test-operation-1-not-exist",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id-not-exist",
		Resource_type:           "GitopsEngineInstance-not-exist",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.GetOperationById(ctx, &operationNotExist)
	if assert.True(t, IsResultNotFoundError(err)) {
		return
	}

}

func TestCreateOperation(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterUser := ClusterUser{
		Clusteruser_id: "test-user-application-1",
		User_name:      "test-user-application",
	}

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-1",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	managedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-1",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env",
	}

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-1",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-1",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	operationput := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.CreateClusterUser(ctx, &clusterUser)
	if !assert.NoError(t, err) {
		return
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

	err = dbq.CreateOperation(ctx, &operationput, operationput.Operation_owner_user_id)
	if !assert.NoError(t, err) {
		return
	}

	operationget := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.GetOperationById(ctx, &operationget)
	if !assert.NoError(t, err) {
		return
	}

	assert.IsType(t, timestamp, operationget.Created_on)
	assert.IsType(t, timestamp, operationget.Last_state_update)
	operationget.Created_on = operationput.Created_on
	operationget.Last_state_update = operationput.Last_state_update
	assert.Equal(t, operationput, operationget)
}

func TestDeleteOperationById(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterUser := ClusterUser{
		Clusteruser_id: "test-user-application-1",
		User_name:      "test-user-application",
	}

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-1",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	managedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-1",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env",
	}

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-1",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-1",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	operation := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.CreateClusterUser(ctx, &clusterUser)
	if !assert.NoError(t, err) {
		return
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

	err = dbq.CreateOperation(ctx, &operation, operation.Operation_owner_user_id)
	if !assert.NoError(t, err) {
		return
	}

	rowsAffected, err := dbq.DeleteOperationById(ctx, operation.Operation_id)
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
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteClusterUserById(ctx, clusterUser.Clusteruser_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	err = dbq.GetOperationById(ctx, &operation)
	if assert.True(t, IsResultNotFoundError(err)) {
		return
	}

	err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstance)
	if assert.True(t, IsResultNotFoundError(err)) {
		return
	}
	err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineCluster)
	if assert.True(t, IsResultNotFoundError(err)) {
		return
	}

	err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironment)
	if assert.True(t, IsResultNotFoundError(err)) {
		return
	}

	err = dbq.GetClusterCredentialsById(ctx, &clusterCredentials)
	if assert.True(t, IsResultNotFoundError(err)) {
		return
	}

	err = dbq.GetClusterUserById(ctx, &clusterUser)
	if assert.True(t, IsResultNotFoundError(err)) {
		return
	}

}

func TestListOperationsByResourceIdAndTypeAndOwnerId(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterUser := ClusterUser{
		Clusteruser_id: "test-user-application-1",
		User_name:      "test-user-application",
	}

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-1",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	managedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-1",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env",
	}

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-1",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-1",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	operation := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	var operations []Operation

	err = dbq.CreateClusterUser(ctx, &clusterUser)
	if !assert.NoError(t, err) {
		return
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

	err = dbq.CreateOperation(ctx, &operation, operation.Operation_owner_user_id)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, operation.Resource_id, operation.Resource_type, &operations, operation.Operation_owner_user_id)
	assert.NoError(t, err)

	assert.IsType(t, timestamp, operation.Created_on)
	assert.IsType(t, timestamp, operation.Last_state_update)
	operation.Created_on = operations[0].Created_on
	operation.Last_state_update = operations[0].Last_state_update

	assert.Equal(t, operations[0].Last_state_update, operation.Last_state_update)
	assert.Equal(t, len(operations), 1)
}

func TestUpdateOperation(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterUser := ClusterUser{
		Clusteruser_id: "test-user-application-1",
		User_name:      "test-user-application",
	}

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-1",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	managedEnvironment := ManagedEnvironment{
		Managedenvironment_id: "test-managed-env-1",
		Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
		Name:                  "my env",
	}

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-1",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-1",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	operationput := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.CreateClusterUser(ctx, &clusterUser)
	if !assert.NoError(t, err) {
		return
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

	err = dbq.CreateOperation(ctx, &operationput, operationput.Operation_owner_user_id)
	if !assert.NoError(t, err) {
		return
	}

	operationget := Operation{
		Operation_id:            "test-operation-1",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             "test-fake-resource-id",
		Resource_type:           "GitopsEngineInstance-update",
		State:                   OperationState_Waiting,
		Operation_owner_user_id: testClusterUser.Clusteruser_id,
	}

	err = dbq.GetOperationById(ctx, &operationget)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.UpdateOperation(ctx, &operationget)
	if !assert.NoError(t, err) {
		return
	}
	err = dbq.GetOperationById(ctx, &operationget)
	if !assert.NoError(t, err) {
		return
	}

	assert.IsType(t, timestamp, operationget.Created_on)
	assert.IsType(t, timestamp, operationget.Last_state_update)
	operationget.Created_on = operationput.Created_on
	operationget.Last_state_update = operationput.Last_state_update
	assert.Equal(t, operationput, operationget)
}
