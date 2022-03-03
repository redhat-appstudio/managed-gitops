package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateandDeleteClusterCredentials(t *testing.T) {
	ctx := context.Background()
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()
	clusterCreds := ClusterCredentials{
		Host:                        "test-host",
		Kube_config:                 "test-kube_config",
		Kube_config_context:         "test-kube_config_context",
		Serviceaccount_bearer_token: "test-serviceaccount_bearer_token",
		Serviceaccount_ns:           "test-serviceaccount_ns",
	}
	err = dbq.CreateClusterCredentials(ctx, &clusterCreds)
	if !assert.NoError(t, err) {
		return
	}
	count, err := dbq.DeleteClusterCredentialsById(ctx, clusterCreds.Clustercredentials_cred_id)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 1, count)
}

func TestGetClusterCredentialsById(t *testing.T) {
	ctx := context.Background()
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()
	clusterCredentials := &ClusterCredentials{
		Clustercredentials_cred_id:  "test-credentials_id",
		Host:                        "test-host",
		Kube_config:                 "test-kube_config",
		Kube_config_context:         "test-kube_config_context",
		Serviceaccount_bearer_token: "test-serviceaccount_bearer_token",
		Serviceaccount_ns:           "test-serviceaccount_ns",
	}
	err = dbq.CreateClusterCredentials(ctx, clusterCredentials)
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
	if !assert.Equal(t, retrievedClusterCredentials, clusterCredentials) {
		return
	}
	count, err := dbq.DeleteClusterCredentialsById(ctx, clusterCredentials.Clustercredentials_cred_id)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 1, count)
}
