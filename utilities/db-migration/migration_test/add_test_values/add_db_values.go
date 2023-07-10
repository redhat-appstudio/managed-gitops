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

	AddTest_PreClusterCredentials = db.ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-1",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
		Namespaces:                  "namespaceA",
		ClusterResources:            true,
	}

	AddTest_PreGitopsEngineCluster = db.GitopsEngineCluster{
		Gitopsenginecluster_id: "test-fake-cluster-1",
		Clustercredentials_id:  AddTest_PreClusterCredentials.Clustercredentials_cred_id,
	}

	AddTest_PreGitopsEngineInstance = db.GitopsEngineInstance{
		Gitopsengineinstance_id: "test-fake-engine-instance-id",
		Namespace_name:          "test-fake-namespace",
		Namespace_uid:           "test-fake-namespace-1",
		EngineCluster_id:        AddTest_PreGitopsEngineCluster.Gitopsenginecluster_id,
	}

	AddTest_PreClusterCredentialsForManagedEnv = db.ClusterCredentials{
		Clustercredentials_cred_id:  "test-cluster-creds-test-2",
		Host:                        "host",
		Kube_config:                 "kube-config",
		Kube_config_context:         "kube-config-context",
		Serviceaccount_bearer_token: "serviceaccount_bearer_token",
		Serviceaccount_ns:           "Serviceaccount_ns",
	}

	AddTest_PreManagedEnvironment = db.ManagedEnvironment{
		Managedenvironment_id: "test-env-1",
		Clustercredentials_id: AddTest_PreClusterCredentialsForManagedEnv.Clustercredentials_cred_id,
		Name:                  "name",
	}

	AddTest_PreClusterAccess = db.ClusterAccess{
		Clusteraccess_user_id:                   AddTest_PreClusterUser.Clusteruser_id,
		Clusteraccess_managed_environment_id:    AddTest_PreManagedEnvironment.Managedenvironment_id,
		Clusteraccess_gitops_engine_instance_id: AddTest_PreGitopsEngineInstance.Gitopsengineinstance_id,
	}

	AddTest_PreApplicationDB = db.Application{
		Application_id:          "test-my-application",
		Name:                    "my-application",
		Spec_field:              "{}",
		Engine_instance_inst_id: AddTest_PreGitopsEngineInstance.Gitopsengineinstance_id,
		Managed_environment_id:  AddTest_PreManagedEnvironment.Managedenvironment_id,
	}

	AddTest_PreApplicationState = db.ApplicationState{
		Applicationstate_application_id: AddTest_PreApplicationDB.Application_id,
		Health:                          "Healthy",
		Sync_Status:                     "Synced",
		ReconciledState:                 "test-reconcile",
		SyncError:                       "test-sync-error",
	}

	AddTest_PreDTAM = db.DeploymentToApplicationMapping{
		Deploymenttoapplicationmapping_uid_id: "test-dtam",
		DeploymentName:                        "test-deployment",
		DeploymentNamespace:                   "test-namespace",
		NamespaceUID:                          "demo-namespace",
		Application_id:                        AddTest_PreApplicationDB.Application_id,
	}

	AddTest_PreOperationDB = db.Operation{
		Operation_id:            "test-operation",
		Instance_id:             AddTest_PreGitopsEngineInstance.Gitopsengineinstance_id,
		Resource_id:             AddTest_PreApplicationDB.Application_id,
		Resource_type:           db.OperationResourceType_Application,
		Operation_owner_user_id: AddTest_PreClusterUser.Clusteruser_id,
		State:                   db.OperationState_Waiting,
	}

	AddTest_PreKubernetesToDBResourceMapping = db.KubernetesToDBResourceMapping{
		KubernetesResourceType: "Namespace",
		KubernetesResourceUID:  "Namespace-uid",
		DBRelationType:         "GitopsEngineCluster",
		DBRelationKey:          AddTest_PreGitopsEngineCluster.Gitopsenginecluster_id,
	}

	AddTest_PreRepositoryCredentials = db.RepositoryCredentials{
		RepositoryCredentialsID: "test-repo-1",
		UserID:                  AddTest_PreClusterUser.Clusteruser_id,
		PrivateURL:              "https://test-private-url",
		AuthUsername:            "test-auth-username",
		AuthPassword:            "test-auth-password",
		AuthSSHKey:              "test-auth-ssh-key",
		SecretObj:               "test-secret-obj",
		EngineClusterID:         AddTest_PreGitopsEngineInstance.Gitopsengineinstance_id,
	}

	AddTest_PreAPICRToDatabaseMapping = db.APICRToDatabaseMapping{
		APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
		APIResourceUID:       "test-uid",
		APIResourceName:      "test-name",
		APIResourceNamespace: "test-namespace",
		NamespaceUID:         "test-namespace-uid",
		DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
		DBRelationKey:        AddTest_PreRepositoryCredentials.RepositoryCredentialsID,
	}

	AddTest_PreSyncOperation = db.SyncOperation{
		SyncOperation_id:    "test-syncOperation",
		Application_id:      AddTest_PreApplicationDB.Application_id,
		Revision:            "master",
		DeploymentNameField: AddTest_PreDTAM.DeploymentName,
		DesiredState:        "Synced",
	}

	AddTest_PreATDMForSyncOperation = db.APICRToDatabaseMapping{
		APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
		APIResourceUID:       "test-k8s-uid",
		APIResourceName:      "test-k8s-name",
		APIResourceNamespace: "test-k8s-namespace",
		NamespaceUID:         "test-namespace-uid",
		DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
		DBRelationKey:        "test-key",
	}

	AddTest_PreAppProjectRepository = db.AppProjectRepository{
		AppprojectRepositoryID:  "test-app-project-repo",
		RepositorycredentialsID: AddTest_PreRepositoryCredentials.RepositoryCredentialsID,
		Clusteruser_id:          AddTest_PreClusterUser.Clusteruser_id,
		RepoURL:                 AddTest_PreRepositoryCredentials.PrivateURL,
	}

	AddTest_PreAppProjectManagedEnv = db.AppProjectManagedEnvironment{
		AppprojectManagedenvID: "test-app-project-managedenv",
		Managed_environment_id: AddTest_PreManagedEnvironment.Managedenvironment_id,
		Clusteruser_id:         AddTest_PreClusterUser.Clusteruser_id,
	}
)
