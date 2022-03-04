package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetGitopsEngineInstanceById(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
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
	assert.Equal(t, gitopsEngineInstanceput.Namespace_name, gitopsEngineInstanceget.Namespace_name)

}

func TestCreateGitopsEngineInstance(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
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
	assert.Equal(t, gitopsEngineInstanceput.Namespace_name, gitopsEngineInstanceget.Namespace_name)

}

func TestDeleteGitopsEngineInstanceById(t *testing.T) {
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

	rowsAffected3, err := dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineCluster.Gitopsenginecluster_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected3, 1)

	rowsAffected5, err := dbq.DeleteClusterCredentialsById(ctx, clusterCredentials.Clustercredentials_cred_id)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected5, 1)

}
