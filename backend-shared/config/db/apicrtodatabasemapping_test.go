package db

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApiCRToDBMappingFunctions(t *testing.T) {
	item := APICRToDatabaseMapping{
		APIResourceType:      APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
		APIResourceUID:       "test-k8s-uid",
		APIResourceName:      "test-k8s-name",
		APIResourceNamespace: "test-k8s-namespace",
		WorkspaceUID:         "test-workspace-uid",
		DBRelationType:       APICRToDatabaseMapping_DBRelationType_SyncOperation,
		DBRelationKey:        "test-key",
	}
	SetupforTestingDB(t)
	defer TestTeardown(t)
	ctx := context.Background()
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()
	err = dbq.CreateAPICRToDatabaseMapping(ctx, &item)
	assert.NoError(t, err)

	fetchRow := APICRToDatabaseMapping{
		APIResourceType: item.APIResourceType,
		APIResourceUID:  item.APIResourceUID,
		DBRelationKey:   item.DBRelationKey,
		DBRelationType:  item.DBRelationType,
	}

	err = dbq.GetDatabaseMappingForAPICR(ctx, &fetchRow)
	assert.NoError(t, err)
	assert.ObjectsAreEqualValues(fetchRow, item)

	var items []APICRToDatabaseMapping

	err = dbq.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx, item.APIResourceType, item.APIResourceName, item.APIResourceNamespace, item.WorkspaceUID, item.DBRelationType, &items)
	assert.NoError(t, err)
	assert.ObjectsAreEqualValues(items[0], item)

	rowsAffected, err := dbq.DeleteAPICRToDatabaseMapping(ctx, &fetchRow)
	assert.NoError(t, err)
	assert.True(t, rowsAffected == 1)
	fetchRow = APICRToDatabaseMapping{
		APIResourceType: item.APIResourceType,
		APIResourceUID:  item.APIResourceUID,
		DBRelationKey:   item.DBRelationKey,
		DBRelationType:  item.DBRelationType,
	}
	err = dbq.GetDatabaseMappingForAPICR(ctx, &fetchRow)
	assert.True(t, IsResultNotFoundError(err))

	// Set the invalid value
	item.APIResourceName = strings.Repeat("abc", 100)
	err = dbq.CreateAPICRToDatabaseMapping(ctx, &item)
	assert.True(t, isMaxLengthError(err))
}
