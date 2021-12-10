package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UncheckedListDeploymentToApplicationMappingByNamespaceAndName(ctx context.Context, deploymentName string, deploymentNamespace string, workspaceUID string, deplToAppMappingParam *[]DeploymentToApplicationMapping) error {

	if err := validateQueryParamsEntity(deplToAppMappingParam, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UncheckedListDeploymentToApplicationMappingByNamespaceAndName",
		"DeploymentName", deploymentName,
		"DeploymentNamespace", deploymentNamespace,
		"WorkspaceUID", workspaceUID,
	); err != nil {
		return err
	}

	// Application exists, and user can access it

	var dbResults []DeploymentToApplicationMapping

	// TODO: PERF - Add index for this

	if err := dbq.dbConnection.Model(&dbResults).
		Where("dta.name = ?", deploymentName).
		Where("dta.namespace = ?", deploymentNamespace).
		Where("dta.workspace_uid = ?", workspaceUID).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving UncheckedListDeploymentToApplicationMappingByNamespaceAndName: %v", err)
	}

	*deplToAppMappingParam = dbResults

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UncheckedGetDeploymentToApplicationMappingByDeplId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping) error {

	if err := validateQueryParamsEntity(deplToAppMappingParam, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UncheckedGetDeploymentToApplicationMappingByDeplId",
		"Deploymenttoapplicationmapping_uid_id", deplToAppMappingParam.Deploymenttoapplicationmapping_uid_id,
	); err != nil {
		return err
	}

	// Application exists, and user can access it

	var dbResults []DeploymentToApplicationMapping

	if err := dbq.dbConnection.Model(&dbResults).
		Where("dta.deploymenttoapplicationmapping_uid_id = ?", deplToAppMappingParam.Deploymenttoapplicationmapping_uid_id).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving GetDeploymentToApplicationMappingById: %v", err)
	}

	if len(dbResults) >= 2 {
		return fmt.Errorf("multiple results returned from GetDeploymentToApplicationMappingById")
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("GetDeploymentToApplicationMappingById")
	}

	*deplToAppMappingParam = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetDeploymentToApplicationMappingByDeplId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping, ownerId string) error {

	if err := validateQueryParamsEntity(deplToAppMappingParam, dbq); err != nil {
		return err
	}

	if isEmpty(deplToAppMappingParam.Deploymenttoapplicationmapping_uid_id) {
		return fmt.Errorf("GetDeploymentToApplicationMappingByDeplId: param is nil")
	}

	if isEmpty(ownerId) {
		return fmt.Errorf("ownerid is empty")
	}

	// Application exists, and user can access it

	var dbResults []DeploymentToApplicationMapping

	if err := dbq.dbConnection.Model(&dbResults).
		Where("dta.deploymenttoapplicationmapping_uid_id = ?", deplToAppMappingParam.Deploymenttoapplicationmapping_uid_id).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving GetDeploymentToApplicationMappingById: %v", err)
	}

	if len(dbResults) >= 2 {
		return fmt.Errorf("multiple results returned from GetDeploymentToApplicationMappingById")
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("GetDeploymentToApplicationMappingById")
	}

	// Check that the user has access to retrieve the referenced Application
	deplApplication := Application{Application_id: dbResults[0].Application_id}
	if err := dbq.GetApplicationById(ctx, &deplApplication, ownerId); err != nil {

		if IsResultNotFoundError(err) {
			return NewResultNotFoundError(fmt.Sprintf("unable to retrieve deployment mapping for Application: %v", err))
		}

		return fmt.Errorf("unable to retrieve application of deployment mapping: %v", err)
	}

	*deplToAppMappingParam = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteDeploymentToApplicationMappingByDeplId(ctx context.Context, id string, ownerId string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	entity := &DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: id,
	}

	// Verify that the user can delete the mapping, by checking that they can access it.
	if err := dbq.GetDeploymentToApplicationMappingByDeplId(ctx, entity, ownerId); err != nil {

		if IsResultNotFoundError(err) {
			return 0, nil
		}

		return 0, err
	}

	deleteResult, err := dbq.dbConnection.Model(entity).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting application: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) UncheckedUpdateApplication(ctx context.Context, obj *Application) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UncheckedUpdateApplication", "Application_id", obj.Application_id); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).WherePK().Context(ctx).Update()
	if err != nil {
		return fmt.Errorf("error on updating application %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) UncheckedDeleteDeploymentToApplicationMappingByDeplId(ctx context.Context, id string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	entity := &DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(entity).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting application: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateDeploymentToApplicationMapping(ctx context.Context, obj *DeploymentToApplicationMapping) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("CreateDeploymentToApplicationMapping",
		"Application_id", obj.Application_id,
		"Deploymenttoapplicationmapping_uid_id", obj.Deploymenttoapplicationmapping_uid_id,
		"DeploymentName", obj.DeploymentName,
		"DeploymentNamespace", obj.DeploymentNamespace,
		"WorkspaceUID", obj.WorkspaceUID,
	); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting DeploymentToApplicationMapping %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}
