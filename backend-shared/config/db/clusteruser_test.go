package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClusterUserFunctions(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	ctx := context.Background()

	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	user := &ClusterUser{
		Clusteruser_id: "test-user-id",
		User_name:      "tirthuser",
	}
	err = dbq.CreateClusterUser(ctx, user)
	if !assert.NoError(t, err) {
		return
	}

	retrieveUser := &ClusterUser{
		User_name: "tirthuser",
	}
	err = dbq.GetClusterUserByUsername(ctx, retrieveUser)
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, err)
	assert.ObjectsAreEqualValues(user, retrieveUser)
	retrieveUser = &ClusterUser{
		Clusteruser_id: user.Clusteruser_id,
	}
	err = dbq.GetClusterUserById(ctx, retrieveUser)
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, err)
	assert.ObjectsAreEqualValues(user, retrieveUser)
	rowsAffected, err := dbq.DeleteClusterUserById(ctx, retrieveUser.Clusteruser_id)
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, err)
	assert.True(t, rowsAffected == 1)
	err = dbq.GetClusterUserById(ctx, retrieveUser)
	assert.True(t, IsResultNotFoundError(err))
}
