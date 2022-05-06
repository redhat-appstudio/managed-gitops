package db

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateKubernetesResourceToDBResourceMapping(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	kubernetesToDBResourceMappingpost := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test-resource_1",
		KubernetesResourceUID:  "test-resource_uid",
		DBRelationType:         "test-relation_type",
		DBRelationKey:          "test-relation_key",
	}
	err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingpost)
	assert.NoError(t, err)

	kubernetesToDBResourceMappingget := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test-resource_1",
		KubernetesResourceUID:  "test-resource_uid",
		DBRelationType:         "test-relation_type",
		DBRelationKey:          "test-relation_key",
	}

	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingget)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, kubernetesToDBResourceMappingpost, kubernetesToDBResourceMappingget)

}

func TestGetDBResourceMappingForKubernetesResource(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	kubernetesToDBResourceMappingpost := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test-resource_2",
		KubernetesResourceUID:  "test-resource_uid",
		DBRelationType:         "test-relation_type",
		DBRelationKey:          "test-relation_key",
	}
	err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingpost)
	assert.NoError(t, err)

	kubernetesToDBResourceMappingget := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test-resource_2",
		KubernetesResourceUID:  "test-resource_uid",
		DBRelationType:         "test-relation_type",
		DBRelationKey:          "test-relation_key",
	}

	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingget)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, kubernetesToDBResourceMappingpost, kubernetesToDBResourceMappingget)

	kubernetesToDBResourceMappingNotExist := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test-resource_2_not_exist",
		KubernetesResourceUID:  "test-resource_uid_not_exist",
		DBRelationType:         "test-relation_type_not_exist",
		DBRelationKey:          "test-relation_key_not_exist",
	}
	//check for inexistent primary key
	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingNotExist)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

	// Set the invalid value
	kubernetesToDBResourceMappingpost.DBRelationType = strings.Repeat("abc", 100)
	err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingpost)
	assert.True(t, IsMaxLengthError(err))
}

func TestDeleteKubernetesResourceToDBResourceMapping(t *testing.T) {
	SetupforTestingDB(t)
	defer TestTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	kubernetesToDBResourceMapping := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test-resource_11",
		KubernetesResourceUID:  "test-resource_uid",
		DBRelationType:         "test-relation_type",
		DBRelationKey:          "test-relation_key",
	}

	err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
	assert.NoError(t, err)

	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMapping)
	assert.NoError(t, err)

	rowsAffected, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMapping)
	if !assert.True(t, IsResultNotFoundError(err)) {
		return
	}

}
