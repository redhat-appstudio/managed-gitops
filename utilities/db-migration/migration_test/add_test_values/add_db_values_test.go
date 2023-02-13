package addtestvalues

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"k8s.io/apimachinery/pkg/util/uuid"
)

var _ = Describe("Test to populate the database fields", func() {
	Context("Initialize Test Values in database", func() {

		It("Adding data in database", func() {

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			By("Create cluster user")
			var clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-1",
				User_name:      "test-user-1",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			By("Create a cluster credentials for the gitops engine")
			clusterCredentials := &db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, clusterCredentials)
			Expect(err).To(BeNil())

			By("Create a gitopsengine cluster pointing to those cluster credentials")
			gitopsEngineCluster := &db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-1",
				Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
			}
			err = dbq.CreateGitopsEngineCluster(ctx, gitopsEngineCluster)
			Expect(err).To(BeNil())

			By("Create a gitopsengine instance pointing to that gitops engine cluster")
			gitopsEngineInstance := &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id",
				Namespace_name:          "test-fake-namespace",
				Namespace_uid:           "test-fake-namespace-1",
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbq.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
			Expect(err).To(BeNil())

			By("Create cluster credentials for a manager environment")
			clusterCredentialsForManagedEnv := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-2",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsForManagedEnv)
			Expect(err).To(BeNil())

			By("Create a managed environed pointing to those cluster credentials")
			managedEnvironmentDb := db.ManagedEnvironment{
				Managedenvironment_id: "test-env-1",
				Clustercredentials_id: clusterCredentialsForManagedEnv.Clustercredentials_cred_id,
				Name:                  "name",
			}
			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentDb)
			Expect(err).To(BeNil())

			By("Create a clusteraccess granting access the cluster user access to the target managed environment, on the target gitops engine instnace")
			clusterAccess := &db.ClusterAccess{
				Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
				Clusteraccess_managed_environment_id:    managedEnvironmentDb.Managedenvironment_id,
				Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
			}

			err = dbq.CreateClusterAccess(ctx, clusterAccess)
			Expect(err).To(BeNil())

			By("Create an application pointing to the gitops engine instance  and managed environment")
			applicationDB := &db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironmentDb.Managedenvironment_id,
			}
			err = dbq.CreateApplication(ctx, applicationDB)
			Expect(err).To(BeNil())

			By("Create an applicationstate pointing to the application")
			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
				Health:                          "Healthy",
				Sync_Status:                     "Synced",
				ReconciledState:                 "test-reconcile",
				SyncError:                       "test-sync-error",
			}
			err = dbq.CreateApplicationState(ctx, applicationState)
			Expect(err).To(BeNil())

			By("Create a deployment to application mapping to the application")
			dtam := db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: "test-dtam",
				DeploymentName:                        "test-deployment",
				DeploymentNamespace:                   "test-namespace",
				NamespaceUID:                          "demo-namespace",
				Application_id:                        applicationDB.Application_id,
			}
			err = dbq.CreateDeploymentToApplicationMapping(ctx, &dtam)
			Expect(err).To(BeNil())

			By("Create an operation database row pointing to the application")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             applicationDB.Application_id,
				Resource_type:           db.OperationResourceType_Application,
				Operation_owner_user_id: clusterUser.Clusteruser_id,
			}
			err = dbq.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("Create a KubernetesToDBResourceMapping between a gitops engine instance and argo cd namespace")
			kubernetesToDBResourceMapping := &db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "Namespace",
				KubernetesResourceUID:  "Namespace-uid",
				DBRelationType:         "GitopsEngineCluster",
				DBRelationKey:          gitopsEngineCluster.Gitopsenginecluster_id,
			}

			err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, kubernetesToDBResourceMapping)
			Expect(err).To(BeNil())

			By("Create a repository credentials")
			gitopsRepositoryCredentialsDb := db.RepositoryCredentials{
				RepositoryCredentialsID: "test-repo-1",
				UserID:                  clusterUser.Clusteruser_id,
				PrivateURL:              "https://test-private-url",
				AuthUsername:            "test-auth-username",
				AuthPassword:            "test-auth-password",
				AuthSSHKey:              "test-auth-ssh-key",
				SecretObj:               "test-secret-obj",
				EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id,
			}
			err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsDb)
			Expect(err).To(BeNil())

			By("Create an APICRToDatabasemapping pointing to those repository credentials")
			apiCRToDatabaseMappingDb := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
				APIResourceUID:       "test-uid",
				APIResourceName:      "test-name",
				APIResourceNamespace: "test-namespace",
				NamespaceUID:         "test-" + string(uuid.NewUUID()),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
				DBRelationKey:        gitopsRepositoryCredentialsDb.RepositoryCredentialsID,
			}
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
			Expect(err).To(BeNil())

			By("Create a SyncOperation pointing to the Application")
			syncOperation := db.SyncOperation{
				SyncOperation_id:    "test-syncOperation",
				Application_id:      applicationDB.Application_id,
				Revision:            "master",
				DeploymentNameField: dtam.DeploymentName,
				DesiredState:        "Synced",
			}
			err = dbq.CreateSyncOperation(ctx, &syncOperation)
			Expect(err).To(BeNil())

			By("Create an APICRToDatabasemapping pointing to the SyncOperations")
			item := db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				APIResourceUID:       "test-k8s-uid",
				APIResourceName:      "test-k8s-name",
				APIResourceNamespace: "test-k8s-namespace",
				NamespaceUID:         "test-namespace-uid",
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
				DBRelationKey:        "test-key",
			}

			err = dbq.CreateAPICRToDatabaseMapping(ctx, &item)
			Expect(err).To(BeNil())

		})

	})
})
