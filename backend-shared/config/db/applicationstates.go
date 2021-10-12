package db

import "fmt"

func (dbq *PostgreSQLDatabaseQueries) UnsafeListAllApplicationStates() ([]ApplicationState, error) {
	if dbq.dbConnection == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return nil, fmt.Errorf("unsafe call to ListAllApplicationStates")
	}

	var appStates []ApplicationState
	err := dbq.dbConnection.Model(&appStates).Select()

	if err != nil {
		return nil, err
	}

	return appStates, nil
}

func (dbq *PostgreSQLDatabaseQueries) DeleteApplicationStateById(id string) (int, error) {

	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return 0, fmt.Errorf("unsafe delete is not allowed")
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
	}

	result := &ApplicationState{
		Applicationstate_application_id: id,
	}

	deleteResult, err := dbq.dbConnection.Model(result).WherePK().Delete()
	if err != nil {
		return 0, fmt.Errorf("error on deleting application state: %v", err)
	}

	return deleteResult.RowsAffected(), nil
}
