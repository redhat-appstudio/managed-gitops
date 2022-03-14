package db

import (
	"strings"
	"unicode/utf8"
)

// These values should be equiv. with their related VARCHAR values from 'db-schema.sql' respectively
const (
	ClusterCredentialsClustercredentialsCredId                        = 48
	ClusterCredentialsHost                                            = 512
	ClusterCredentialsKubeConfig                                      = 65000
	ClusterCredentialsKubeConfigContext                               = 64
	ClusterCredentialsServiceaccountBearerToken                       = 128
	ClusterCredentialsServiceaccountNs                                = 128
	GitopsEngineClusterGitopsengineclusterId                          = 48
	GitopsEngineInstanceGitopsengineinstanceId                        = 48
	GitopsEngineInstanceNamespaceName                                 = 48
	GitopsEngineInstanceNamespaceUid                                  = 48
	GitopsEngineInstanceEngineclusterId                               = 48
	ManagedEnvironmentManagedenvironmentId                            = 48
	ManagedEnvironmentName                                            = 256
	ManagedEnvironmentClustercredentialsId                            = 48
	ClusterUserClusteruserId                                          = 48
	ClusterUserUserName                                               = 256
	ClusterAccessClusteraccessUserId                                  = 48
	ClusterAccessClusteraccessManagedEnvironmentId                    = 48
	ClusterAccessClusteraccessGitopsEngineInstanceId                  = 48
	OperationOperationId                                              = 48
	OperationInstanceId                                               = 48
	OperationResourceId                                               = 48
	OperationOperationOwnerUserId                                     = 48
	OperationResourceType                                             = 32
	OperationState                                                    = 30
	OperationHumanReadableState                                       = 1024
	ApplicationApplicationId                                          = 48
	ApplicationName                                                   = 256
	ApplicationSpecField                                              = 16384
	ApplicationEngineInstanceInstId                                   = 48
	ApplicationManagedEnvironmentId                                   = 48
	ApplicationStateApplicationstateApplicationId                     = 48
	ApplicationStateHealth                                            = 30
	ApplicationStateMessage                                           = 1024
	ApplicationStateRevision                                          = 1024
	ApplicationStateSyncStatus                                        = 30
	DeploymentToApplicationMappingDeploymenttoapplicationmappingUidId = 48
	DeploymentToApplicationMappingName                                = 256
	DeploymentToApplicationMappingNamespace                           = 96
	DeploymentToApplicationMappingWorkspaceUid                        = 48
	DeploymentToApplicationMappingApplicationId                       = 48
	KubernetesToDBResourceMappingKubernetesResourceType               = 64
	KubernetesToDBResourceMappingKubernetesResourceUid                = 64
	KubernetesToDBResourceMappingDbRelationType                       = 64
	KubernetesToDBResourceMappingDbRelationKey                        = 64
	APICRToDatabaseMappingApiResourceType                             = 64
	APICRToDatabaseMappingApiResourceUid                              = 64
	APICRToDatabaseMappingApiResourceName                             = 256
	APICRToDatabaseMappingApiResourceNamespace                        = 256
	APICRToDatabaseMappingApiResourceWorkspaceUid                     = 64
	APICRToDatabaseMappingDbRelationType                              = 32
	APICRToDatabaseMappingDbRelationKey                               = 64
	SyncOperationSyncoperationId                                      = 48
	SyncOperationApplicationId                                        = 48
	SyncOperationOperationId                                          = 48
	SyncOperationDeploymentName                                       = 256
	SyncOperationRevision                                             = 256
	SyncOperationDesiredState                                         = 16
)

// TruncateVarchar converts string to "str..." if chars is > maxLength
// returns a relative number of dots '.' string if maxLength <= 3
// returns empty string if maxLength < 0 or if string is not UTF-8 encoded
// Notice: This is based on characters -- not bytes (default VARCHAR behavior)
func TruncateVarchar(s string, maxLength int) string {
	if maxLength <= 3 && maxLength >= 0 {
		return strings.Repeat(".", maxLength)
	}

	if maxLength < 0 || !utf8.ValidString(s) {
		return ""
	}

	var wb = strings.Split(s, "")

	if maxLength < len(wb) {
		maxLength = maxLength - 3
		return strings.Join(wb[:maxLength], "") + "..."
	}

	return s
}
