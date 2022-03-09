package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateKubernetesResourceToDBResourceMapping(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	kubernetesToDBResourceMappingpost := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test_resource_1",
		KubernetesResourceUID:  "test_resource_uid",
		DBRelationType:         "test_relation_type",
		DBRelationKey:          "test_relation_key",
	}
	err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingpost)
	assert.NoError(t, err)

	kubernetesToDBResourceMappingget := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test_resource_1",
		KubernetesResourceUID:  "test_resource_uid",
		DBRelationType:         "test_relation_type",
		DBRelationKey:          "test_relation_key",
	}

	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingget)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, kubernetesToDBResourceMappingpost, kubernetesToDBResourceMappingget)

}

func TestGetDBResourceMappingForKubernetesResource(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	kubernetesToDBResourceMappingpost := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test_resource_2",
		KubernetesResourceUID:  "test_resource_uid",
		DBRelationType:         "test_relation_type",
		DBRelationKey:          "test_relation_key",
	}
	err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMappingpost)
	assert.NoError(t, err)

	kubernetesToDBResourceMappingget := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test_resource_2",
		KubernetesResourceUID:  "test_resource_uid",
		DBRelationType:         "test_relation_type",
		DBRelationKey:          "test_relation_key",
	}

	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingget)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, kubernetesToDBResourceMappingpost, kubernetesToDBResourceMappingget)

	kubernetesToDBResourceMappingNotExist := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test_resource_2_not_exist",
		KubernetesResourceUID:  "test_resource_uid_not_exist",
		DBRelationType:         "test_relation_type_not_exist",
		DBRelationKey:          "test_relation_key_not_exist",
	}
	//check for inexistent primary key
	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMappingNotExist)
	if assert.Error(t, err) {
		if !IsResultNotFoundError(err) {
			return
		}
	}

}

func TestDeleteKubernetesResourceToDBResourceMapping(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	ctx := context.Background()

	kubernetesToDBResourceMapping := KubernetesToDBResourceMapping{
		KubernetesResourceType: "test_resource_11",
		KubernetesResourceUID:  "test_resource_uid",
		DBRelationType:         "test_relation_type",
		DBRelationKey:          "test_relation_key",
	}

	err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
	assert.NoError(t, err)

	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMapping)
	assert.NoError(t, err)

	rowsAffected, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
	assert.NoError(t, err)
	assert.Equal(t, rowsAffected, 1)

	err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMapping)
	if assert.Error(t, err) {
		if !IsResultNotFoundError(err) {
			return
		}
	}

}
