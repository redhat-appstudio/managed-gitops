package db

import "fmt"

func (dbq *PostgreSQLDatabaseQueries) UnsafeGetApplicationById(id string) (*Application, error) {

	if err := validateUnsafeGenericEntity(id, dbq); err != nil {
		return nil, err
	}

	result := &Application{
		Application_id: id,
	}

	if err := dbq.dbConnection.Model(result).WherePK().Select(); err != nil {

		return nil, fmt.Errorf("error on retrieving Applicatiion: %v", err)
	}

	return result, nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateApplication(obj *Application, ownerId string) error {

	if dbq.allowTestUuids {
		if isEmpty(obj.Application_id) {
			obj.Application_id = generateUuid()
		}
	} else {
		if !isEmpty(obj.Application_id) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.Application_id = generateUuid()
	}

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if isEmpty(obj.Engine_instance_inst_id) {
		return fmt.Errorf("application's engine instance id field should not be empty")

	}

	if isEmpty(obj.Managed_environment_id) {
		return fmt.Errorf("application's environment id field should not be empty")

	}

	if isEmpty(obj.Spec_field) {
		return fmt.Errorf("application's spec field should not be empty")

	}

	if isEmpty(obj.Name) {
		return fmt.Errorf("application's name field should not be empty")
	}

	// Verify the user can access the managed environment
	managedEnv, err := dbq.GetManagedEnvironmentById(obj.Managed_environment_id, ownerId)
	if err != nil || managedEnv == nil {
		return fmt.Errorf("on creating Application, unable to retrieve managed environment %s for user %s: %v", obj.Managed_environment_id, ownerId, err)
	}

	result, err := dbq.dbConnection.Model(obj).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting application: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

// func (dbq *PostgreSQLDatabaseQueries) UnsafeCreateApplication(obj *Application) error {

// 	if dbq.allowTestUuids {
// 		if isEmpty(obj.Application_id) {
// 			obj.Application_id = generateUuid()
// 		}
// 	} else {
// 		if !isEmpty(obj.Application_id) {
// 			return fmt.Errorf("primary key should be empty")
// 		}

// 		obj.Application_id = generateUuid()
// 	}

// 	if err := validateUnsafeGenericEntityNoPK(dbq); err != nil {
// 		return err
// 	}

// 	if dbq.dbConnection == nil {
// 		return fmt.Errorf("database connection is nil")
// 	}

// 	if !dbq.allowUnsafe {
// 		return fmt.Errorf("unsafe operation is not allowed in this context")
// 	}

// 	if isEmpty(obj.Engine_instance_inst_id) {
// 		return fmt.Errorf("application's engine instance id field should not be empty")

// 	}

// 	if isEmpty(obj.Managed_environment_id) {
// 		return fmt.Errorf("application's environment id field should not be empty")

// 	}

// 	if isEmpty(obj.Spec_field) {
// 		return fmt.Errorf("application's spec field should not be empty")

// 	}

// 	if isEmpty(obj.Name) {
// 		return fmt.Errorf("application's name field should not be empty")
// 	}

// 	result, err := dbq.dbConnection.Model(obj).Insert()
// 	if err != nil {
// 		return fmt.Errorf("error on inserting application %v", err)
// 	}

// 	if result.RowsAffected() != 1 {
// 		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
// 	}

// 	return nil

// }

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllApplications() ([]Application, error) {
	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return nil, fmt.Errorf("unsafe call to ListAllApplications")
	}

	var applications []Application
	err := dbq.dbConnection.Model(&applications).Select()

	if err != nil {
		return nil, err
	}

	return applications, nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteApplicationById(id string) (int, error) {

	if err := validateUnsafeGenericEntity(id, dbq); err != nil {
		return 0, err
	}

	result := &Application{
		Application_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting application: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
