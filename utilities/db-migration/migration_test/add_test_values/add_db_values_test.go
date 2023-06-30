package addtestvalues

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Test to populate the database fields", func() {
	Context("Initialize Test Values in database", func() {

		It("Adding data in database", func() {

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			By("Create cluster user")
			clusterUser := AddTest_PreClusterUser
			err = dbq.CreateClusterUser(ctx, &clusterUser)
			Expect(err).To(BeNil())

			By("Create a cluster credentials for the gitops engine")
			clusterCredentials := AddTest_PreClusterCredentials
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			By("Create a gitopsengine cluster pointing to those cluster credentials")
			gitopsEngineCluster := AddTest_PreGitopsEngineCluster
			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())

			By("Create a gitopsengine instance pointing to that gitops engine cluster")
			gitopsEngineInstance := AddTest_PreGitopsEngineInstance
			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())

			By("Create cluster credentials for a managed environment")
			clusterCredentialsForManagedEnv := AddTest_PreClusterCredentialsForManagedEnv
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsForManagedEnv)
			Expect(err).To(BeNil())

			By("Create a managed environment pointing to those cluster credentials")
			managedEnvironmentDb := AddTest_PreManagedEnvironment
			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentDb)
			Expect(err).To(BeNil())

			By("Create a clusteraccess granting access the cluster user access to the target managed environment, on the target gitops engine instnace")
			clusterAccess := AddTest_PreClusterAccess
			err = dbq.CreateClusterAccess(ctx, &clusterAccess)
			Expect(err).To(BeNil())

			By("Create an application pointing to the gitops engine instance  and managed environment")
			applicationDB := AddTest_PreApplicationDB
			err = dbq.CreateApplication(ctx, &applicationDB)
			Expect(err).To(BeNil())

			By("Create an applicationstate pointing to the application")
			applicationState := AddTest_PreApplicationState
			err = dbq.CreateApplicationState(ctx, &applicationState)
			Expect(err).To(BeNil())

			By("Create a deployment to application mapping to the application")
			dtam := AddTest_PreDTAM
			err = dbq.CreateDeploymentToApplicationMapping(ctx, &dtam)
			Expect(err).To(BeNil())

			By("Create an operation database row pointing to the application")
			operationDB := AddTest_PreOperationDB
			err = dbq.CreateOperation(ctx, &operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("Create a KubernetesToDBResourceMapping between a gitops engine instance and argo cd namespace")
			kubernetesToDBResourceMapping := AddTest_PreKubernetesToDBResourceMapping
			err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
			Expect(err).To(BeNil())

			By("Create a repository credentials")
			gitopsRepositoryCredentialsDb := AddTest_PreRepositoryCredentials
			err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsDb)
			Expect(err).To(BeNil())

			By("Create an APICRToDatabasemapping pointing to those repository credentials")
			apiCRToDatabaseMappingDb := AddTest_PreAPICRToDatabaseMapping
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
			Expect(err).To(BeNil())

			By("Create a SyncOperation pointing to the Application")
			syncOperation := AddTest_PreSyncOperation
			err = dbq.CreateSyncOperation(ctx, &syncOperation)
			Expect(err).To(BeNil())

			By("Create an APICRToDatabasemapping pointing to the SyncOperations")
			atdm := AddTest_PreATDMForSyncOperation
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &atdm)
			Expect(err).To(BeNil())

			By("Create an AppProjectRepository pointing to the RepoCred")
			appProjectRepo := AddTest_PreAppProjectRepository
			err = dbq.CreateAppProjectRepository(ctx, &appProjectRepo)
			Expect(err).To(BeNil())

			By("Create an AppProjectManagedEnv pointing to the ManagedEnv")
			appProjectManagedEnv := AddTest_PreAppProjectManagedEnv
			err = dbq.CreateAppProjectManagedEnvironment(ctx, &appProjectManagedEnv)
			Expect(err).To(BeNil())
		})

	})
})
