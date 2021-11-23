package db

import (
	"context"
	"fmt"
)

// TODO: Add tests for these functions

func (dbq *PostgreSQLDatabaseQueries) GetDBResourceMappingForKubernetesResource(ctx context.Context, obj *KubernetesToDBResourceMapping) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if isEmpty(obj.KubernetesResourceType) {
		return fmt.Errorf("KubernetesResourceType field should not be empty")
	}

	if isEmpty(obj.KubernetesResourceUID) {
		return fmt.Errorf("KubernetesResourceUID field should not be empty")
	}

	if isEmpty(obj.DBRelationType) {
		return fmt.Errorf("DBRelationType field should not be empty")
	}

	var result []KubernetesToDBResourceMapping

	if err := dbq.dbConnection.Model(&result).
		// TODO: Performance: Add a DB index for this
		Where("ktdbrm.kubernetes_resource_type = ?", obj.KubernetesResourceType).
		Where("ktdbrm.kubernetes_resource_uid = ?", obj.KubernetesResourceUID).
		Where("ktdbrm.db_relation_type = ?", obj.DBRelationType).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving db resource mapping: %v", err)
	}

	if len(result) == 0 {
		return NewResultNotFoundError(fmt.Sprintf("unable to retrieve mapping for %s:%s", obj.KubernetesResourceType, obj.KubernetesResourceUID))
	}

	if len(result) > 1 {
		return fmt.Errorf("unexpected number of results when retrieving mapping for %s:%s", obj.KubernetesResourceType, obj.KubernetesResourceUID)
	}

	*obj = result[0]

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) CreateKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if isEmpty(obj.DBRelationKey) {
		return fmt.Errorf("DBRelationKey field should not be empty")
	}

	if isEmpty(obj.DBRelationType) {
		return fmt.Errorf("DBRelationType field should not be empty")
	}

	if isEmpty(obj.KubernetesResourceType) {
		return fmt.Errorf("KubernetesResourceType field should not be empty")
	}

	if isEmpty(obj.KubernetesResourceUID) {
		return fmt.Errorf("KubernetesResourceUID field should not be empty")
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting managed environment: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil
}
