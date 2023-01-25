package db

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
)

var _ DatabaseQueries = &ChaosDBClient{}

// ChaosDBClient is a DB Client that optionally simulates an unreliable database connections that randomly injects errors
// to allow us to verify that our service tolerates this and/or fails gracefully.

// This unreliable client will be enabled by an environment variable, which we can/will enable when running the service.
// The unreliable DB client will randomly fail a certain percent, and that % is controllable via environment variable.
//
// To enable it, set the following environment varaibles before running the GitOps Service controllers.
// For example:
// - ENABLE_UNRELIABLE_DB=true
// - UNRELIABLE_DB_FAILURE_RATE=10
type ChaosDBClient struct {
	InnerClient DatabaseQueries
}

func (cdb *ChaosDBClient) UpdateOperation(ctx context.Context, obj *Operation) error {

	if err := shouldSimulateFailure("UpdateOperation", obj); err != nil {
		return err
	}

	return cdb.InnerClient.UpdateOperation(ctx, obj)

}

func (cdb *ChaosDBClient) CreateOperation(ctx context.Context, obj *Operation, ownerId string) error {

	if err := shouldSimulateFailure("CreateOperation", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateOperation(ctx, obj, ownerId)

}

func (cdb *ChaosDBClient) GetOperationById(ctx context.Context, obj *Operation) error {

	if err := shouldSimulateFailure("GetOperationById", obj); err != nil {
		return err
	}

	return cdb.InnerClient.GetOperationById(ctx, obj)

}

func (cdb *ChaosDBClient) ListOperationsByResourceIdAndTypeAndOwnerId(ctx context.Context, resourceID string, resourceType OperationResourceType, operations *[]Operation, ownerId string) error {

	if err := shouldSimulateFailure("ListOperationsByResourceIdAndTypeAndOwnerId", resourceID, resourceType, operations, ownerId); err != nil {
		return err
	}

	return cdb.InnerClient.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, resourceID, resourceType, operations, ownerId)

}

func (cdb *ChaosDBClient) CheckedDeleteOperationById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := shouldSimulateFailure("CheckedDeleteOperationById", id, ownerId); err != nil {
		return 0, err
	}

	return cdb.InnerClient.CheckedDeleteOperationById(ctx, id, ownerId)

}

func (cdb *ChaosDBClient) DeleteOperationById(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteOperationById", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteOperationById(ctx, id)

}

func (cdb *ChaosDBClient) ListOperationsToBeGarbageCollected(ctx context.Context, operations *[]Operation) error {

	if err := shouldSimulateFailure("ListOperationsToBeGarbageCollected", operations); err != nil {
		return err
	}

	return cdb.InnerClient.ListOperationsToBeGarbageCollected(ctx, operations)

}

func (cdb *ChaosDBClient) CreateSyncOperation(ctx context.Context, obj *SyncOperation) error {

	if err := shouldSimulateFailure("CreateSyncOperation", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateSyncOperation(ctx, obj)

}

func (cdb *ChaosDBClient) GetSyncOperationById(ctx context.Context, syncOperation *SyncOperation) error {

	if err := shouldSimulateFailure("GetSyncOperationById", syncOperation); err != nil {
		return err
	}

	return cdb.InnerClient.GetSyncOperationById(ctx, syncOperation)

}

func (cdb *ChaosDBClient) DeleteSyncOperationById(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteSyncOperationById", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteSyncOperationById(ctx, id)

}

func (cdb *ChaosDBClient) UpdateSyncOperation(ctx context.Context, obj *SyncOperation) error {

	if err := shouldSimulateFailure("UpdateSyncOperation", obj); err != nil {
		return err
	}

	return cdb.InnerClient.UpdateSyncOperation(ctx, obj)

}

func (cdb *ChaosDBClient) CreateApplication(ctx context.Context, obj *Application) error {

	if err := shouldSimulateFailure("CreateApplication", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateApplication(ctx, obj)

}

func (cdb *ChaosDBClient) CheckedCreateApplication(ctx context.Context, obj *Application, ownerId string) error {

	if err := shouldSimulateFailure("CheckedCreateApplication", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedCreateApplication(ctx, obj, ownerId)

}

func (cdb *ChaosDBClient) GetApplicationById(ctx context.Context, application *Application) error {

	if err := shouldSimulateFailure("GetApplicationById", application); err != nil {
		return err
	}

	return cdb.InnerClient.GetApplicationById(ctx, application)

}

func (cdb *ChaosDBClient) UpdateApplication(ctx context.Context, obj *Application) error {

	if err := shouldSimulateFailure("UpdateApplication", obj); err != nil {
		return err
	}

	return cdb.InnerClient.UpdateApplication(ctx, obj)

}

func (cdb *ChaosDBClient) DeleteApplicationById(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteApplicationById", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteApplicationById(ctx, id)

}

func (cdb *ChaosDBClient) CheckedDeleteApplicationById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := shouldSimulateFailure("CheckedDeleteApplicationById", id, ownerId); err != nil {
		return 0, err
	}

	return cdb.InnerClient.CheckedDeleteApplicationById(ctx, id, ownerId)

}

func (cdb *ChaosDBClient) GetApplicationBatch(ctx context.Context, applications *[]Application, limit, offSet int) error {

	if err := shouldSimulateFailure("GetApplicationBatch", applications, limit, offSet); err != nil {
		return err
	}

	return cdb.InnerClient.GetApplicationBatch(ctx, applications, limit, offSet)

}

func (cdb *ChaosDBClient) CreateAPICRToDatabaseMapping(ctx context.Context, obj *APICRToDatabaseMapping) error {

	if err := shouldSimulateFailure("CreateAPICRToDatabaseMapping", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateAPICRToDatabaseMapping(ctx, obj)

}

func (cdb *ChaosDBClient) ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx context.Context, apiCRResourceType APICRToDatabaseMapping_ResourceType,
	crName string, crNamespace string, crNamespaceUID string, dbRelationType APICRToDatabaseMapping_DBRelationType,
	apiCRToDBMappingParam *[]APICRToDatabaseMapping) error {

	if err := shouldSimulateFailure("ListAPICRToDatabaseMappingByAPINamespaceAndName", apiCRResourceType, crName, crNamespace, crNamespaceUID, dbRelationType, apiCRToDBMappingParam); err != nil {
		return err
	}

	return cdb.InnerClient.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx, apiCRResourceType,
		crName, crNamespace, crNamespaceUID, dbRelationType, apiCRToDBMappingParam)

}

func (cdb *ChaosDBClient) GetDatabaseMappingForAPICR(ctx context.Context, obj *APICRToDatabaseMapping) error {

	if err := shouldSimulateFailure("GetDatabaseMappingForAPICR", obj); err != nil {
		return err
	}

	return cdb.InnerClient.GetDatabaseMappingForAPICR(ctx, obj)

}

func (cdb *ChaosDBClient) DeleteAPICRToDatabaseMapping(ctx context.Context, obj *APICRToDatabaseMapping) (int, error) {

	if err := shouldSimulateFailure("DeleteAPICRToDatabaseMapping", obj); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteAPICRToDatabaseMapping(ctx, obj)

}

func (cdb *ChaosDBClient) CreateDeploymentToApplicationMapping(ctx context.Context, obj *DeploymentToApplicationMapping) error {

	if err := shouldSimulateFailure("CreateDeploymentToApplicationMapping", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateDeploymentToApplicationMapping(ctx, obj)

}

func (cdb *ChaosDBClient) GetDeploymentToApplicationMappingByDeplId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping) error {

	if err := shouldSimulateFailure("GetDeploymentToApplicationMappingByDeplId", deplToAppMappingParam.GetAsLogKeyValues()...); err != nil {
		return err
	}

	return cdb.InnerClient.GetDeploymentToApplicationMappingByDeplId(ctx, deplToAppMappingParam)

}

func (cdb *ChaosDBClient) ListDeploymentToApplicationMappingByNamespaceAndName(ctx context.Context, deploymentName string, deploymentNamespace string, namespaceUID string, deplToAppMappingParam *[]DeploymentToApplicationMapping) error {

	if err := shouldSimulateFailure("ListDeploymentToApplicationMappingByNamespaceAndName", deploymentName, deploymentNamespace, namespaceUID, deplToAppMappingParam); err != nil {
		return err
	}

	return cdb.InnerClient.ListDeploymentToApplicationMappingByNamespaceAndName(ctx, deploymentName, deploymentNamespace, namespaceUID, deplToAppMappingParam)

}

func (cdb *ChaosDBClient) ListDeploymentToApplicationMappingByNamespaceUID(ctx context.Context, namespaceUID string, deplToAppMappingParam *[]DeploymentToApplicationMapping) error {

	if err := shouldSimulateFailure("ListDeploymentToApplicationMappingByNamespaceUID", namespaceUID, deplToAppMappingParam); err != nil {
		return err
	}

	return cdb.InnerClient.ListDeploymentToApplicationMappingByNamespaceUID(ctx, namespaceUID, deplToAppMappingParam)

}

func (cdb *ChaosDBClient) DeleteDeploymentToApplicationMappingByDeplId(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteDeploymentToApplicationMappingByDeplId", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteDeploymentToApplicationMappingByDeplId(ctx, id)

}

func (cdb *ChaosDBClient) DeleteDeploymentToApplicationMappingByNamespaceAndName(ctx context.Context, deploymentName string, deploymentNamespace string, namespaceUID string) (int, error) {

	if err := shouldSimulateFailure("DeleteDeploymentToApplicationMappingByNamespaceAndName", deploymentName, deploymentNamespace, namespaceUID); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteDeploymentToApplicationMappingByNamespaceAndName(ctx, deploymentName, deploymentNamespace, namespaceUID)

}

func (cdb *ChaosDBClient) UpdateSyncOperationRemoveApplicationField(ctx context.Context, applicationId string) (int, error) {

	if err := shouldSimulateFailure("UpdateSyncOperationRemoveApplicationField", applicationId); err != nil {
		return 0, err
	}

	return cdb.InnerClient.UpdateSyncOperationRemoveApplicationField(ctx, applicationId)

}

func (cdb *ChaosDBClient) GetApplicationStateById(ctx context.Context, obj *ApplicationState) error {

	if err := shouldSimulateFailure("GetApplicationStateById", obj); err != nil {
		return err
	}

	return cdb.InnerClient.GetApplicationStateById(ctx, obj)

}

func (cdb *ChaosDBClient) CreateApplicationState(ctx context.Context, obj *ApplicationState) error {

	if err := shouldSimulateFailure("CreateApplicationState", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateApplicationState(ctx, obj)

}

func (cdb *ChaosDBClient) UpdateApplicationState(ctx context.Context, obj *ApplicationState) error {

	if err := shouldSimulateFailure("UpdateApplicationState", obj); err != nil {
		return err
	}

	return cdb.InnerClient.UpdateApplicationState(ctx, obj)

}

func (cdb *ChaosDBClient) DeleteApplicationStateById(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteApplicationStateById", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteApplicationStateById(ctx, id)

}

func (cdb *ChaosDBClient) GetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment) error {

	if err := shouldSimulateFailure("GetManagedEnvironmentById", managedEnvironment); err != nil {
		return err
	}

	return cdb.InnerClient.GetManagedEnvironmentById(ctx, managedEnvironment)
}

func (cdb *ChaosDBClient) GetGitopsEngineInstanceById(ctx context.Context, engineInstanceParam *GitopsEngineInstance) error {

	if err := shouldSimulateFailure("GetGitopsEngineInstanceById", engineInstanceParam); err != nil {
		return err
	}

	return cdb.InnerClient.GetGitopsEngineInstanceById(ctx, engineInstanceParam)

}

func (cdb *ChaosDBClient) GetAPICRForDatabaseUID(ctx context.Context, apiCRToDatabaseMapping *APICRToDatabaseMapping) error {

	if err := shouldSimulateFailure("GetAPICRForDatabaseUID", apiCRToDatabaseMapping); err != nil {
		return err
	}

	return cdb.InnerClient.GetAPICRForDatabaseUID(ctx, apiCRToDatabaseMapping)

}

func (cdb *ChaosDBClient) CreateClusterAccess(ctx context.Context, obj *ClusterAccess) error {

	if err := shouldSimulateFailure("CreateClusterAccess", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateClusterAccess(ctx, obj)

}

func (cdb *ChaosDBClient) CreateRepositoryCredentials(ctx context.Context, obj *RepositoryCredentials) error {

	if err := shouldSimulateFailure("CreateRepositoryCredentials", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateRepositoryCredentials(ctx, obj)

}

func (cdb *ChaosDBClient) UpdateRepositoryCredentials(ctx context.Context, obj *RepositoryCredentials) error {

	if err := shouldSimulateFailure("UpdateRepositoryCredentials", obj); err != nil {
		return err
	}

	return cdb.InnerClient.UpdateRepositoryCredentials(ctx, obj)

}

func (cdb *ChaosDBClient) CreateClusterCredentials(ctx context.Context, obj *ClusterCredentials) error {

	if err := shouldSimulateFailure("CreateClusterCredentials", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateClusterCredentials(ctx, obj)

}

func (cdb *ChaosDBClient) CreateClusterUser(ctx context.Context, obj *ClusterUser) error {

	if err := shouldSimulateFailure("CreateClusterUser", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateClusterUser(ctx, obj)

}

func (cdb *ChaosDBClient) CreateGitopsEngineCluster(ctx context.Context, obj *GitopsEngineCluster) error {

	if err := shouldSimulateFailure("CreateGitopsEngineCluster", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateGitopsEngineCluster(ctx, obj)

}

func (cdb *ChaosDBClient) CreateGitopsEngineInstance(ctx context.Context, obj *GitopsEngineInstance) error {

	if err := shouldSimulateFailure("CreateGitopsEngineInstance", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateGitopsEngineInstance(ctx, obj)

}

func (cdb *ChaosDBClient) CreateManagedEnvironment(ctx context.Context, obj *ManagedEnvironment) error {

	if err := shouldSimulateFailure("CreateManagedEnvironment", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateManagedEnvironment(ctx, obj)

}

func (cdb *ChaosDBClient) CreateKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) error {

	if err := shouldSimulateFailure("CreateKubernetesResourceToDBResourceMapping", obj); err != nil {
		return err
	}

	return cdb.InnerClient.CreateKubernetesResourceToDBResourceMapping(ctx, obj)

}

func (cdb *ChaosDBClient) CheckedDeleteDeploymentToApplicationMappingByDeplId(ctx context.Context, id string, ownerId string) (int, error) {

	if err := shouldSimulateFailure("CheckedDeleteDeploymentToApplicationMappingByDeplId", id, ownerId); err != nil {
		return 0, err
	}

	return cdb.InnerClient.CheckedDeleteDeploymentToApplicationMappingByDeplId(ctx, id, ownerId)

}

func (cdb *ChaosDBClient) DeleteClusterAccessById(ctx context.Context, userId string, managedEnvironmentId string, gitopsEngineInstanceId string) (int, error) {

	if err := shouldSimulateFailure("DeleteClusterAccessById", userId, managedEnvironmentId, gitopsEngineInstanceId); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteClusterAccessById(ctx, userId, managedEnvironmentId, gitopsEngineInstanceId)

}

func (cdb *ChaosDBClient) CheckedDeleteGitopsEngineInstanceById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := shouldSimulateFailure("CheckedDeleteGitopsEngineInstanceById", id, ownerId); err != nil {
		return 0, err
	}

	return cdb.InnerClient.CheckedDeleteGitopsEngineInstanceById(ctx, id, ownerId)

}

func (cdb *ChaosDBClient) CheckedDeleteManagedEnvironmentById(ctx context.Context, id string, ownerId string) (int, error) {

	if err := shouldSimulateFailure("CheckedDeleteManagedEnvironmentById", id, ownerId); err != nil {
		return 0, err
	}

	return cdb.InnerClient.CheckedDeleteManagedEnvironmentById(ctx, id, ownerId)

}

func (cdb *ChaosDBClient) CheckedGetApplicationById(ctx context.Context, application *Application, ownerId string) error {

	if err := shouldSimulateFailure("CheckedGetApplicationById", application, ownerId); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedGetApplicationById(ctx, application, ownerId)

}

func (cdb *ChaosDBClient) CheckedGetClusterCredentialsById(ctx context.Context, clusterCredentials *ClusterCredentials, ownerId string) error {

	if err := shouldSimulateFailure("CheckedGetClusterCredentialsById", clusterCredentials, ownerId); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedGetClusterCredentialsById(ctx, clusterCredentials, ownerId)

}

func (cdb *ChaosDBClient) GetClusterUserById(ctx context.Context, clusterUser *ClusterUser) error {

	if err := shouldSimulateFailure("GetClusterUserById", clusterUser); err != nil {
		return err
	}

	return cdb.InnerClient.GetClusterUserById(ctx, clusterUser)

}

func (cdb *ChaosDBClient) GetClusterUserByUsername(ctx context.Context, clusterUser *ClusterUser) error {

	if err := shouldSimulateFailure("GetClusterUserByUsername", clusterUser); err != nil {
		return err
	}

	return cdb.InnerClient.GetClusterUserByUsername(ctx, clusterUser)

}

func (cdb *ChaosDBClient) GetOrCreateSpecialClusterUser(ctx context.Context, clusterUser *ClusterUser) error {

	if err := shouldSimulateFailure("GetOrCreateSpecialClusterUser", clusterUser); err != nil {
		return err
	}

	return cdb.InnerClient.GetOrCreateSpecialClusterUser(ctx, clusterUser)

}

func (cdb *ChaosDBClient) CheckedGetGitopsEngineClusterById(ctx context.Context, gitopsEngineCluster *GitopsEngineCluster, ownerId string) error {

	if err := shouldSimulateFailure("CheckedGetGitopsEngineClusterById", gitopsEngineCluster, ownerId); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedGetGitopsEngineClusterById(ctx, gitopsEngineCluster, ownerId)

}

func (cdb *ChaosDBClient) CheckedGetGitopsEngineInstanceById(ctx context.Context, engineInstanceParam *GitopsEngineInstance, ownerId string) error {

	if err := shouldSimulateFailure("CheckedGetGitopsEngineInstanceById", engineInstanceParam, ownerId); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedGetGitopsEngineInstanceById(ctx, engineInstanceParam, ownerId)

}

func (cdb *ChaosDBClient) CheckedGetManagedEnvironmentById(ctx context.Context, managedEnvironment *ManagedEnvironment, ownerId string) error {

	if err := shouldSimulateFailure("CheckedGetManagedEnvironmentById", managedEnvironment, ownerId); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedGetManagedEnvironmentById(ctx, managedEnvironment, ownerId)

}

func (cdb *ChaosDBClient) CheckedGetOperationById(ctx context.Context, operation *Operation, ownerId string) error {

	if err := shouldSimulateFailure("CheckedGetOperationById", operation, ownerId); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedGetOperationById(ctx, operation, ownerId)

}

func (cdb *ChaosDBClient) CheckedGetDeploymentToApplicationMappingByDeplId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping, ownerId string) error {

	if err := shouldSimulateFailure("CheckedGetDeploymentToApplicationMappingByDeplId", deplToAppMappingParam, ownerId); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedGetDeploymentToApplicationMappingByDeplId(ctx, deplToAppMappingParam, ownerId)

}

func (cdb *ChaosDBClient) GetClusterAccessByPrimaryKey(ctx context.Context, obj *ClusterAccess) error {

	if err := shouldSimulateFailure("GetClusterAccessByPrimaryKey", obj); err != nil {
		return err
	}

	return cdb.InnerClient.GetClusterAccessByPrimaryKey(ctx, obj)

}

func (cdb *ChaosDBClient) GetDBResourceMappingForKubernetesResource(ctx context.Context, obj *KubernetesToDBResourceMapping) error {

	if err := shouldSimulateFailure("GetDBResourceMappingForKubernetesResource", obj); err != nil {
		return err
	}

	return cdb.InnerClient.GetDBResourceMappingForKubernetesResource(ctx, obj)

}

func (cdb *ChaosDBClient) GetGitopsEngineClusterById(ctx context.Context, gitopsEngineCluster *GitopsEngineCluster) error {

	if err := shouldSimulateFailure("GetGitopsEngineClusterById", gitopsEngineCluster); err != nil {
		return err
	}

	return cdb.InnerClient.GetGitopsEngineClusterById(ctx, gitopsEngineCluster)

}

func (cdb *ChaosDBClient) GetRepositoryCredentialsByID(ctx context.Context, id string) (obj RepositoryCredentials, err error) {

	if err := shouldSimulateFailure("GetRepositoryCredentialsByID", obj); err != nil {
		return obj, err
	}

	return cdb.InnerClient.GetRepositoryCredentialsByID(ctx, id)

}

func (cdb *ChaosDBClient) DeleteKubernetesResourceToDBResourceMapping(ctx context.Context, obj *KubernetesToDBResourceMapping) (int, error) {

	if err := shouldSimulateFailure("DeleteKubernetesResourceToDBResourceMapping", obj); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteKubernetesResourceToDBResourceMapping(ctx, obj)

}

func (cdb *ChaosDBClient) DeleteClusterCredentialsById(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteClusterCredentialsById", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteClusterCredentialsById(ctx, id)

}

func (cdb *ChaosDBClient) DeleteClusterUserById(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteClusterUserById", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteClusterUserById(ctx, id)

}

func (cdb *ChaosDBClient) DeleteGitopsEngineClusterById(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteGitopsEngineClusterById", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteGitopsEngineClusterById(ctx, id)

}

func (cdb *ChaosDBClient) DeleteRepositoryCredentialsByID(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteRepositoryCredentialsByID", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteRepositoryCredentialsByID(ctx, id)

}

func (cdb *ChaosDBClient) GetClusterCredentialsById(ctx context.Context, clusterCreds *ClusterCredentials) error {

	if err := shouldSimulateFailure("GetClusterCredentialsById", clusterCreds); err != nil {
		return err
	}

	return cdb.InnerClient.GetClusterCredentialsById(ctx, clusterCreds)

}

func (cdb *ChaosDBClient) GetDeploymentToApplicationMappingByApplicationId(ctx context.Context, deplToAppMappingParam *DeploymentToApplicationMapping) error {

	if err := shouldSimulateFailure("GetDeploymentToApplicationMappingByApplicationId", deplToAppMappingParam); err != nil {
		return err
	}

	return cdb.InnerClient.GetDeploymentToApplicationMappingByApplicationId(ctx, deplToAppMappingParam)

}

func (cdb *ChaosDBClient) GetDeploymentToApplicationMappingBatch(ctx context.Context, deploymentToApplicationMappings *[]DeploymentToApplicationMapping, limit, offSet int) error {

	if err := shouldSimulateFailure("GetDeploymentToApplicationMappingBatch", deploymentToApplicationMappings, limit, offSet); err != nil {
		return err
	}

	return cdb.InnerClient.GetDeploymentToApplicationMappingBatch(ctx, deploymentToApplicationMappings, limit, offSet)

}

func (cdb *ChaosDBClient) UpdateManagedEnvironment(ctx context.Context, obj *ManagedEnvironment) error {

	if err := shouldSimulateFailure("UpdateManagedEnvironment", obj); err != nil {
		return err
	}

	return cdb.InnerClient.UpdateManagedEnvironment(ctx, obj)

}

func (cdb *ChaosDBClient) DeleteGitopsEngineInstanceById(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteGitopsEngineInstanceById", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteGitopsEngineInstanceById(ctx, id)

}

func (cdb *ChaosDBClient) DeleteManagedEnvironmentById(ctx context.Context, id string) (int, error) {

	if err := shouldSimulateFailure("DeleteManagedEnvironmentById", id); err != nil {
		return 0, err
	}

	return cdb.InnerClient.DeleteManagedEnvironmentById(ctx, id)

}

func (cdb *ChaosDBClient) CheckedListClusterCredentialsByHost(ctx context.Context, hostName string, clusterCredentials *[]ClusterCredentials, ownerId string) error {

	if err := shouldSimulateFailure("CheckedListClusterCredentialsByHost", hostName, clusterCredentials, ownerId); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedListClusterCredentialsByHost(ctx, hostName, clusterCredentials, ownerId)

}

func (cdb *ChaosDBClient) ListManagedEnvironmentForClusterCredentialsAndOwnerId(ctx context.Context, clusterCredentialId string, ownerId string, managedEnvironments *[]ManagedEnvironment) error {

	if err := shouldSimulateFailure("ListManagedEnvironmentForClusterCredentialsAndOwnerId", clusterCredentialId, ownerId, managedEnvironments); err != nil {
		return err
	}

	return cdb.InnerClient.ListManagedEnvironmentForClusterCredentialsAndOwnerId(ctx, clusterCredentialId, ownerId, managedEnvironments)

}

func (cdb *ChaosDBClient) CheckedListGitopsEngineClusterByCredentialId(ctx context.Context, credentialId string, engineClustersParam *[]GitopsEngineCluster, ownerId string) error {

	if err := shouldSimulateFailure("CheckedListGitopsEngineClusterByCredentialId", credentialId, engineClustersParam, ownerId); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedListGitopsEngineClusterByCredentialId(ctx, credentialId, engineClustersParam, ownerId)

}

func (cdb *ChaosDBClient) RemoveManagedEnvironmentFromAllApplications(ctx context.Context, managedEnvironmentID string, applications *[]Application) (int, error) {

	if err := shouldSimulateFailure("DeleteAPICRToDatabaseMapping", managedEnvironmentID, applications); err != nil {
		return 0, err
	}

	return cdb.InnerClient.RemoveManagedEnvironmentFromAllApplications(ctx, managedEnvironmentID, applications)

}

func (cdb *ChaosDBClient) ListClusterAccessesByManagedEnvironmentID(ctx context.Context, managedEnvironmentID string, clusterAccesses *[]ClusterAccess) error {

	if err := shouldSimulateFailure("ListClusterAccessesByManagedEnvironmentID", managedEnvironmentID, clusterAccesses); err != nil {
		return err
	}

	return cdb.InnerClient.ListClusterAccessesByManagedEnvironmentID(ctx, managedEnvironmentID, clusterAccesses)

}

func (cdb *ChaosDBClient) ListApplicationsForManagedEnvironment(ctx context.Context, managedEnvironmentID string, applications *[]Application) (int, error) {

	if err := shouldSimulateFailure("ListApplicationsForManagedEnvironment", managedEnvironmentID, applications); err != nil {
		return 0, err
	}

	return cdb.InnerClient.ListApplicationsForManagedEnvironment(ctx, managedEnvironmentID, applications)

}

func (cdb *ChaosDBClient) CheckedListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx context.Context, engineClusterId string, ownerId string, gitopsEngineInstancesParam *[]GitopsEngineInstance) error {

	if err := shouldSimulateFailure("CheckedListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId", engineClusterId, ownerId, gitopsEngineInstancesParam); err != nil {
		return err
	}

	return cdb.InnerClient.CheckedListAllGitopsEngineInstancesForGitopsEngineClusterIdAndOwnerId(ctx, engineClusterId, ownerId, gitopsEngineInstancesParam)

}

func (cdb *ChaosDBClient) GetAPICRToDatabaseMappingBatch(ctx context.Context, apiCRToDatabaseMapping *[]APICRToDatabaseMapping, limit, offSet int) error {
	if err := shouldSimulateFailure("GetAPICRToDatabaseMappingBatch", apiCRToDatabaseMapping, limit, offSet); err != nil {
		return err
	}

	return cdb.InnerClient.GetAPICRToDatabaseMappingBatch(ctx, apiCRToDatabaseMapping, limit, offSet)
}

func (cdb *ChaosDBClient) CloseDatabase() {
	cdb.InnerClient.CloseDatabase()
}

func shouldSimulateFailure(apiType string, obj ...interface{}) error {

	if !isEnvExist("UNRELIABLE_DB_FAILURE_RATE") {
		return nil
	}

	// set environment variable(UNRELIABLE_DB_FAILURE_RATE) in your terminal before running Gitops service
	unreliableDBClientFailureRate := os.Getenv("UNRELIABLE_DB_FAILURE_RATE")
	unreliableDBClientFailureRateValue, err := strconv.Atoi(unreliableDBClientFailureRate)
	if err != nil {
		return err
	}

	// Convert it to a decimal point, e.g. 50% to 0.5
	unreliableClientFailureRateValueDouble := (float64)(unreliableDBClientFailureRateValue) / (float64)(100)

	// return an error randomly x% of the time (using math/rand package)
	// #nosec G404 -- not used for cryptographic purposes
	if rand.Float64() < float64(unreliableClientFailureRateValueDouble) {
		fmt.Println("unreliable_db_client.go - Simulated DB error:", apiType, obj)
		return fmt.Errorf("unreliable_db_client.go - simulated DB error: %s %v", apiType, obj)
	}

	return nil
}
