package addtestvalues

import (
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

// To avoid code duplication all the structs are declared as variable which can be acessed in our test files
var (
	AddTest_PreClusterUser = db.ClusterUser{
		Clusteruser_id: "test-user-1",
		User_name:      "test-user-1",
	}
	clusterUser = AddTest_PreClusterUser

	AddTest_PreClusterCredentials = db.ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-1",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}
	clusterCredentials = AddTest_PreClusterCredentials

	AddTest_PreGitopsEngineCluster = db.GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-1",
		Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
	}
	gitopsEngineCluster = AddTest_PreGitopsEngineCluster

	AddTest_PreGitopsEngineInstance = db.GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-1",
		EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
	}
	gitopsEngineInstance = AddTest_PreGitopsEngineInstance

	AddTest_PreClusterCredentialsForManagedEnv = db.ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-2",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}
	clusterCredentialsForManagedEnv = AddTest_PreClusterCredentialsForManagedEnv

	AddTest_PreManagedEnvironment = db.ManagedEnvironment{
		Managedenvironment_id: "test-env-1",
		Clustercredentials_id: clusterCredentialsForManagedEnv.Clustercredentials_cred_id,
		Name:                  "name",
	}
	managedEnvironmentDb = AddTest_PreManagedEnvironment

	AddTest_PreClusterAccess = db.ClusterAccess{
		Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    managedEnvironmentDb.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
	}
	clusterAccess = AddTest_PreClusterAccess

	AddTest_PreApplicationDB = db.Application{
		Application_id:          "test-my-application",
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  managedEnvironmentDb.Managedenvironment_id,
	}
	applicationDB = AddTest_PreApplicationDB

	AddTest_PreApplicationState = db.ApplicationState{
		Applicationstate_application_id: applicationDB.Application_id,
		Health:                          "Healthy",
		Sync_Status:                     "Synced",
		ReconciledState:                 "test-reconcile",
		SyncError:                       "test-sync-error",
	}
	applicationState = AddTest_PreApplicationState

	AddTest_PreDTAM = db.DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: "test-dtam",
		DeploymentName:                        "test-deployment",
		DeploymentNamespace:                   "test-namespace",
		NamespaceUID:                          "demo-namespace",
		Application_id:                        applicationDB.Application_id,
	}
	dtam = AddTest_PreDTAM

	AddTest_PreOperationDB = db.Operation{
		Operation_id:            "test-operation",
		Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             applicationDB.Application_id,
		Resource_type:           db.OperationResourceType_Application,
		Operation_owner_user_id: clusterUser.Clusteruser_id,
		State:                   db.OperationState_Waiting,
	}
	operationDB = AddTest_PreOperationDB

	AddTest_PreKubernetesToDBResourceMapping = db.KubernetesToDBResourceMapping{
		KubernetesResourceType: "Namespace",
		KubernetesResourceUID:  "Namespace-uid",
		DBRelationType:         "GitopsEngineCluster",
		DBRelationKey:          gitopsEngineCluster.Gitopsenginecluster_id,
	}
	kubernetesToDBResourceMapping = AddTest_PreKubernetesToDBResourceMapping

	AddTest_PreRepositoryCredentials = db.RepositoryCredentials{
		RepositoryCredentialsID: "test-repo-1",
		UserID:                  clusterUser.Clusteruser_id,
		PrivateURL:              "https://test-private-url",
		AuthUsername:            "test-auth-username",
		AuthPassword:            "test-auth-password",
		AuthSSHKey:              "test-auth-ssh-key",
		SecretObj:               "test-secret-obj",
		EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id,
	}
	gitopsRepositoryCredentialsDb = AddTest_PreRepositoryCredentials

	AddTest_PreAPICRToDatabaseMapping = db.APICRToDatabaseMapping{
		APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
		APIResourceUID:       "test-uid",
		APIResourceName:      "test-name",
		APIResourceNamespace: "test-namespace",
		NamespaceUID:         "test-namespace-uid",
		DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
		DBRelationKey:        gitopsRepositoryCredentialsDb.RepositoryCredentialsID,
	}
	apiCRToDatabaseMappingDb = AddTest_PreAPICRToDatabaseMapping

	AddTest_PreSyncOperation = db.SyncOperation{
		SyncOperation_id:    "test-syncOperation",
		Application_id:      applicationDB.Application_id,
		Revision:            "master",
		DeploymentNameField: dtam.DeploymentName,
		DesiredState:        "Synced",
	}
	syncOperation = AddTest_PreSyncOperation

	AddTest_PreATDMForSyncOperation = db.APICRToDatabaseMapping{
		APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
		APIResourceUID:       "test-k8s-uid",
		APIResourceName:      "test-k8s-name",
		APIResourceNamespace: "test-k8s-namespace",
		NamespaceUID:         "test-namespace-uid",
		DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
		DBRelationKey:        "test-key",
	}
	atdm = AddTest_PreATDMForSyncOperation
)
