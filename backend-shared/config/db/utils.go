package db

import (
	"fmt"
	"runtime/debug"
)

// validateGenericEntity is common, simple validation logic shared by most entities
func validateGenericEntity(entityId string, dbq *PostgreSQLDatabaseQueries) error {
	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if isEmpty(entityId) {
		debug.PrintStack()
		return fmt.Errorf("primary key is empty")
	}

	return nil
}

// validateGenericEntity is common, simple validation logic shared by most entities
func validateUnsafeGenericEntity(entityId string, dbq *PostgreSQLDatabaseQueries) error {

	if err := validateGenericEntity(entityId, dbq); err != nil {
		return err
	}

	if !dbq.allowUnsafe {
		return fmt.Errorf("unsafe operation is not allowed in this context")
	}

	return nil
}

// validateGenericEntity is common, simple validation logic shared by most entities
func validateUnsafeGenericEntityNoPK(dbq *PostgreSQLDatabaseQueries) error {

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return fmt.Errorf("unsafe operation is not allowed in this context")
	}

	return nil
}
