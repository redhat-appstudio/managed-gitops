package db

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetGitopsEngineInstanceById(t *testing.T) {
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

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-1",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstanceput := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-1",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}
	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceput)
	if !assert.NoError(t, err) {
		return
	}
	gitopsEngineInstanceget := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-1",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceget)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, gitopsEngineInstanceput, gitopsEngineInstanceget)

	//check for inexistent primary key

	gitopsEngineInstanceNotExist := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id-not-exist",
		Namespace_name:          "test-fake-namespace-not-exist",
		Namespace_uid:           "test-fake-namespace-1-not-exist",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceNotExist)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

	// Set the invalid value
	gitopsEngineInstanceput.EngineCluster_id = strings.Repeat("abc", 100)
	err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceput)
	assert.True(t, IsMaxLengthError(err))
}

func TestCreateGitopsEngineInstance(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	clusterCredentials := ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-2",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-2",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstanceput := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-2",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}
	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
	if !assert.NoError(t, err) {
		return
	}

	err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceput)
	if !assert.NoError(t, err) {
		return
	}
	gitopsEngineInstanceget := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-2",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}

	err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceget)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, gitopsEngineInstanceput, gitopsEngineInstanceget)

}

func TestDeleteGitopsEngineInstanceById(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
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

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-3",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	gitopsEngineInstance := GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-3",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}
	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
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
	rowsAffected, err := dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineCluster.Gitopsenginecluster_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteClusterCredentialsById(ctx, clusterCredentials.Clustercredentials_cred_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstance)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}
	err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineCluster)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}
	err = dbq.GetClusterCredentialsById(ctx, &clusterCredentials)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}
}
