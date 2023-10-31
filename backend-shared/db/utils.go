package db

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// isEmptyValues returns an error if at least one of the parameters is nil or empty.
// The returned error string indicates which parameter was empty, plus the calling function.
//
// This function can be used as a generic check of empty or nil values, in order to reduce
// the amount of boilerplate code.
//
// See functions that are calling this one for examples.
func isEmptyValues(callLocation string, params ...any) error {

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

	if IsEmpty(entityId) {
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
func validateQueryParamsEntity(entity any, dbq *PostgreSQLDatabaseQueries) error {
	if dbq.dbConnection == nil {
		return fmt.Errorf("database connection is nil")
	}

	if entity == nil {
		return fmt.Errorf("query parameter value is nil")
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

func (e *APICRToDatabaseMapping) ShortString() string {

	res := ""
	res += "name: " + e.APIResourceName + ", "
	res += "namespace: " + e.APIResourceNamespace + ", "
	res += "resource-type: : " + string(e.APIResourceType) + ", "
	res += "namespace-uid: " + e.NamespaceUID + ", "
	res += "db-relation-key: " + e.DBRelationKey + ", "
	res += "db-relation-type: " + string(e.DBRelationType)

	return res
}

func (o *Operation) ShortString() string {
	res := ""
	res += "operation-id: " + o.Operation_id + ", "
	res += "instance-id: " + o.Instance_id + ", "
	res += "owner: " + o.Operation_owner_user_id + ", "
	res += "resource: " + o.Resource_id + ", "
	res += "resource-type: " + string(o.Resource_type) + ", "
	return res
}

func (o *Operation) LongString() string {
	res := ""
	res += "instance-id: " + o.Instance_id + ", "
	res += "operation-id: " + o.Operation_id + ", "
	res += "owner: " + o.Operation_owner_user_id + ", "
	res += "resource: " + o.Resource_id + ", "
	res += "resource-type: " + string(o.Resource_type) + ", "

	res += "human-readable-state: " + o.Human_readable_state + ", "
	res += "state: " + string(o.State) + ", "
	res += fmt.Sprintf("last-status-update: %v", o.Last_state_update) + ", "
	res += fmt.Sprintf("created_on: %v", o.Last_state_update)

	return res
}

func generateUuid() string {
	return uuid.New().String()
}

func IsEmpty(str string) bool {
	return len(strings.TrimSpace(str)) == 0
}

func ConvertSnakeCaseToCamelCase(fieldName string) string {
	splitFieldName := strings.Split(fieldName, "_")
	var fieldNameInCamelCase string

	for i := 0; i < len(splitFieldName); i++ {
		if splitFieldName[i] == "id" || splitFieldName[i] == "uid" || splitFieldName[i] == "url" {
			fieldNameInCamelCase += strings.ToUpper(splitFieldName[i])
		} else {
			fieldNameInCamelCase += cases.Title(language.English).String(splitFieldName[i])
		}
	}

	return fieldNameInCamelCase
}

// A generic function to validate length of string values in input provided by users.
// The max length of string is checked using constant variables defined for each type and field in db_field_constants.go
func validateFieldLength(obj any) error {
	valuesOfObject := reflect.ValueOf(obj).Elem()
	typeOfObject := reflect.TypeOf(obj).Elem().Name()

	// Iterate through each field present in object
	for i := 0; i < valuesOfObject.NumField(); i++ {
		fieldName := valuesOfObject.Type().Field(i).Name
		fieldValue := valuesOfObject.FieldByName(fieldName)
		fieldType := fieldValue.Type().Name()

		if fieldType != "string" {
			continue
		}
		// Format object type and field name according to constants defined in db_field_constants.go
		maximumSize := getConstantValue(ConvertSnakeCaseToCamelCase(typeOfObject + "_" + fieldName + "_Length"))

		if len(fieldValue.String()) > maximumSize {
			return fmt.Errorf("%v value exceeds maximum size: max: %d, actual: %d", fieldName, maximumSize, len(fieldValue.String()))
		}
	}
	return nil
}

func IsMaxLengthError(err error) bool {
	if err != nil {
		return strings.Contains(err.Error(), "value exceeds maximum size")
	}
	return false
}
