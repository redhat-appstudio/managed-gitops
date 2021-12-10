package db

import (
	"context"
	"fmt"
)

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllApplicationStates(ctx context.Context, applicationStates *[]ApplicationState) error {

	if err := validateUnsafeQueryParamsNoPK(dbq); err != nil {
		return err
	}

	if err := dbq.dbConnection.Model(applicationStates).Context(ctx).Select(); err != nil {
		return err
	}

	return nil
}

func (dbq *PostgreSQLDatabaseQueries) UncheckedDeleteApplicationStateById(ctx context.Context, id string) (int, error) {

	if err := validateQueryParams(id, dbq); err != nil {
		return 0, err
	}

	result := &ApplicationState{
		Applicationstate_application_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Context(ctx).Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting application state: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
