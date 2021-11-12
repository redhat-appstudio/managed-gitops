package db

import "fmt"

// TODO: Add context to all database query methods

func (dbq *PostgreSQLDatabaseQueries) GetDeploymentToApplicationMappingById(id string) (*DeploymentToApplicationMapping, error) {

	if err := validateGenericEntity(id, dbq); err != nil {
		return nil, err
	}

	var result []DeploymentToApplicationMapping

	if err := dbq.dbConnection.Model(&result).
		Where("dta.deploymenttoapplicationmapping_uid_id = ?", id).
		Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving GetDeploymentToApplicationMappingById: %v", err)
	}

	if len(result) >= 2 {
		return nil, fmt.Errorf("multiple results returned from GetDeploymentToApplicationMappingById")
	}

	if len(result) == 0 {
		return nil, NewResultNotFoundError("error on retrieving GetDeploymentToApplicationMappingById")
	}

	return &(result[0]), nil
}

func (dbq *PostgreSQLDatabaseQueries) UnsafeDeleteDeploymentToApplicationMappingById(id string) (int, error) {

	if err := validateUnsafeGenericEntity(id, dbq); err != nil {
		return 0, err
	}

	entity := &DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(entity).WherePK().Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting application: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateDeploymentToApplicationMapping(obj *DeploymentToApplicationMapping) error {

	if dbq.allowTestUuids {
		if isEmpty(obj.Deploymenttoapplicationmapping_uid_id) {
			obj.Deploymenttoapplicationmapping_uid_id = generateUuid()
		}
	} else {
		if !isEmpty(obj.Deploymenttoapplicationmapping_uid_id) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.Deploymenttoapplicationmapping_uid_id = generateUuid()
	}

	if err := validateGenericEntity(obj.Deploymenttoapplicationmapping_uid_id, dbq); err != nil {
		return err
	}

	if isEmpty(obj.Application_id) {
		return fmt.Errorf("application id should not be empty")
	}

	result, err := dbq.dbConnection.Model(obj).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting DeploymentToApplicationMapping %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}
