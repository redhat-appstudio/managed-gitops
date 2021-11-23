package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) GetApplicationById(ctx context.Context, application *Application, ownerId string) error {

	if err := validateQueryParamsEntity(application, dbq); err != nil {
		return err
	}

	if isEmpty(application.Application_id) {
		return fmt.Errorf("application_Id is nil")
	}

	var applicationResult Application
	{
		var results []Application

		if err := dbq.dbConnection.Model(&results).
			Where("application_id = ?", application.Application_id).
			Context(ctx).
			Select(); err != nil {

			return fmt.Errorf("error on retrieving Application: %v", err)
		}

		if len(results) == 0 {
			return NewResultNotFoundError(fmt.Sprintf("Application '%s'", application.Application_id))
		}

		if len(results) > 1 {
			return fmt.Errorf("multiple results found on retrieving Application: %v", application.Application_id)
		}

		applicationResult = results[0]
	}

	// Ensure there is a cluster access for this user, and the application's managed env and engine instance
	if err := dbq.GetClusterAccessByPrimaryKey(ctx,
		&ClusterAccess{Clusteraccess_user_id: ownerId,
			Clusteraccess_managed_environment_id:    applicationResult.Managed_environment_id,
			Clusteraccess_gitops_engine_instance_id: applicationResult.Engine_instance_inst_id}); err != nil {

		if IsResultNotFoundError(err) {
			return NewResultNotFoundError(fmt.Sprintf("No cluster access exists for application '%s'", application.Application_id))
		}
		return err
	}

	*application = applicationResult

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UnsafeGetApplicationById(ctx context.Context, application *Application) error {

	if err := validateUnsafeQueryParamsEntity(application, dbq); err != nil {
		return err
	}

	if isEmpty(application.Application_id) {
		return fmt.Errorf("application_Id is nil")
	}

	var results []Application

	if err := dbq.dbConnection.Model(&results).
		Where("application_id = ?", application.Application_id).
		Context(ctx).
		Select(); err != nil {

		return fmt.Errorf("error on retrieving Application: %v", err)
	}

	if len(results) == 0 {
		return NewResultNotFoundError(fmt.Sprintf("Application '%s'", application.Application_id))
	}

	if len(results) > 1 {
		return fmt.Errorf("multiple results found on retrieving Application: %v", application.Application_id)
	}

	*application = results[0]

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) CreateApplication(ctx context.Context, obj *Application, ownerId string) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

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
	managedEnv := ManagedEnvironment{Managedenvironment_id: obj.Managed_environment_id}
	if err := dbq.GetManagedEnvironmentById(ctx, &managedEnv, ownerId); err != nil {
		return fmt.Errorf("on creating Application, unable to retrieve managed environment %s for user %s: %v", obj.Managed_environment_id, ownerId, err)
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting application: %v", err)
	}

	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}

	return nil

}

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllApplications(ctx context.Context, applications *[]Application) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	err := dbq.dbConnection.Model(applications).Context(ctx).Select()

	if err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteApplicationById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	result := &Application{
		Application_id: id,
	}

	if err := dbq.GetApplicationById(ctx, result, ownerId); err != nil {
		if IsResultNotFoundError(err) {
			return 0, nil
		}

		return 0, err
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting application: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}

func (dbq *PostgreSQLDatabaseQueries) UnsafeDeleteApplicationById(ctx context.Context, id string) (int, error) {

	if err := validateUnsafeQueryParams(id, dbq); err != nil {
		return 0, err
	}

	result := &Application{
		Application_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting application: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
