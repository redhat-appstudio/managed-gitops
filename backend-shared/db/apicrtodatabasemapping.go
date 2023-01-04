package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) DeleteAPICRToDatabaseMapping(ctx context.Context, obj *APICRToDatabaseMapping) (int, error) {
	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return 0, err
	}

	if err := isEmptyValues("DeleteAPICRToDatabaseMapping",
		"APIResourceType", obj.APIResourceType,
		"APIResourceUID", obj.APIResourceUID,
		"DBRelationKey", obj.DBRelationKey,
		"DBRelationType", obj.DBRelationType,
	); err != nil {
		return 0, err
	}

	deleteResult, err := dbq.dbConnection.Model(obj).
		Where("atdbm.api_resource_type = ?", obj.APIResourceType).
		Where("atdbm.api_resource_uid = ?", obj.APIResourceUID).
		Where("atdbm.db_relation_key = ?", obj.DBRelationKey).
		Where("atdbm.db_relation_type = ?", obj.DBRelationType).
		Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting APICRToDatabaseMapping: %v", err)
	}

	return deleteResult.RowsAffected(), nil

}

func (dbq *PostgreSQLDatabaseQueries) CreateAPICRToDatabaseMapping(ctx context.Context, obj *APICRToDatabaseMapping) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("CreateAPICRToDatabaseMapping",
		"APIResourceName", obj.APIResourceName,
		"APIResourceNamespace", obj.APIResourceNamespace,
		"APIResourceType", obj.APIResourceType,
		"APIResourceUID", obj.APIResourceUID,
		"DBRelationKey", obj.DBRelationKey,
		"DBRelationType", obj.DBRelationType,
	); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting APICRToDatabaseMapping %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) GetDatabaseMappingForAPICR(ctx context.Context, obj *APICRToDatabaseMapping) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("GetDatabaseMappingForAPICR",
		"APIResourceType", obj.APIResourceType,
		"APIResourceUID", obj.APIResourceUID,
		"DBRelationType", obj.DBRelationType); err != nil {
		return err
	}

	var result []APICRToDatabaseMapping

	if err := dbq.dbConnection.Model(&result).
		// TODO: GITOPSRVCE-68 - PERF - Add a DB index for this
		Where("atdbm.api_resource_type = ?", obj.APIResourceType).
		Where("atdbm.api_resource_uid = ?", obj.APIResourceUID).
		Where("atdbm.db_relation_type = ?", obj.DBRelationType).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving database mapping for APICRToDatabase: %v", err)
	}

	if len(result) == 0 {
		return NewResultNotFoundError(fmt.Sprintf("unable to retrieve APICRToDatabase mapping for %s:%s", obj.APIResourceType, obj.APIResourceUID))
	}

	if len(result) > 1 {
		return fmt.Errorf("unexpected number of results when retrieving APICRToDatabase mapping for %s:%s", obj.APIResourceType, obj.APIResourceUID)
	}

	*obj = result[0]

	return nil

}

// ListAPICRToDatabaseMappingByAPINamespaceAndName returns the DBRelationKey and APIResourceUID for a given type/name/namespace/namespace uid/db-relation-type query
func (dbq *PostgreSQLDatabaseQueries) ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx context.Context,
	apiCRResourceType APICRToDatabaseMapping_ResourceType, crName string, crNamespace string, crNamespaceUID string,
	dbRelationType APICRToDatabaseMapping_DBRelationType, apiCRToDBMappingParam *[]APICRToDatabaseMapping) error {

	if err := validateQueryParamsEntity(apiCRToDBMappingParam, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("ListAPICRToDatabaseMappingByAPINamespaceAndName",
		"apiCRResourceType", apiCRResourceType,
		"crName", crName,
		"crNamespace", crNamespace,
		"crNamespaceUID", crNamespaceUID,
		"dbRelationType", dbRelationType,
	); err != nil {
		return err
	}

	var dbResults []APICRToDatabaseMapping

	// TODO: GITOPSRVCE-68 - PERF - Add index for this

	if err := dbq.dbConnection.Model(&dbResults).
		Where("atdbm.api_resource_type = ?", apiCRResourceType).
		Where("atdbm.api_resource_name = ?", crName).
		Where("atdbm.api_resource_namespace = ?", crNamespace).
		Where("atdbm.api_resource_namespace_uid = ?", crNamespaceUID).
		Where("atdbm.db_relation_type = ?", dbRelationType).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving ListAPICRToDatabaseMappingByAPINamespaceAndName: %v", err)
	}

	*apiCRToDBMappingParam = dbResults

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllAPICRToDatabaseMappings(ctx context.Context, mappings *[]APICRToDatabaseMapping) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(mappings).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

var _ AppScopedDisposableResource = &APICRToDatabaseMapping{}

func (dbMapping *APICRToDatabaseMapping) DisposeAppScoped(ctx context.Context, dbq ApplicationScopedQueries) error {

	if err := isEmptyValues("APICRToDatabaseMappingDispose", "dbq", dbq); err != nil {
		return err
	}
	_, err := dbq.DeleteAPICRToDatabaseMapping(ctx, dbMapping)

	return err
}

// GetAPICRForDatabaseUID retrieves the name/namespace/uid of an API Resources (such as GitOpsDeploymentManagedEnvironment)
// based on the primary key of the corresponding database row (for example, ManagedEnvironment)
func (dbq *PostgreSQLDatabaseQueries) GetAPICRForDatabaseUID(ctx context.Context, obj *APICRToDatabaseMapping) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("GetAPICRForDatabaseUID",
		"APIResourceType", obj.APIResourceType,
		"DBRelationType", obj.DBRelationType,
		"DBRelationKey", obj.DBRelationKey); err != nil {
		return err
	}

	var result []APICRToDatabaseMapping

	if err := dbq.dbConnection.Model(&result).
		// TODO: GITOPSRVCE-68 - PERF - Add a DB index for this
		Where("atdbm.api_resource_type = ?", obj.APIResourceType).
		Where("atdbm.db_relation_type = ?", obj.DBRelationType).
		Where("atdbm.db_relation_key = ?", obj.DBRelationKey).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving database mapping for APICRToDatabase: %v", err)
	}

	if len(result) == 0 {
		return NewResultNotFoundError(fmt.Sprintf("unable to retrieve APICRToDatabase mapping for %s:%s",
			obj.APIResourceType, obj.DBRelationKey))
	}

	if len(result) > 1 {
		return fmt.Errorf("unexpected number of results when retrieving APICRToDatabase mapping for %s:%s",
			obj.APIResourceType, obj.DBRelationKey)
	}

	*obj = result[0]

	return nil
}

// GetAsLogKeyValues returns an []interface that can be passed to log.Info(...).
// e.g. log.Info("Creating database resource", obj.GetAsLogKeyValues()...)
func (obj *APICRToDatabaseMapping) GetAsLogKeyValues() []interface{} {
	if obj == nil {
		return []interface{}{}
	}

	return []interface{}{"apiResourceName", obj.APIResourceName,
		"apiResourceNamespace", obj.APIResourceNamespace,
		"apiResourceType", obj.APIResourceType,
		"apiResourceUID", obj.APIResourceUID,
		"dbRelationKey", obj.DBRelationKey,
		"dbRelationType", obj.DBRelationType,
		"namespaceUID", obj.NamespaceUID}
}

// Get APICRToDatabaseMapping in a batch. Batch size defined by 'limit' and starting point of batch is defined by 'offSet'.
// For example if you want APICRToDatabaseMappings starting from 51-150 then set the limit to 100 and offset to 50.
func (dbq *PostgreSQLDatabaseQueries) GetAPICRToDatabaseMappingBatch(ctx context.Context, apiCRToDatabaseMapping *[]APICRToDatabaseMapping, limit, offSet int) error {
	return dbq.dbConnection.
		Model(apiCRToDatabaseMapping).
		Order("seq_id ASC").
		Limit(limit).   // Batch size
		Offset(offSet). // offset+1 is starting point of batch
		Context(ctx).
		Select()
}
