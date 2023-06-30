package verifytestvalues

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	addtestvalues "github.com/redhat-appstudio/managed-gitops/utilities/db-migration/migration_test/add_test_values"
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
			clusterUser := db.ClusterUser{
				Clusteruser_id: addtestvalues.AddTest_PreClusterUser.Clusteruser_id,
			}

			err = dbq.GetClusterUserById(ctx, &clusterUser)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreClusterUser.SeqID = clusterUser.SeqID
			addtestvalues.AddTest_PreClusterUser.Created_on = clusterUser.Created_on
			Expect(addtestvalues.AddTest_PreClusterUser).To(Equal(clusterUser))

			By("Get cluster credentials by ClusterCredentialsId")
			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: addtestvalues.AddTest_PreClusterCredentials.Clustercredentials_cred_id,
			}

			err = dbq.GetClusterCredentialsById(ctx, &clusterCredentials)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreClusterCredentials.SeqID = clusterCredentials.SeqID
			addtestvalues.AddTest_PreClusterCredentials.Created_on = clusterCredentials.Created_on
			Expect(addtestvalues.AddTest_PreClusterCredentials).To(Equal(clusterCredentials))

			By("Get a gitopsengine cluster by Id")
			gitopsEngineCluster := db.GitopsEngineCluster{
				Gitopsenginecluster_id: addtestvalues.AddTest_PreGitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreGitopsEngineCluster.SeqID = gitopsEngineCluster.SeqID
			Expect(addtestvalues.AddTest_PreGitopsEngineCluster).To(Equal(gitopsEngineCluster))

			By("Get a gitopsengine instance by Id")
			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: addtestvalues.AddTest_PreGitopsEngineInstance.Gitopsengineinstance_id,
			}
			err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreGitopsEngineInstance.SeqID = gitopsEngineInstance.SeqID
			Expect(addtestvalues.AddTest_PreGitopsEngineInstance).To(Equal(gitopsEngineInstance))

			By("Get cluster credentials for a managed environment")
			clusterCredentialsForManagedEnv := db.ClusterCredentials{
				Clustercredentials_cred_id: addtestvalues.AddTest_PreClusterCredentialsForManagedEnv.Clustercredentials_cred_id,
			}
			err = dbq.GetClusterCredentialsById(ctx, &clusterCredentialsForManagedEnv)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreClusterCredentialsForManagedEnv.SeqID = clusterCredentialsForManagedEnv.SeqID
			addtestvalues.AddTest_PreClusterCredentialsForManagedEnv.Created_on = clusterCredentialsForManagedEnv.Created_on
			Expect(addtestvalues.AddTest_PreClusterCredentialsForManagedEnv).To(Equal(clusterCredentialsForManagedEnv))

			By("Get a managed environment by Id")
			managedEnvironmentDb := db.ManagedEnvironment{
				Managedenvironment_id: addtestvalues.AddTest_PreManagedEnvironment.Managedenvironment_id,
			}
			err = dbq.GetManagedEnvironmentById(ctx, &managedEnvironmentDb)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreManagedEnvironment.SeqID = managedEnvironmentDb.SeqID
			addtestvalues.AddTest_PreManagedEnvironment.Created_on = managedEnvironmentDb.Created_on
			Expect(addtestvalues.AddTest_PreManagedEnvironment).To(Equal(managedEnvironmentDb))

			By("Get clusteraccess by user id")
			clusterAccess := db.ClusterAccess{
				Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
				Clusteraccess_managed_environment_id:    managedEnvironmentDb.Managedenvironment_id,
				Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
			}
			err = dbq.GetClusterAccessByPrimaryKey(ctx, &clusterAccess)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreClusterAccess.SeqID = clusterAccess.SeqID
			addtestvalues.AddTest_PreClusterAccess.Created_on = clusterAccess.Created_on
			Expect(addtestvalues.AddTest_PreClusterAccess).To(Equal(clusterAccess))

			By("Get application by id")
			applicationDB := db.Application{
				Application_id: addtestvalues.AddTest_PreApplicationDB.Application_id,
			}
			err = dbq.GetApplicationById(ctx, &applicationDB)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreApplicationDB.SeqID = applicationDB.SeqID
			addtestvalues.AddTest_PreApplicationDB.Created_on = applicationDB.Created_on
			Expect(addtestvalues.AddTest_PreApplicationDB).To(Equal(applicationDB))

			By("Get an applicationstate pointing to the application")
			applicationState := db.ApplicationState{
				Applicationstate_application_id: applicationDB.Application_id,
			}
			err = dbq.GetApplicationStateById(ctx, &applicationState)
			Expect(err).To(BeNil())
			Expect(addtestvalues.AddTest_PreApplicationState).To(Equal(applicationState))

			By("Get a deployment to application mapping to the application")
			dtam := db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: addtestvalues.AddTest_PreDTAM.Deploymenttoapplicationmapping_uid_id,
				Application_id:                        applicationDB.Application_id,
			}
			err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, &dtam)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreDTAM.SeqID = dtam.SeqID
			Expect(addtestvalues.AddTest_PreDTAM).To(Equal(dtam))

			By("Get an operation database row pointing to the application")
			operationDB := db.Operation{
				Operation_id: addtestvalues.AddTest_PreOperationDB.Operation_id,
			}
			err = dbq.GetOperationById(ctx, &operationDB)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreOperationDB.SeqID = operationDB.SeqID
			addtestvalues.AddTest_PreOperationDB.Created_on = operationDB.Created_on
			addtestvalues.AddTest_PreOperationDB.Last_state_update = operationDB.Last_state_update
			Expect(addtestvalues.AddTest_PreOperationDB).To(Equal(operationDB))

			By("Get kubernetesToDBResourceMapping between a gitops engine instance and argo cd namespace")
			kubernetesToDBResourceMapping := db.KubernetesToDBResourceMapping{
				KubernetesResourceType: addtestvalues.AddTest_PreKubernetesToDBResourceMapping.KubernetesResourceType,
				KubernetesResourceUID:  addtestvalues.AddTest_PreKubernetesToDBResourceMapping.KubernetesResourceUID,
				DBRelationType:         addtestvalues.AddTest_PreKubernetesToDBResourceMapping.DBRelationType,
				DBRelationKey:          addtestvalues.AddTest_PreKubernetesToDBResourceMapping.DBRelationKey,
			}

			err = dbq.GetDBResourceMappingForKubernetesResource(ctx, &kubernetesToDBResourceMapping)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreKubernetesToDBResourceMapping.SeqID = kubernetesToDBResourceMapping.SeqID
			Expect(addtestvalues.AddTest_PreKubernetesToDBResourceMapping).To(Equal(kubernetesToDBResourceMapping))

			By("Get repository credentials by Id")
			gitopsRepositoryCredentialsDb := db.RepositoryCredentials{
				RepositoryCredentialsID: addtestvalues.AddTest_PreRepositoryCredentials.RepositoryCredentialsID,
			}
			_, err = dbq.GetRepositoryCredentialsByID(ctx, gitopsRepositoryCredentialsDb.RepositoryCredentialsID)
			Expect(err).To(BeNil())
			Expect(addtestvalues.AddTest_PreRepositoryCredentials.RepositoryCredentialsID).To(Equal(gitopsRepositoryCredentialsDb.RepositoryCredentialsID))

			By("Get APICRToDatabasemapping pointing to those repository credentials")
			apiCRToDatabaseMappingDb := db.APICRToDatabaseMapping{
				APIResourceType: db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential,
				DBRelationType:  db.APICRToDatabaseMapping_DBRelationType_ManagedEnvironment,
				DBRelationKey:   gitopsRepositoryCredentialsDb.RepositoryCredentialsID,
			}
			err = dbq.GetAPICRForDatabaseUID(ctx, &apiCRToDatabaseMappingDb)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreAPICRToDatabaseMapping.SeqID = apiCRToDatabaseMappingDb.SeqID
			Expect(addtestvalues.AddTest_PreAPICRToDatabaseMapping).To(Equal(apiCRToDatabaseMappingDb))

			By("Get SyncOperation pointing to the Application")
			syncOperation := db.SyncOperation{
				SyncOperation_id: addtestvalues.AddTest_PreSyncOperation.SyncOperation_id,
			}
			err = dbq.GetSyncOperationById(ctx, &syncOperation)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreSyncOperation.Created_on = syncOperation.Created_on
			Expect(addtestvalues.AddTest_PreSyncOperation).To(Equal(syncOperation))

			By("Get APICRToDatabasemapping pointing to the SyncOperations")
			atdm := db.APICRToDatabaseMapping{
				APIResourceType: db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
				DBRelationType:  db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
				DBRelationKey:   addtestvalues.AddTest_PreATDMForSyncOperation.DBRelationKey,
			}
			err = dbq.GetAPICRForDatabaseUID(ctx, &atdm)
			Expect(err).To(BeNil())
			addtestvalues.AddTest_PreATDMForSyncOperation.SeqID = atdm.SeqID
			Expect(addtestvalues.AddTest_PreATDMForSyncOperation).To(Equal(atdm))

			By("Get AppProjectMangedEnv pointing to the MangedEnv")
			appProjectManagedEnv := db.AppProjectManagedEnvironment{
				Managed_environment_id: managedEnvironmentDb.Managedenvironment_id,
			}
			err = dbq.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnv)
			Expect(err).To(BeNil())

			By("Get AppProjectRepo pointing to the RepoCred")
			appProjectRepo := db.AppProjectRepository{
				Clusteruser_id: clusterUser.Clusteruser_id,
				RepoURL:        "https://test-private-url",
			}
			err = dbq.GetAppProjectRepositoryByUniqueConstraint(ctx, &appProjectRepo)
			Expect(err).To(BeNil())

		})

	})
})
