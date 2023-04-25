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

func LogAPIResourceChangeEvent(resourceNamespace string, resourceName string, resource any, resourceChangeType ResourceChangeType, log logr.Logger) {
	log = log.WithValues("audit", "true")

	if resource == nil {
		log.Error(nil, "resource passed to LogAPIResourceChangeEvent was nil")
		return
	}
	secret, isSecret := (resource).(*corev1.Secret)
	if isSecret {
		// Make a copy of the resource, before we modify it
		secret = secret.DeepCopy()
		// Remove the data field, as this contains data that should not be logged.
		secret.Data = map[string][]byte{}
		resource = secret
	}
	jsonRepresentation, err := json.Marshal(resource)

	if err != nil {
		log.Error(err, "SEVERE: Unable to marshal log to JSON.")
		return
	}

	log.Info(fmt.Sprintf("API Resource changed: %s", string(resourceChangeType)), "namespace",
		resourceNamespace, "name", resourceName, "object", string(jsonRepresentation))
}
