package util

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

// Variables to be used for ADR6 logging.
const (
	LogLogger_managed_gitops = "managed-gitops"
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

const (
	logError              = "resource passed to LogAPIResourceChangeEvent was nil"
	logMarshalJsonError   = "SEVERE: Unable to marshal log to JSON."
	logUnMarshalJsonError = "SEVERE: Unable to unmarshal JSON."
)

func LogAPIResourceChangeEvent(resourceNamespace string, resourceName string, resource any, resourceChangeType ResourceChangeType, log logr.Logger) {
	log = log.WithValues("audit", "true").WithCallDepth(1)

	if resource == nil {
		log.Error(nil, logError)
		return
	}
	_, isSecretPointer := (resource).(*corev1.Secret)
	_, isSecretObj := (resource).(corev1.Secret)
	if isSecretPointer || isSecretObj {
		log.Info(fmt.Sprintf("API Resource changed for secret resource: %s, name: %s, namespace: %s", string(resourceChangeType), resourceName, resourceNamespace))
		return
	}

	jsonRepresentation, err := json.Marshal(resource)
	if err != nil {
		log.Error(err, logMarshalJsonError)
		return
	}
	var resourceMap map[string]interface{}
	errUnmarshal := json.Unmarshal(jsonRepresentation, &resourceMap)
	if errUnmarshal != nil {
		log.Error(errUnmarshal, logUnMarshalJsonError)
		return
	}
	mf := resourceMap["metadata"].(map[string]interface{})
	if mf != nil && mf["managedFields"] != nil {
		mf["managedFields"] = nil
		modifiedJsonRep, err := json.Marshal(resourceMap)
		if err != nil {
			log.Error(err, "SEVERE: Unable to marshal resource to JSON.")
		}
		jsonRepresentation = modifiedJsonRep
	}

	log.Info(fmt.Sprintf("API Resource changed: %s", string(resourceChangeType)), "namespace",
		resourceNamespace, "name", resourceName, "object", string(jsonRepresentation))

}
