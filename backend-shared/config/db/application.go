package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) CheckedGetApplicationById(ctx context.Context, application *Application, ownerId string) error {

	if err := validateQueryParamsEntity(application, dbq); err != nil {
		return err
	}

	if IsEmpty(application.Application_id) {
		return fmt.Errorf("application_Id is nil in GetApplicationById")
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
			return NewAccessDeniedError(fmt.Sprintf("No cluster access exists for application '%s'", application.Application_id))
		}
		return err
	}

	*application = applicationResult

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) GetApplicationById(ctx context.Context, application *Application) error {

	if err := validateQueryParamsEntity(application, dbq); err != nil {
		return err
	}

	if IsEmpty(application.Application_id) {
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

func (dbq *PostgreSQLDatabaseQueries) CheckedCreateApplication(ctx context.Context, obj *Application, ownerId string) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if IsEmpty(obj.Application_id) {
			obj.Application_id = generateUuid()
		}
	} else {
		if !IsEmpty(obj.Application_id) {
			return fmt.Errorf("primary key should be empty")
		}

		obj.Application_id = generateUuid()
	}

	if err := isEmptyValues("CreateApplication",
		"Engine_instance_inst_id", obj.Engine_instance_inst_id,
		"Managed_environment_id", obj.Managed_environment_id,
		"Spec_field", obj.Spec_field,
		"Name", obj.Name); err != nil {
		return err
	}

	// Verify the user can access the managed environment
	managedEnv := ManagedEnvironment{Managedenvironment_id: obj.Managed_environment_id}
	if err := dbq.CheckedGetManagedEnvironmentById(ctx, &managedEnv, ownerId); err != nil {
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

func (dbq *PostgreSQLDatabaseQueries) CheckedDeleteApplicationById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	result := &Application{
		Application_id: id,
	}

	if err := dbq.CheckedGetApplicationById(ctx, result, ownerId); err != nil {
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

func (dbq *PostgreSQLDatabaseQueries) DeleteApplicationById(ctx context.Context, id string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
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

func (dbq *PostgreSQLDatabaseQueries) CreateApplication(ctx context.Context, obj *Application) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if dbq.allowTestUuids {
		if IsEmpty(obj.Application_id) {
			obj.Application_id = generateUuid()
		}
	} else {
		if !IsEmpty(obj.Application_id) {
			return fmt.Errorf("primary key should be empty")
		}
		obj.Application_id = generateUuid()
	}

	if err := isEmptyValues("CreateApplication",
		"Engine_instance_inst_id", obj.Engine_instance_inst_id,
		"Managed_environment_id", obj.Managed_environment_id,
		"Spec_field", obj.Spec_field,
		"Name", obj.Name); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
		return err
	}

	result, err := dbq.dbConnection.Model(obj).Context(ctx).Insert()
	if err != nil {
		return fmt.Errorf("error on inserting application %v", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("unexpected number of rows affected: %d", result.RowsAffected())
	}
	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UpdateApplication(ctx context.Context, obj *Application) error {

	if err := validateQueryParamsEntity(obj, dbq); err != nil {
		return err
	}

	if err := isEmptyValues("UpdateApplication",
		"Application_id", obj.Application_id,
		"Engine_instance_inst_id", obj.Engine_instance_inst_id,
		"Managed_environment_id", obj.Managed_environment_id,
		"Spec_field", obj.Spec_field,
		"Name", obj.Name); err != nil {
		return err
	}

	if err := validateFieldLength(obj); err != nil {
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

// Get applications in a batch. Batch size defined by 'limit' and starting point of batch is defined by 'offSet'.
// For example if you want applications starting from 51-150 then set the limit to 100 and offset to 50.
func (dbq *PostgreSQLDatabaseQueries) GetApplicationBatch(ctx context.Context, applications *[]Application, limit, offSet int) error {
	err := dbq.dbConnection.
		Model(applications).
		Order("seq_id ASC").
		Limit(limit).   // Batch size
		Offset(offSet). // offset+1 is starting point of batch
		Context(ctx).
		Select()

	if err != nil {
		return err
	}
	return nil
}
