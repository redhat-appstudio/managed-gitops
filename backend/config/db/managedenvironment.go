package db

import "fmt"

func (dbq *PostgreSQLDatabaseQueries) CreateManagedEnvironment(obj *ManagedEnvironment) error {

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if dbq.allowTestUuids {
		if isEmpty(obj.Managedenvironment_id) {
			obj.Managedenvironment_id = generateUuid()
		}
	} else {
		if !isEmpty(obj.Managedenvironment_id) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.Managedenvironment_id = generateUuid()
	}

	if isEmpty(obj.Clustercredentials_id) {
		return fmt.Errorf("managed environment cluster credentials id field should not be empty")
	}

	if isEmpty(obj.Name) {
		return fmt.Errorf("managed environment name field should not be empty")
	}

	result, err := dbq.dbConnection.Model(obj).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting managed environment: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllManagedEnvironments() ([]ManagedEnvironment, error) {
	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return nil, fmt.Errorf("unsafe call to ListAllManagedEnvironments")
	}

	var managedEnvironments []ManagedEnvironment
	err := dbq.dbConnection.Model(&managedEnvironments).Select()

	if err != nil {
		return nil, err
	}

	return managedEnvironments, nil
}

func (dbq *PostgreSQLDatabaseQueries) GetManagedEnvironmentById(id string, ownerId string) (*ManagedEnvironment, error) {

	// find all managed environments a user has access to
	// - select on clusteraccess by userid

	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return nil, fmt.Errorf("invalid pk")
	}

	var result []ManagedEnvironment

	if err := dbq.dbConnection.Model(&result).
		Where("me.managedenvironment_id = ?", id).
		Where("ca.clusteraccess_user_id = ?", ownerId).
		Join("JOIN clusteraccess AS ca ON ca.clusteraccess_managed_environment_id = me.managedenvironment_id").
		Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving ManagedEnvironment: %v", err)
	}

	if len(result) >= 2 {
		return nil, fmt.Errorf("multiple results returned from GetManagedEnvironmentById")
	}

	if len(result) == 0 {
		return nil, NewResultNotFoundError("error on retrieving GetGitopsEngineInstanceById")
	}

	return &result[0], nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteManagedEnvironmentById(id string, ownerId string) (int, error) {
	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
	}

	if isEmpty(ownerId) {
		return 0, fmt.Errorf("owner id is empty")
	}

	existingValue, err := dbq.GetManagedEnvironmentById(id, ownerId)
	if err != nil || existingValue == nil || existingValue.Managedenvironment_id != id {
		return 0, fmt.Errorf("unable to locate managed environment id, or access denied: %s", id)
	}

	result := &ManagedEnvironment{
		Managedenvironment_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

// This method does NOT check whether the user has access
func (dbq *PostgreSQLDatabaseQueries) UnsafeDeleteManagedEnvironmentById(id string) (int, error) {

	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return 0, fmt.Errorf("unsafe operation is not allowed")
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
	}

	result := &ManagedEnvironment{
		Managedenvironment_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting operation: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
