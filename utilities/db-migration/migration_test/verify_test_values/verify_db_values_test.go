package verifytestvalues

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"k8s.io/apimachinery/pkg/util/uuid"
)

var _ = Describe("Test to verify that the data added to database is still present", func() {
	Context("Verify Test Values in database", func() {
		AfterEach(func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())
		})
		It("Test to verify the data added in database is still present", func() {
			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			By("Get cluster user")
			var clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-1",
				User_name:      "test-user-1",
			}

			err = dbq.GetClusterUserById(ctx, clusterUser)
			Expect(err).To(BeNil())

			By("Get cluster credentials by ClusterCredentialsId")
			clusterCredentials := &db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-cluster-creds-test-1",
				Host:                        "host",
				Kube_config:                 "kube-config",
				Kube_config_context:         "kube-config-context",
				Serviceaccount_bearer_token: "serviceaccount_bearer_token",
				Serviceaccount_ns:           "Serviceaccount_ns",
			}

			err = dbq.GetClusterCredentialsById(ctx, clusterCredentials)
			Expect(err).To(BeNil())

			By("Get a gitopsengine cluster by Id")
			gitopsEngineCluster := &db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-fake-cluster-1",
			}
			err = dbq.GetGitopsEngineClusterById(ctx, gitopsEngineCluster)
			Expect(err).To(BeNil())

			By("Get a gitopsengine instance by Id")
			gitopsEngineInstance := &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id",
			}
			err = dbq.GetGitopsEngineInstanceById(ctx, gitopsEngineInstance)
			Expect(err).To(BeNil())

			By("Get cluster credentials for a manager environment")
			clusterCredentialsForManagedEnv := db.ClusterCredentials{
				Clustercredentials_cred_id: "test-cluster-creds-test-2",
			}
			err = dbq.GetClusterCredentialsById(ctx, &clusterCredentialsForManagedEnv)
			Expect(err).To(BeNil())

			By("Get a managed environment by Id")
			managedEnvironmentDb := db.ManagedEnvironment{
				Managedenvironment_id: "test-env-1",
			}
			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentDb)
			Expect(err).To(BeNil())

			By("Get clusteraccess by user id")
			clusterAccess := &db.ClusterAccess{
				Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
				Clusteraccess_managed_environment_id:    managedEnvironmentDb.Managedenvironment_id,
				Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
			}
			err = dbq.GetClusterAccessByPrimaryKey(ctx, clusterAccess)
			Expect(err).To(BeNil())

			By("Get application by id")
			applicationDB := &db.Application{
				Application_id: "test-my-application",
			}
			err = dbq.GetApplicationById(ctx, applicationDB)
			Expect(err).To(BeNil())

			By("Get an applicationstate pointing to the application")
			applicationState := &db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}
			err = dbq.GetApplicationStateById(ctx, applicationState)
			Expect(err).To(BeNil())

			By("Get a deployment to application mapping to the application")
			dtam := db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: "test-dtam",
				Application_id:                        applicationDB.Application_id,
			}
			err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, &dtam)
			Expect(err).To(BeNil())

			By("Get an operation database row pointing to the application")
			operationDB := &db.Operation{
				Operation_id: "test-operation",
			}
			err = dbq.GetOperationById(ctx, operationDB)
			Expect(err).To(BeNil())

			By("Get kubernetesToDBResourceMapping between a gitops engine instance and argo cd namespace")
			kubernetesToDBResourceMapping := &db.KubernetesToDBResourceMapping{
				KubernetesResourceType: "Namespace",
				KubernetesResourceUID:  "Namespace-uid",
				DBRelationType:         "GitopsEngineCluster",
				DBRelationKey:          gitopsEngineCluster.Gitopsenginecluster_id,
			}

			err = dbq.GetDBResourceMappingForKubernetesResource(ctx, kubernetesToDBResourceMapping)
			Expect(err).To(BeNil())

			By("Get repository credentials by Id")
			gitopsRepositoryCredentialsDb := &db.RepositoryCredentials{
				RepositoryCredentialsID: "test-repo-1",
			}
			_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsDb.RepositoryCredentialsID)
			Expect(err).To(BeNil())

			By("Get APICRToDatabasemapping pointing to those repository credentials")
			apiCRToDatabaseMappingDb := &db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
				APIResourceUID:       "test-uid",
				APIResourceName:      "test-name",
				APIResourceNamespace: "test-namespace",
				NamespaceUID:         "test-" + string(uuid.NewUUID()),
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
				DBRelationKey:        gitopsRepositoryCredentialsDb.RepositoryCredentialsID,
			}
			err = dbq.GetAPICRForDatabaseUID(ctx, apiCRToDatabaseMappingDb)
			Expect(err).To(BeNil())

			By("Get SyncOperation pointing to the Application")
			syncOperation := db.SyncOperation{
				SyncOperation_id:    "test-syncOperation",
				Application_id:      applicationDB.Application_id,
				Revision:            "master",
				DeploymentNameField: dtam.DeploymentName,
				DesiredState:        "Synced",
			}
			err = dbq.GetSyncOperationById(ctx, &syncOperation)
			Expect(err).To(BeNil())

			By("Get APICRToDatabasemapping pointing to the SyncOperations")
			item := &db.APICRToDatabaseMapping{
				APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				APIResourceUID:       "test-k8s-uid",
				APIResourceName:      "test-k8s-name",
				APIResourceNamespace: "test-k8s-namespace",
				NamespaceUID:         "test-namespace-uid",
				DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
				DBRelationKey:        "test-key",
			}

			err = dbq.GetAPICRForDatabaseUID(ctx, item)
			Expect(err).To(BeNil())

		})

	})
})
