package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

const (
	DBSchemaRelativeFileLocation         = "../db-schema.sql"
	DBFieldConstantsRelativeFileLocation = "./db/db_field_constants.go"
	minimumExpectedFields                = 50
)

func main() {
	fieldToSize := parseDBSchema(DBSchemaRelativeFileLocation)
	fieldConstantToSize := parseDBConstants(DBFieldConstantsRelativeFileLocation)
	checkIfSchemaInSyncWithConstants(fieldConstantToSize, fieldToSize)
}

func checkIfSchemaInSyncWithConstants(fieldConstantToSize map[string]string, fieldToSize map[string]string) {
	for fieldName, fieldSize := range fieldConstantToSize {
		if _, ok := fieldToSize[fieldName]; !ok {
			exitWithError(fmt.Errorf("field %s not present in the schema", fieldName))
		}

		if fieldToSize[fieldName] != fieldSize {
			exitWithError(fmt.Errorf("sizes for the field %s are not in sync", fieldName))
		}

		if _, ok := db.DbFieldMap[fieldName]; !ok {
			exitWithError(fmt.Errorf("field %s is not present in db.DbFieldMap.", fieldName))
		}
	}

	for fieldName, fieldSize := range fieldToSize {
		if _, ok := fieldConstantToSize[fieldName]; !ok {
			exitWithError(fmt.Errorf("field %s not present as a constant in the db_field_constants.go file", fieldName))
		}

		if fieldConstantToSize[fieldName] != fieldSize {
			exitWithError(fmt.Errorf("sizes for the field %s are not in sync", fieldName))
		}
	}
}

func parseDBSchema(DBSchemaRelativeFileLocation string) map[string]string {
	fieldToSize := make(map[string]string)
	dbSchemaContents, err := os.ReadFile(filepath.Clean(DBSchemaRelativeFileLocation))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	dbSchema := strings.Split(string(dbSchemaContents), "\n")

	i := 0
	for i < len(dbSchema) {
		// process the fields of each table individually by identifying the beginning of the creation
		// of a table with the substring "CREATE TABLE" and the end of a table with ");"
		if strings.Contains(dbSchema[i], "CREATE TABLE") {
			tableName := strings.Split(dbSchema[i], " ")[2]
			// process the current table till a ");" is encountered which marks the end of the table's definition
			for !(strings.Contains(dbSchema[i], ");")) {

				// check if the current string contains the type as VARCHAR, and extract the name of the field and size of the field
				// while omitting comments
				if strings.Contains(dbSchema[i], "VARCHAR") && !(strings.Contains(dbSchema[i], "--")) {
					spaces := regexp.MustCompile(`\s+`)
					// remove all leading white spaces
					var fieldName string
					j := 0
					for {
						fieldName = spaces.ReplaceAllString(strings.Split(dbSchema[i], " ")[j], "")
						if fieldName == "" {
							j++
						} else {
							break
						}
					}
					fieldNameInCamelCase := db.ConvertSnakeCaseToCamelCase(fieldName)
					currentField := strings.ReplaceAll(dbSchema[i], " ", "")
					// extract size of the VARCHAR field as a substring
					StartIndexOfSize := strings.Index(currentField, "(")
					endIndexOfSize := strings.Index(currentField, ")")
					size := currentField[StartIndexOfSize+1 : endIndexOfSize]
					uniqueFieldName := tableName + fieldNameInCamelCase + "Length"
					fieldToSize[uniqueFieldName] = size
				}
				i++
			}
		}
		i++
	}

	if len(fieldToSize) < minimumExpectedFields {
		exitWithError(fmt.Errorf("minimum number of fields i.e., %d not present in the db-schema.sql file", minimumExpectedFields))
	}

	return fieldToSize
}

func parseDBConstants(DBFieldConstantsRelativeFileLocation string) map[string]string {
	fieldConstantToSize := make(map[string]string)
	dbFieldConstantsContents, err := os.ReadFile(filepath.Clean(DBFieldConstantsRelativeFileLocation))
	if err != nil {
		exitWithError(err)
	}

	dbFieldConstants := strings.Split(string(dbFieldConstantsContents), "\n")

	i := 0
	for !(strings.Contains(dbFieldConstants[i], "const")) {
		i++
	}
	i++
	for !(strings.Contains(dbFieldConstants[i], ")")) {
		spaces := regexp.MustCompile(`\s+`)
		// remove all white spaces for the current field
		dbFieldConstants[i] = spaces.ReplaceAllString(dbFieldConstants[i], "")
		fieldDetails := strings.Split(dbFieldConstants[i], "=")
		fieldName := fieldDetails[0]
		fieldSize := fieldDetails[1]
		fieldConstantToSize[fieldName] = fieldSize
		i++
	}
	if len(fieldConstantToSize) < minimumExpectedFields {
		exitWithError(fmt.Errorf("minimum number of fields i.e., %d not present as constants in the db_field_constants.go file", minimumExpectedFields))
	}

	return fieldConstantToSize
}

func exitWithError(err error) {
	fmt.Println(err)
	os.Exit(1)
}
