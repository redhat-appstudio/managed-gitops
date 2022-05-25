package db

import (
	"strings"
	"unicode/utf8"
)

// These values should be equiv. with their related VARCHAR values from 'db-schema.sql' respectively
const (
	ClusterCredentialsClustercredentialsCredIDLength                        = 48
	ClusterCredentialsHostLength                                            = 512
	ClusterCredentialsKubeConfigLength                                      = 65000
	ClusterCredentialsKubeConfigContextLength                               = 64
	ClusterCredentialsServiceaccountBearerTokenLength                       = 128
	ClusterCredentialsServiceaccountNsLength                                = 128
	GitopsEngineClusterGitopsengineclusterIDLength                          = 48
	GitopsEngineInstanceGitopsengineinstanceIDLength                        = 48
	GitopsEngineInstanceNamespaceNameLength                                 = 48
	GitopsEngineInstanceNamespaceUIDLength                                  = 48
	GitopsEngineClusterClustercredentialsIDLength                           = 48
	GitopsEngineInstanceEngineclusterIDLength                               = 48
	ManagedEnvironmentManagedenvironmentIDLength                            = 48
	ManagedEnvironmentNameLength                                            = 256
	ManagedEnvironmentClustercredentialsIDLength                            = 48
	ClusterUserClusteruserIDLength                                          = 48
	ClusterUserUserNameLength                                               = 256
	ClusterAccessClusteraccessUserIDLength                                  = 48
	ClusterAccessClusteraccessManagedEnvironmentIDLength                    = 48
	ClusterAccessClusteraccessGitopsEngineInstanceIDLength                  = 48
	OperationOperationIDLength                                              = 48
	OperationInstanceIDLength                                               = 48
	OperationResourceIDLength                                               = 48
	OperationOperationOwnerUserIDLength                                     = 48
	OperationResourceTypeLength                                             = 32
	OperationStateLength                                                    = 30
	OperationHumanReadableStateLength                                       = 1024
	ApplicationApplicationIDLength                                          = 48
	ApplicationNameLength                                                   = 256
	ApplicationSpecFieldLength                                              = 16384
	ApplicationEngineInstanceInstIDLength                                   = 48
	ApplicationManagedEnvironmentIDLength                                   = 48
	ApplicationStateApplicationstateApplicationIDLength                     = 48
	ApplicationStateHealthLength                                            = 30
	ApplicationStateMessageLength                                           = 1024
	ApplicationStateRevisionLength                                          = 1024
	ApplicationStateSyncStatusLength                                        = 30
	DeploymentToApplicationMappingDeploymenttoapplicationmappingUIDIDLength = 48
	DeploymentToApplicationMappingNameLength                                = 256
	DeploymentToApplicationMappingNamespaceLength                           = 96
	DeploymentToApplicationMappingWorkspaceUIDLength                        = 48
	DeploymentToApplicationMappingApplicationIDLength                       = 48
	KubernetesToDBResourceMappingKubernetesResourceTypeLength               = 64
	KubernetesToDBResourceMappingKubernetesResourceUIDLength                = 64
	KubernetesToDBResourceMappingDbRelationTypeLength                       = 64
	KubernetesToDBResourceMappingDbRelationKeyLength                        = 64
	APICRToDatabaseMappingApiResourceTypeLength                             = 64
	APICRToDatabaseMappingApiResourceUIDLength                              = 64
	APICRToDatabaseMappingApiResourceNameLength                             = 256
	APICRToDatabaseMappingApiResourceNamespaceLength                        = 256
	APICRToDatabaseMappingApiResourceWorkspaceUIDLength                     = 64
	APICRToDatabaseMappingDbRelationTypeLength                              = 32
	APICRToDatabaseMappingDbRelationKeyLength                               = 64
	SyncOperationSyncoperationIDLength                                      = 48
	SyncOperationApplicationIDLength                                        = 48
	SyncOperationOperationIDLength                                          = 48
	SyncOperationDeploymentNameLength                                       = 256
	SyncOperationRevisionLength                                             = 256
	SyncOperationDesiredStateLength                                         = 16
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

// A Map object to be used by utils.ValidateFieldLength function.( Map[<Constant Variable Name as String>] <Constant Variable> )
// At run time we need to check if number of characters in user input are within the max limit defined by constant variables above.
// Because constant variable names (Ex: ClusterUserUserNameLength) are not same as field names (Ex: ClusterUser.User_Name) of object,
// we need to format field name to get constant variable name we need (i.e <ObjectType><FieldName><Length>),
// but issue here that after formating we will get constant variable name as a String, and golang does not support eval() functionality similar to Node, Python etc,
// we need to create a Map[<Constant Variable Name as String>] <Constant Variable> object.
var DbFieldMap = map[string]int{
	"ClusterCredentialsClustercredentialsCredIDLength":                        ClusterCredentialsClustercredentialsCredIDLength,
	"ClusterCredentialsHostLength":                                            ClusterCredentialsHostLength,
	"ClusterCredentialsKubeConfigLength":                                      ClusterCredentialsKubeConfigLength,
	"ClusterCredentialsKubeConfigContextLength":                               ClusterCredentialsKubeConfigContextLength,
	"ClusterCredentialsServiceaccountBearerTokenLength":                       ClusterCredentialsServiceaccountBearerTokenLength,
	"ClusterCredentialsServiceaccountNsLength":                                ClusterCredentialsServiceaccountNsLength,
	"GitopsEngineClusterGitopsengineclusterIDLength":                          GitopsEngineClusterGitopsengineclusterIDLength,
	"GitopsEngineInstanceGitopsengineinstanceIDLength":                        GitopsEngineInstanceGitopsengineinstanceIDLength,
	"GitopsEngineInstanceNamespaceNameLength":                                 GitopsEngineInstanceNamespaceNameLength,
	"GitopsEngineInstanceNamespaceUIDLength":                                  GitopsEngineInstanceNamespaceUIDLength,
	"GitopsEngineClusterClustercredentialsIDLength":                           GitopsEngineClusterClustercredentialsIDLength,
	"GitopsEngineInstanceEngineclusterIDLength":                               GitopsEngineInstanceEngineclusterIDLength,
	"GitopsEngineInstanceEngineClusterIDLength":                               GitopsEngineInstanceEngineclusterIDLength,
	"ManagedEnvironmentManagedenvironmentIDLength":                            ManagedEnvironmentManagedenvironmentIDLength,
	"ManagedEnvironmentNameLength":                                            ManagedEnvironmentNameLength,
	"ManagedEnvironmentClustercredentialsIDLength":                            ManagedEnvironmentClustercredentialsIDLength,
	"ClusterUserClusteruserIDLength":                                          ClusterUserClusteruserIDLength,
	"ClusterUserUserNameLength":                                               ClusterUserUserNameLength,
	"ClusterAccessClusteraccessUserIDLength":                                  ClusterAccessClusteraccessUserIDLength,
	"ClusterAccessClusteraccessManagedEnvironmentIDLength":                    ClusterAccessClusteraccessManagedEnvironmentIDLength,
	"ClusterAccessClusteraccessGitopsEngineInstanceIDLength":                  ClusterAccessClusteraccessGitopsEngineInstanceIDLength,
	"OperationOperationIDLength":                                              OperationOperationIDLength,
	"OperationInstanceIDLength":                                               OperationInstanceIDLength,
	"OperationResourceIDLength":                                               OperationResourceIDLength,
	"OperationOperationOwnerUserIDLength":                                     OperationOperationOwnerUserIDLength,
	"OperationResourceTypeLength":                                             OperationResourceTypeLength,
	"OperationStateLength":                                                    OperationStateLength,
	"OperationHumanReadableStateLength":                                       OperationHumanReadableStateLength,
	"ApplicationApplicationIDLength":                                          ApplicationApplicationIDLength,
	"ApplicationNameLength":                                                   ApplicationNameLength,
	"ApplicationSpecFieldLength":                                              ApplicationSpecFieldLength,
	"ApplicationEngineInstanceInstIDLength":                                   ApplicationEngineInstanceInstIDLength,
	"ApplicationManagedEnvironmentIDLength":                                   ApplicationManagedEnvironmentIDLength,
	"ApplicationStateApplicationstateApplicationIDLength":                     ApplicationStateApplicationstateApplicationIDLength,
	"ApplicationStateHealthLength":                                            ApplicationStateHealthLength,
	"ApplicationStateMessageLength":                                           ApplicationStateMessageLength,
	"ApplicationStateRevisionLength":                                          ApplicationStateRevisionLength,
	"ApplicationStateSyncStatusLength":                                        ApplicationStateSyncStatusLength,
	"ApplicationStateResourcesLength":                                         262144, /*Size is defined here because table doesn't have byte Array limit.*/
	"DeploymentToApplicationMappingDeploymenttoapplicationmappingUIDIDLength": DeploymentToApplicationMappingDeploymenttoapplicationmappingUIDIDLength,
	"DeploymentToApplicationMappingNameLength":                                DeploymentToApplicationMappingNameLength,
	"DeploymentToApplicationMappingDeploymentNameLength":                      DeploymentToApplicationMappingNameLength,
	"DeploymentToApplicationMappingNamespaceLength":                           DeploymentToApplicationMappingNamespaceLength,
	"DeploymentToApplicationMappingDeploymentNamespaceLength":                 DeploymentToApplicationMappingNamespaceLength,
	"DeploymentToApplicationMappingNamespaceUIDLength":                        DeploymentToApplicationMappingWorkspaceUIDLength,
	"DeploymentToApplicationMappingWorkspaceUIDLength":                        DeploymentToApplicationMappingWorkspaceUIDLength,
	"DeploymentToApplicationMappingApplicationIDLength":                       DeploymentToApplicationMappingApplicationIDLength,
	"KubernetesToDBResourceMappingKubernetesResourceTypeLength":               KubernetesToDBResourceMappingKubernetesResourceTypeLength,
	"KubernetesToDBResourceMappingKubernetesResourceUIDLength":                KubernetesToDBResourceMappingKubernetesResourceUIDLength,
	"KubernetesToDBResourceMappingDbRelationTypeLength":                       KubernetesToDBResourceMappingDbRelationTypeLength,
	"KubernetesToDBResourceMappingDBRelationTypeLength":                       KubernetesToDBResourceMappingDbRelationTypeLength,
	"KubernetesToDBResourceMappingDbRelationKeyLength":                        KubernetesToDBResourceMappingDbRelationKeyLength,
	"KubernetesToDBResourceMappingDBRelationKeyLength":                        KubernetesToDBResourceMappingDbRelationKeyLength,
	"APICRToDatabaseMappingApiResourceTypeLength":                             APICRToDatabaseMappingApiResourceTypeLength,
	"APICRToDatabaseMappingAPIResourceTypeLength":                             APICRToDatabaseMappingApiResourceTypeLength,
	"APICRToDatabaseMappingApiResourceUIDLength":                              APICRToDatabaseMappingApiResourceUIDLength,
	"APICRToDatabaseMappingAPIResourceUIDLength":                              APICRToDatabaseMappingApiResourceUIDLength,
	"APICRToDatabaseMappingApiResourceNameLength":                             APICRToDatabaseMappingApiResourceNameLength,
	"APICRToDatabaseMappingAPIResourceNameLength":                             APICRToDatabaseMappingApiResourceNameLength,
	"APICRToDatabaseMappingApiResourceNamespaceLength":                        APICRToDatabaseMappingApiResourceNamespaceLength,
	"APICRToDatabaseMappingAPIResourceNamespaceLength":                        APICRToDatabaseMappingApiResourceNamespaceLength,
	"APICRToDatabaseMappingApiResourceWorkspaceUIDLength":                     APICRToDatabaseMappingApiResourceWorkspaceUIDLength,
	"APICRToDatabaseMappingWorkspaceUIDLength":                                APICRToDatabaseMappingApiResourceWorkspaceUIDLength,
	"APICRToDatabaseMappingDbRelationTypeLength":                              APICRToDatabaseMappingDbRelationTypeLength,
	"APICRToDatabaseMappingDBRelationTypeLength":                              APICRToDatabaseMappingDbRelationTypeLength,
	"APICRToDatabaseMappingDbRelationKeyLength":                               APICRToDatabaseMappingDbRelationKeyLength,
	"APICRToDatabaseMappingDBRelationKeyLength":                               APICRToDatabaseMappingDbRelationKeyLength,
	"SyncOperationSyncoperationIDLength":                                      SyncOperationSyncoperationIDLength,
	"SyncOperationSyncOperationIDLength":                                      SyncOperationSyncoperationIDLength,
	"SyncOperationApplicationIDLength":                                        SyncOperationApplicationIDLength,
	"SyncOperationOperationIDLength":                                          SyncOperationOperationIDLength,
	"SyncOperationDeploymentNameLength":                                       SyncOperationDeploymentNameLength,
	"SyncOperationDeploymentNameFieldLength":                                  SyncOperationDeploymentNameLength,
	"SyncOperationRevisionLength":                                             SyncOperationRevisionLength,
	"SyncOperationDesiredStateLength":                                         SyncOperationDesiredStateLength,
}

// Get value of constants based on constant variable name given as String.
func getConstantValue(variable string) int {
	return DbFieldMap[variable]
}
