package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	DbSchemaRelativeFileLocation         = "../../db-schema.sql"
	DbFieldConstantsRelativeFileLocation = "../config/db/db_field_constants.go"
)

func main() {

	fieldToSize := parseDbSchema()
	fieldConstantToSize := parseDbConstants()
	checkIfSchemaInSyncWithConstants(fieldConstantToSize, fieldToSize)

}

func checkIfSchemaInSyncWithConstants(fieldConstantToSize map[string]string, fieldToSize map[string]string) {

	for fieldName, fieldSize := range fieldConstantToSize {
		if _, ok := fieldToSize[fieldName]; !ok {
			err := fmt.Errorf("field %s not present in the schema", fieldName)
			fmt.Println(err)
			os.Exit(1)
		}

		if fieldToSize[fieldName] != fieldSize {
			err := fmt.Errorf("sizes for the field %s are not in sync", fieldName)
			fmt.Println(err)
			os.Exit(1)
		}

	}

	for fieldName, fieldSize := range fieldToSize {

		if _, ok := fieldConstantToSize[fieldName]; !ok {

			err := fmt.Errorf("field %s not present as a constant in the db_field_constants.go file", fieldName)
			fmt.Println(err)
			os.Exit(1)
		}

		if fieldConstantToSize[fieldName] != fieldSize {

			err := fmt.Errorf("sizes for the field %s are not in sync", fieldName)
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

func parseDbSchema() map[string]string {

	fieldToSize := make(map[string]string)
	dbSchemaFile, err := os.ReadFile(DbSchemaRelativeFileLocation)
	
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	dbSchema := strings.Split(string(dbSchemaFile), "\n")

	i := 0
	for i < len(dbSchema) {

		//process the fields of each table individually by identifying the beginning of the creation
		//of a table with the substring "CREATE TABLE" and the end of a table with ");"
		if strings.Contains(dbSchema[i], "CREATE TABLE") {

			tableName := strings.Split(dbSchema[i], " ")[2]
			//process the current table till a ");" is encountered which marks the end of the table's definition
			for !(strings.Contains(dbSchema[i], ");")) {

				//check if the current string contains the type as VARCHAR, and extract the name of the field and size of the field
				//while omitting comments
				if strings.Contains(dbSchema[i], "VARCHAR") && !(strings.Contains(dbSchema[i], "--")) {

					spaces := regexp.MustCompile(`\s+`)
					//remove all leading white spaces
					fieldName := spaces.ReplaceAllString(strings.Split(dbSchema[i], " ")[0], "")
					fieldNameInCamelCase := convertSnakeCaseToCamelCase(fieldName)
					currentField := strings.ReplaceAll(dbSchema[i], " ", "")
					//extract size of the VARCHAR field as a substring
					StartIndexOfSize := strings.Index(currentField, "(")
					endIndexOfSize := strings.Index(currentField, ")")
					size := currentField[StartIndexOfSize+1 : endIndexOfSize]
					uniqueFieldName := tableName + fieldNameInCamelCase
					fieldToSize[uniqueFieldName] = size
				}
				i++
			}
		}
		i++
	}
	return fieldToSize
}

func parseDbConstants() map[string]string {

	fieldConstantToSize := make(map[string]string)
	dbFieldConstantsFile, err := os.ReadFile(DbFieldConstantsRelativeFileLocation)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	dbFieldConstants := strings.Split(string(dbFieldConstantsFile), "\n")

	i := 0
	for !(strings.Contains(dbFieldConstants[i], "const")) {

		i++
	}
	i++
	for !(strings.Contains(dbFieldConstants[i], ")")) {

		spaces := regexp.MustCompile(`\s+`)
		//remove all white spaces for the current field
		dbFieldConstants[i] = spaces.ReplaceAllString(dbFieldConstants[i], "")
		fieldDetails := strings.Split(dbFieldConstants[i], "=")
		fieldName := fieldDetails[0]
		fieldSize := fieldDetails[1]
		fieldConstantToSize[fieldName] = fieldSize
		i++
	}
	return fieldConstantToSize
}

func convertSnakeCaseToCamelCase(fieldName string) string {

	splitFieldName := strings.Split(fieldName, "_")
	var fieldNameInCamelCase string
	
	for i := 0; i < len(splitFieldName); i++ {
		fieldNameInCamelCase += strings.Title(splitFieldName[i])
	}

	return fieldNameInCamelCase
}
