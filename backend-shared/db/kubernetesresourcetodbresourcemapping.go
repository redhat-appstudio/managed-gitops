package db

import (
	"context"
	"fmt"
)

// Supported mappings to/from K8s <=> database tables
const (
	// Support K8s Resource types:
	K8sToDBMapping_Namespace = "Namespace"

	// Supported DB tables:
	K8sToDBMapping_ManagedEnvironment   = "ManagedEnvironment"
	K8sToDBMapping_GitopsEngineCluster  = "GitopsEngineCluster"
	K8sToDBMapping_GitopsEngineInstance = "GitopsEngineInstance"
)

func (dbq *PostgreSQLDatabaseQueries) UpdateKubernetesResourceUIDForKubernetesToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UpdateKubernetesToDBResourceMapping",
		"DBRelationKey", obj.DBRelationKey,
		"DBRelationType", obj.DBRelationType,
		"KubernetesResourceType", obj.KubernetesResourceType,
		"KubernetesResourceUID", obj.KubernetesResourceUID,
	); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Set("kubernetes_resource_uid = ?", obj.KubernetesResourceUID).
		Where("ktdbrm.kubernetes_resource_type = ?", obj.KubernetesResourceType).
		Where("ktdbrm.db_relation_key = ?", obj.DBRelationKey).
		Where("ktdbrm.db_relation_type = ?", obj.DBRelationType).
		Update()
	if err != nil {
		return fmt.Errorf("error on updating KubernetesToDBResourceMapping: %v, %s", err, obj.asString())
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d, %s", result.RowsAffected(), obj.asString())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) (int, error) {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return 0, err
	}

	if err := isEmptyValues("DeleteKubernetesResourceToDBResourceMapping",
		"KubernetesResourceType", obj.KubernetesResourceType,
		"KubernetesResourceUID", obj.KubernetesResourceUID,
		"DBRelationKey", obj.DBRelationKey,
		"DBRelationType", obj.DBRelationType); err != nil {
		return 0, err
	}

	deleteResult, err := dbq.dbConnection.Model(obj).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting KubernetesToDBResourceMapping: %v, %s", err, obj.asString())
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) GetDBResourceMappingForKubernetesResource(ctx context.Context, obj *KubernetesToDBResourceMapping) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("GetDBResourceMappingForKubernetesResource",
		"KubernetesResourceType", obj.KubernetesResourceType,
		"KubernetesResourceUID", obj.KubernetesResourceUID,
		"DBRelationType", obj.DBRelationType); err != nil {
		return err
	}

	var result []KubernetesToDBResourceMapping

	if err := dbq.dbConnection.Model(&result).
		Where("ktdbrm.kubernetes_resource_type = ?", obj.KubernetesResourceType).
		Where("ktdbrm.kubernetes_resource_uid = ?", obj.KubernetesResourceUID).
		Where("ktdbrm.db_relation_type = ?", obj.DBRelationType).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving db resource mapping: %v", err)
	}

	if len(result) == 0 {
		return NewResultNotFoundError(fmt.Sprintf("unable to retrieve mapping for %s", obj.asString()))
	}

	if len(result) > 1 {
		return fmt.Errorf("unexpected number of results when retrieving mapping for %s", obj.asString())
	}

	*obj = result[0]

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) GetKubernetesResourceMappingForDatabaseResource(ctx context.Context, obj *KubernetesToDBResourceMapping) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("GetKubernetesResourceMappingForDatabaseResource",
		"KubernetesResourceType", obj.KubernetesResourceType,
		"DBRelationType", obj.DBRelationType,
		"DBRelationKey", obj.DBRelationKey); err != nil {
		return err
	}

	var result []KubernetesToDBResourceMapping

	if err := dbq.dbConnection.Model(&result).
		Where("ktdbrm.kubernetes_resource_type = ?", obj.KubernetesResourceType).
		Where("ktdbrm.db_relation_key = ?", obj.DBRelationKey).
		Where("ktdbrm.db_relation_type = ?", obj.DBRelationType).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving k8s resource UID of db resource mapping: %v", err)
	}

	if len(result) == 0 {
		return NewResultNotFoundError(fmt.Sprintf("unable to k8s resource UID mapping for %s", obj.asString()))
	}

	if len(result) > 1 {
		return fmt.Errorf("unexpected number of results when retrieving k8s resource UID mapping for %s", obj.asString())
	}

	*obj = result[0]

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) CreateKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("CreateKubernetesResourceToDBResourceMapping",
		"DBRelationKey", obj.DBRelationKey,
		"DBRelationType", obj.DBRelationType,
		"KubernetesResourceType", obj.KubernetesResourceType,
		"KubernetesResourceUID", obj.KubernetesResourceUID); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting KubernetesResourceToDBMapping: %v, %s", err, obj.asString())
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d, %s", result.RowsAffected(), obj.asString())
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllKubernetesResourceToDBResourceMapping(ctx context.Context, kubernetesToDBResourceMapping *[]KubernetesToDBResourceMapping) error {
	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(kubernetesToDBResourceMapping).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (obj *KubernetesToDBResourceMapping) Dispose(ctx context.Context, dbq DatabaseQueries) error {
	if dbq == nil {
		return fmt.Errorf("missing database interface in KubernetesToDBResourceMapping dispose")
	}

	_, err := dbq.DeleteKubernetesResourceToDBResourceMapping(ctx, obj)
	return err
}

// Get KubernetesToDBResourceMapping in a batch. Batch size defined by 'limit' and starting point of batch is defined by 'offSet'.
// For example if you want KubernetesToDBResourceMapping starting from 51-150 then set the limit to 100 and offset to 50.
func (dbq *PostgreSQLDatabaseQueries) GetKubernetesToDBResourceMappingBatch(ctx context.Context, k8sToDBResourceMapping *[]KubernetesToDBResourceMapping, limit, offset int) error {
	return dbq.dbConnection.
		Model(k8sToDBResourceMapping).
		Order("seq_id ASC").
		Limit(limit).   // Batch size
		Offset(offset). // offset+1 is starting point of batch
		Context(ctx).
		Select()
}

// GetAsLogKeyValues returns an []interface that can be passed to log.Info(...).
// e.g. log.Info("Creating database resource", obj.GetAsLogKeyValues()...)
func (obj *KubernetesToDBResourceMapping) GetAsLogKeyValues() []interface{} {
	if obj == nil {
		return []interface{}{}
	}

	return []interface{}{
		"k8sResourceType", obj.KubernetesResourceType, "k8sResourceUID", obj.KubernetesResourceUID,
		"dbRelationType", obj.DBRelationType, "dbRelationKey", obj.DBRelationKey}
}

func (obj *KubernetesToDBResourceMapping) asString() string {
	if obj == nil {
		return "nil"
	}
	return fmt.Sprintf("%v/%v/%v/%v", obj.DBRelationType, obj.DBRelationKey, obj.KubernetesResourceType, obj.KubernetesResourceUID)
}
