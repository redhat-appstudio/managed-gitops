package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateandDeleteDeploymentToApplicationMapping(t *testing.T) {

	testSetup(t)
	defer testTeardown(t)

	ctx := context.Background()
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	_, managedEnvironment, _, gitopsEngineInstance, _, err := createSampleData(t, dbq)
	if !assert.NoError(t, err) {
		return
	}

	application := &Application{
		Application_id:          "test-my-application",
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	err = dbq.CreateApplication(ctx, application)

	if !assert.NoError(t, err) {
		return
	}

	deploymentToApplicationMapping := &DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: "test-" + generateUuid(),
		Application_id:                        application.Application_id,
		DeploymentName:                        "test-deployment",
		DeploymentNamespace:                   "test-namespace",
		WorkspaceUID:                          "demo-workspace",
	}

	err = dbq.CreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMapping)
	if !assert.NoError(t, err) {
		return
	}
	fetchRow := &DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
	}
	err = dbq.GetDeploymentToApplicationMappingByDeplId(ctx, fetchRow)
	if !assert.NoError(t, err) && !assert.ObjectsAreEqualValues(fetchRow, deploymentToApplicationMapping) {
		assert.Fail(t, "Values between Fetched Row and Inserted Row don't match.")
	}
	rowsAffected, err := dbq.DeleteDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id)
	assert.NoError(t, err)
	assert.Equal(t, 1, rowsAffected)
}

func TestAllListDeploymentToApplicationMapping(t *testing.T) {
	testSetup(t)
	defer testTeardown(t)

	ctx := context.Background()
	dbq, err := NewUnsafePostgresDBQueries(true, true)
	if !assert.NoError(t, err) {
		return
	}
	defer dbq.CloseDatabase()

	_, managedEnvironment, _, gitopsEngineInstance, _, err := createSampleData(t, dbq)
	if !assert.NoError(t, err) {
		return
	}

	application := &Application{
		Application_id:          "test-my-application",
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironment.Managedenvironment_id,
	}

	err = dbq.CreateApplication(ctx, application)

	if !assert.NoError(t, err) {
		return
	}

	deploymentToApplicationMapping := &DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: "test-" + generateUuid(),
		Application_id:                        application.Application_id,
		DeploymentName:                        "test-deployment",
		DeploymentNamespace:                   "test-namespace",
		WorkspaceUID:                          "demo-workspace",
	}

	err = dbq.CreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMapping)
	if !assert.NoError(t, err) {
		return
	}
	var dbResults []DeploymentToApplicationMapping
	dbResult := &DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
	}

	err = dbq.ListDeploymentToApplicationMappingByWorkspaceUID(ctx, "demo-workspace", &dbResults)
	if !assert.NoError(t, err) {
		return
	}
	assert.True(t, len(dbResults) == 1)
	assert.Equal(t, dbResults[0], *deploymentToApplicationMapping)

	err = dbq.ListDeploymentToApplicationMappingByNamespaceAndName(ctx, deploymentToApplicationMapping.DeploymentName, deploymentToApplicationMapping.DeploymentNamespace, deploymentToApplicationMapping.WorkspaceUID, &dbResults)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, dbResults[0], *deploymentToApplicationMapping)

	err = dbq.GetDeploymentToApplicationMappingByDeplId(ctx, dbResult)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, dbResult, deploymentToApplicationMapping)
	err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, dbResult)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, dbResult, deploymentToApplicationMapping)

	rowsAffected, err := dbq.DeleteDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id)
	assert.NoError(t, err)
	assert.Equal(t, 1, rowsAffected)
}
