package util

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
)

const (
	LogLevel_Debug int = 1

	// LogLevel_Warn is used for unexpected conditions that occur, which might
	// be an actual error, or might just be the normal functioning of the program.
	LogLevel_Warn int = -1
)

type ResourceChangeType string

const (
	ResourceCreated  ResourceChangeType = "Created"
	ResourceModified ResourceChangeType = "Modified"
	ResourceDeleted  ResourceChangeType = "Deleted"
)

func LogAPIResourceChangeEvent(resourceNamespace string, resourceName string, resource interface{}, resourceChangeType ResourceChangeType, log logr.Logger) {

	jsonRepresentation, err := json.Marshal(resource)
	if err != nil {
		fmt.Println("Log Marshal error :", err)
	}

	log.Info(fmt.Sprintf("API Resource changed: %s", string(resourceChangeType)),
		"namespace", resourceNamespace, "name", resourceName, "object", jsonRepresentation)

}
