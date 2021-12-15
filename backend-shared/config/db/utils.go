package db

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// isEmptyValues returns an error if at least one of the parameters is nil or empty.
// The returned error string indicates which parameter was empty, plus the calling function.
//
// This function can be used as a generic check of empty or nil values, in order to reduce
// the amount of boilerplate code.
//
// See functions that are calling this one for examples.
func isEmptyValues(callLocation string, params ...interface{}) error {

	if len(params)%2 == 1 {
		return fmt.Errorf("invalid number of parameters, expected an even number: %v", len(params))
	}

	if len(params) == 0 {
		return fmt.Errorf("invalid number of parameters, at least 2 expected")
	}

	x := 0
	for {

		fieldNameParam := params[x]

		if fieldNameParam == nil || fieldNameParam == "" {
			return fmt.Errorf("field name in position %d was empty, in %v", x, callLocation)
		}

		fieldName, isString := fieldNameParam.(string)
		if !isString {
			return fmt.Errorf("field name in position %d is not a string, in %v", x, callLocation)
		}

		value := params[x+1]
		if value == nil {
			return fmt.Errorf("%v field should not be nil, in %v", fieldName, callLocation)

		} else if valueStr, isString := value.(string); isString && len(strings.TrimSpace(valueStr)) == 0 {
			return fmt.Errorf("%v field should not be empty string, in %v", fieldName, callLocation)
		}

		x += 2
		if x >= len(params) {
			break
		}
	}

	return nil

}

// validateQueryParams is common, simple validation logic shared by most entities
func validateQueryParams(entityId string, dbq *PostgreSQLDatabaseQueries) error {
	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if isEmpty(entityId) {
		debug.PrintStack()
		return fmt.Errorf("primary key is empty")
	}

	return nil
}

// validateUnsafeQueryParams is common, simple validation logic shared by most entities
func validateUnsafeQueryParams(entityId string, dbq *PostgreSQLDatabaseQueries) error {

	if err := validateQueryParams(entityId, dbq); err != nil {
		return err
	}

	if !dbq.allowUnsafe {
		return fmt.Errorf("unsafe operation is not allowed in this context")
	}

	return nil
}

// validateQueryParams is common, simple validation logic shared by most entities
func validateQueryParamsEntity(entity interface{}, dbq *PostgreSQLDatabaseQueries) error {
	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if entity == nil {
		return fmt.Errorf("query parameter value is nil")
	}

	return nil
}

// validateUnsafeQueryParams is common, simple validation logic shared by most entities
func validateUnsafeQueryParamsEntity(entity interface{}, dbq *PostgreSQLDatabaseQueries) error {

	if err := validateQueryParamsEntity(entity, dbq); err != nil {
		return err
	}

	if !dbq.allowUnsafe {
		return fmt.Errorf("unsafe operation is not allowed in this context")
	}

	return nil
}

// validateGenericEntity is common, simple validation logic shared by most entities
func validateUnsafeQueryParamsNoPK(dbq *PostgreSQLDatabaseQueries) error {

	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if !dbq.allowUnsafe {
		return fmt.Errorf("unsafe operation is not allowed in this context")
	}

	return nil
}

// validateQueryParams is common, simple validation logic shared by most entities
func validateQueryParamsNoPK(dbq *PostgreSQLDatabaseQueries) error {
	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	return nil
}

func (o *Operation) ShortString() string {
	res := ""
	res += "operation-id: " + o.Operation_id + ", "
	res += "instance-id: " + o.Instance_id + ", "
	res += "owner: " + o.Operation_owner_user_id + ", "
	res += "resource: " + o.Resource_id + ", "
	res += "resource-type: " + o.Resource_type + ", "
	return res
}

func (o *Operation) LongString() string {
	res := ""
	res += "instance-id: " + o.Instance_id + ", "
	res += "operation-id: " + o.Operation_id + ", "
	res += "owner: " + o.Operation_owner_user_id + ", "
	res += "resource: " + o.Resource_id + ", "
	res += "resource-type: " + o.Resource_type + ", "

	res += "human-readable-state: " + o.Human_readable_state + ", "
	res += "state: " + o.State + ", "
	res += fmt.Sprintf("last-status-update: %v", o.Last_state_update) + ", "
	res += fmt.Sprintf("created_on: %v", o.Last_state_update)

	return res
}
