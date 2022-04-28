package db_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

func TestClusterUserFunctions(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	ctx := context.Background()

	dbq, err := db.NewUnsafePostgresDBQueries(true, true)
	Expect(err).To(BeNil())
	defer dbq.CloseDatabase()

	user := &db.ClusterUser{
		Clusteruser_id: "test-user-id",
		User_name:      "tirthuser",
	}
	err = dbq.CreateClusterUser(ctx, user)
	Expect(err).To(BeNil())

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

	// Set the invalid value
	user.User_name = strings.Repeat("abc", 100)
	err = dbq.CreateClusterUser(ctx, user)
	assert.True(t, isMaxLengthError(err))
}
