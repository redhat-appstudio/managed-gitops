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
			err = dbq.CreateClusterUser(ctx, &clusterUser)
			Expect(err).To(BeNil())

			By("Create a cluster credentials for the gitops engine")
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			By("Create a gitopsengine cluster pointing to those cluster credentials")
			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())

			By("Create a gitopsengine instance pointing to that gitops engine cluster")
			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())

			By("Create cluster credentials for a managed environment")
			err = dbq.CreateClusterCredentials(ctx, &clusterCredentialsForManagedEnv)
			Expect(err).To(BeNil())

			By("Create a managed environment pointing to those cluster credentials")
			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironmentDb)
			Expect(err).To(BeNil())

			By("Create a clusteraccess granting access the cluster user access to the target managed environment, on the target gitops engine instnace")
			err = dbq.CreateClusterAccess(ctx, &clusterAccess)
			Expect(err).To(BeNil())

			By("Create an application pointing to the gitops engine instance  and managed environment")
			err = dbq.CreateApplication(ctx, &applicationDB)
			Expect(err).To(BeNil())

			By("Create an applicationstate pointing to the application")
			err = dbq.CreateApplicationState(ctx, &applicationState)
			Expect(err).To(BeNil())

			By("Create a deployment to application mapping to the application")
			err = dbq.CreateDeploymentToApplicationMapping(ctx, &dtam)
			Expect(err).To(BeNil())

			By("Create an operation database row pointing to the application")
			err = dbq.CreateOperation(ctx, &operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("Create a KubernetesToDBResourceMapping between a gitops engine instance and argo cd namespace")
			err = dbq.CreateKubernetesResourceToDBResourceMapping(ctx, &kubernetesToDBResourceMapping)
			Expect(err).To(BeNil())

			By("Create a repository credentials")
			err = dbq.CreateRepositoryCredentials(ctx, &gitopsRepositoryCredentialsDb)
			Expect(err).To(BeNil())

			By("Create an APICRToDatabasemapping pointing to those repository credentials")
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &apiCRToDatabaseMappingDb)
			Expect(err).To(BeNil())

			By("Create a SyncOperation pointing to the Application")
			err = dbq.CreateSyncOperation(ctx, &syncOperation)
			Expect(err).To(BeNil())

			By("Create an APICRToDatabasemapping pointing to the SyncOperations")
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &atdm)
			Expect(err).To(BeNil())

		})

	})
})
