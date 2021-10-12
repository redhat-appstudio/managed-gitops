package db

import "fmt"

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

	if dbq.dbConnection == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return 0, fmt.Errorf("unsafe delete is not allowed")
	}

	if isEmpty(id) {
		return 0, fmt.Errorf("primary key is empty")
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
