package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) ListDeploymentToApplicationMappingByNamespaceUID(ctx context.Context, namespaceUID string,
	deplToAppMappingParam *[]DeploymentToApplicationMapping) error {

	if err := validateQueryParamsEntity(deplToAppMappingParam, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("ListDeploymentToApplicationMappingByNamespaceUID",
		"NamespaceUID", namespaceUID,
	); err != nil {
		return err
	}

	// Application exists, and user can access it

	var dbResults []DeploymentToApplicationMapping

	// TODO: GITOPSRVCE-68 - PERF - Add index for this

	if err := dbq.dbConnection.Model(&dbResults).
		Where("dta.namespace_uid = ?", namespaceUID).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving ListDeploymentToApplicationMappingByNamespaceUID: %v", err)
	}

	*deplToAppMappingParam = dbResults

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) ListDeploymentToApplicationMappingByNamespaceAndName(ctx context.Context, deploymentName string,
	deploymentNamespace string, namespaceUID string, deplToAppMappingParam *[]DeploymentToApplicationMapping) error {

	if err := validateQueryParamsEntity(deplToAppMappingParam, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("ListDeploymentToApplicationMappingByNamespaceAndName",
		"DeploymentName", deploymentName,
		"DeploymentNamespace", deploymentNamespace,
		"NamespaceUID", namespaceUID,
	); err != nil {
		return err
	}

	// Application exists, and user can access it

	var dbResults []DeploymentToApplicationMapping

	// TODO: GITOPSRVCE-68 - PERF - Add index for this

	if err := dbq.dbConnection.Model(&dbResults).
		Where("dta.name = ?", deploymentName).
		Where("dta.namespace = ?", deploymentNamespace).
		Where("dta.namespace_uid = ?", namespaceUID).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving ListDeploymentToApplicationMappingByNamespaceAndName: %v", err)
	}

	*deplToAppMappingParam = dbResults

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteDeploymentToApplicationMappingByNamespaceAndName(ctx context.Context, deploymentName string, deploymentNamespace string, namespaceUID string) (int, error) {

	if err := validateQueryParamsNoPK(dbq); err != nil {
		return 0, err
	}

	if err := isEmptyValues("DeleteDeploymentToApplicationMappingByNamespaceAndName",
		"deploymentName", deploymentName,
		"deploymentNamespace", deploymentNamespace,
		"namespaceUID", namespaceUID); err != nil {

		return 0, err
	}

	entity := &DeploymentToApplicationMapping{}

	deleteResult, err := dbq.dbConnection.Model(entity).
		Where("dta.name = ?", deploymentName).
		Where("dta.namespace = ?", deploymentNamespace).
		Where("dta.namespace_uid = ?", namespaceUID).Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting application: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) GetDeploymentToApplicationMappingByDeplId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping) error {

	if err := validateQueryParamsEntity(deplToAppMappingParam, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("GetDeploymentToApplicationMappingByDeplId",
		"Deploymenttoapplicationmapping_uid_id", deplToAppMappingParam.Deploymenttoapplicationmapping_uid_id,
	); err != nil {
		return err
	}

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

func (dbq *PostgreSQLDatabaseQueries) GetDeploymentToApplicationMappingByApplicationId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping) error {

	if err := validateQueryParamsEntity(deplToAppMappingParam, dbq); err != nil {
		return err
	}

	if IsEmpty(deplToAppMappingParam.Application_id) {
		return fmt.Errorf("GetDeploymentToApplicationMappingByApplicationId: param is nil")
	}

	var dbResults []DeploymentToApplicationMapping

	if err := dbq.dbConnection.Model(&dbResults).
		Where("dta.application_id = ?", deplToAppMappingParam.Application_id). // TODO: GITOPSRVCE-68 - PERF - Index this
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving GetDeploymentToApplicationMappingByApplicationId: %v", err)
	}

	if len(dbResults) > 1 {
		return fmt.Errorf("multiple results returned from GetDeploymentToApplicationMappingByApplicationId")
	}

	if len(dbResults) == 0 {
		return NewResultNotFoundError("GetDeploymentToApplicationMappingByApplicationId")
	}

	*deplToAppMappingParam = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CheckedGetDeploymentToApplicationMappingByDeplId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping, ownerId string) error {

	if err := validateQueryParamsEntity(deplToAppMappingParam, dbq); err != nil {
		return err
	}

	if IsEmpty(deplToAppMappingParam.Deploymenttoapplicationmapping_uid_id) {
		return fmt.Errorf("GetDeploymentToApplicationMappingByDeplId: param is nil")
	}

	if IsEmpty(ownerId) {
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
	if err := dbq.CheckedGetApplicationById(ctx, &deplApplication, ownerId); err != nil {

		if IsResultNotFoundError(err) {
			return NewResultNotFoundError(fmt.Sprintf("unable to retrieve deployment mapping for Application: %v", err))
		}

		return fmt.Errorf("unable to retrieve application of deployment mapping: %v", err)
	}

	*deplToAppMappingParam = dbResults[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CheckedDeleteDeploymentToApplicationMappingByDeplId(ctx context.Context, id string, ownerId string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	entity := &DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: id,
	}

	// Verify that the user can delete the mapping, by checking that they can access it.
	if err := dbq.CheckedGetDeploymentToApplicationMappingByDeplId(ctx, entity, ownerId); err != nil {

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

func (dbq *PostgreSQLDatabaseQueries) DeleteDeploymentToApplicationMappingByDeplId(ctx context.Context, id string) (int, error) {

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
		"NamespaceUID", obj.NamespaceUID,
	); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
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

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllDeploymentToApplicationMapping(ctx context.Context, deploymentToApplicationMappings *[]DeploymentToApplicationMapping) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}
	if err := dbq.dbConnection.Model(deploymentToApplicationMappings).Context(ctx).Select(); err != nil {
		return err
	}
	return nil
}
