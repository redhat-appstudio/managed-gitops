package db

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetGitopsEngineClusterById(t *testing.T) {
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

	gitopsEngineClusterput := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-1",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	assert.NoError(t, err)

	err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterput)
	if !assert.NoError(t, err) {
		return
	}

	gitopsEngineClusterget := GitopsEngineCluster{
		Gitopsenginecluster_id: gitopsEngineClusterput.Gitopsenginecluster_id,
	}

	err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, gitopsEngineClusterput, gitopsEngineClusterget)

	//check for inexistent primary key

	gitopsEngineClusterNotExist := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-1-not-exist",
	}
	err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterNotExist)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

	// Set the invalid value
	gitopsEngineClusterput.Clustercredentials_id = strings.Repeat("abc", 100)
	err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterput)
	assert.True(t, isMaxLengthError(err))
}

func TestCreateGitopsEngineClusterById(t *testing.T) {
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

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	assert.NoError(t, err)

	gitopsEngineClusterput := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-2",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}
	err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterput)
	if !assert.NoError(t, err) {
		return
	}

	gitopsEngineClusterget := GitopsEngineCluster{
		Gitopsenginecluster_id: gitopsEngineClusterput.Gitopsenginecluster_id,
	}

	err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, gitopsEngineClusterput, gitopsEngineClusterget)
}

func TestDeleteGitopsEngineClusterById(t *testing.T) {
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

	err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
	assert.NoError(t, err)

	gitopsEngineCluster := GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-2",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}
	err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
	if !assert.NoError(t, err) {
		return
	}

	rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineCluster.Gitopsenginecluster_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	rowsAffected, err = dbq.DeleteClusterCredentialsById(ctx, clusterCredentials.Clustercredentials_cred_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineCluster)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}
	err = dbq.GetClusterCredentialsById(ctx, &clusterCredentials)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

}
