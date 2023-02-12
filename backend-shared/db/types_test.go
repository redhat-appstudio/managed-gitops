package db_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Types Test", func() {
	Context("Tests all the functions for types.go", func() {

		var testClusterUser = &db.ClusterUser{
			ClusterUserID: "test-user",
			UserName:      "test-user",
		}

		It("Should execute select on all the fields of the database.", func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			var applicationStates []db.ApplicationState
			err = dbq.UnsafeListAllApplicationStates(ctx, &applicationStates)
			Expect(err).To(BeNil())

			var applications []db.Application
			err = dbq.UnsafeListAllApplications(ctx, &applications)
			Expect(err).To(BeNil())

			var clusterAccess []db.ClusterAccess
			err = dbq.UnsafeListAllClusterAccess(ctx, &clusterAccess)
			Expect(err).To(BeNil())

			var clusterCredentials []db.ClusterCredentials
			err = dbq.UnsafeListAllClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			var clusterUsers []db.ClusterUser
			err = dbq.UnsafeListAllClusterUsers(ctx, &clusterUsers)
			Expect(err).To(BeNil())

			var engineClusters []db.GitopsEngineCluster
			err = dbq.UnsafeListAllGitopsEngineClusters(ctx, &engineClusters)
			Expect(err).To(BeNil())

			var engineInstances []db.GitopsEngineInstance
			err = dbq.UnsafeListAllGitopsEngineInstances(ctx, &engineInstances)
			Expect(err).To(BeNil())

			var managedEnvironments []db.ManagedEnvironment
			err = dbq.UnsafeListAllManagedEnvironments(ctx, &managedEnvironments)
			Expect(err).To(BeNil())

			var operations []db.Operation
			err = dbq.UnsafeListAllOperations(ctx, &operations)
			Expect(err).To(BeNil())
		})

		It("Should CheckedCreate and CheckedDelete an application", func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			_, managedEnvironment, _, gitopsEngineInstance, clusterAccess, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			application := &db.Application{
				ApplicationID:          "test-my-application",
				Name:                   "my-application",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CheckedCreateApplication(ctx, application, clusterAccess.ClusterAccessUserID)
			Expect(err).To(BeNil())

			retrievedApplication := db.Application{ApplicationID: application.ApplicationID}

			err = dbq.GetApplicationById(ctx, &retrievedApplication)
			Expect(err).To(BeNil())
			Expect(application.ApplicationID).Should(Equal(retrievedApplication.ApplicationID))

			rowsAffected, err := dbq.CheckedDeleteApplicationById(ctx, application.ApplicationID, clusterAccess.ClusterAccessUserID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			retrievedApplication = db.Application{ApplicationID: application.ApplicationID}
			err = dbq.GetApplicationById(ctx, &retrievedApplication)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test deploymenttoapplication mapping", func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			mapping := db.DeploymentToApplicationMapping{}
			err = dbq.CheckedGetDeploymentToApplicationMappingByDeplId(ctx, &mapping, "")
			fmt.Println(err, mapping)

		})

		It("Should test GitopsEngineInstance and GitOpsEngineCluster", func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			_, _, gitopsEngineCluster, gitopsEngineInstance, clusterAccess, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			retrievedGitopsEngineCluster := &db.GitopsEngineCluster{PrimaryKeyID: gitopsEngineCluster.PrimaryKeyID}
			err = dbq.CheckedGetGitopsEngineClusterById(ctx, retrievedGitopsEngineCluster, testClusterUser.ClusterUserID)
			Expect(err).To(BeNil())
			Expect(&gitopsEngineCluster).Should(Equal(&retrievedGitopsEngineCluster))

			rowsAffected, err := dbq.DeleteClusterAccessById(ctx, clusterAccess.ClusterAccessUserID, clusterAccess.ClusterAccessManagedEnvironmentID, clusterAccess.ClusterAccessGitopsEngineInstanceID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			rowsAffected, err = dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			gitopsEngineInstance = &db.GitopsEngineInstance{Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id}
			err = dbq.CheckedGetGitopsEngineInstanceById(ctx, gitopsEngineInstance, testClusterUser.ClusterUserID)
			Expect(err).ToNot(BeNil())
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

			rowsAffected, err = dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineCluster.PrimaryKeyID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			retrievedGitopsEngineCluster = &db.GitopsEngineCluster{PrimaryKeyID: gitopsEngineCluster.PrimaryKeyID}
			err = dbq.CheckedGetGitopsEngineClusterById(ctx, retrievedGitopsEngineCluster, testClusterUser.ClusterUserID)
			Expect(err).ToNot(BeNil())
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})

		It("Should test ManagedEnvironment", func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			_, managedEnvironment, _, _, clusterAccess, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			result := &db.ManagedEnvironment{Managedenvironment_id: managedEnvironment.Managedenvironment_id}

			err = dbq.CheckedGetManagedEnvironmentById(ctx, result, testClusterUser.ClusterUserID)
			Expect(err).To(BeNil())
			Expect(managedEnvironment.CreatedOn.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			managedEnvironment.CreatedOn = result.CreatedOn
			Expect(managedEnvironment).Should(Equal(result))

			result = &db.ManagedEnvironment{Managedenvironment_id: managedEnvironment.Managedenvironment_id}
			err = dbq.CheckedGetManagedEnvironmentById(ctx, result, "another-user-test")
			Expect(err).ToNot(BeNil())
			// deleting from another user should fail
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

			rowsAffected, err := dbq.DeleteClusterAccessById(ctx, clusterAccess.ClusterAccessUserID, clusterAccess.ClusterAccessManagedEnvironmentID, clusterAccess.ClusterAccessGitopsEngineInstanceID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			rowsAffected, err = dbq.DeleteManagedEnvironmentById(ctx, managedEnvironment.Managedenvironment_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			result = &db.ManagedEnvironment{Managedenvironment_id: managedEnvironment.Managedenvironment_id}
			err = dbq.CheckedGetManagedEnvironmentById(ctx, result, testClusterUser.ClusterUserID)
			Expect(err).ToNot(BeNil())
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		})

		It("Should test Operations", func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			operation := &db.Operation{
				Operation_id:         "test-operation",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "fake resource id",
				Resource_type:        "GitopsEngineInstance",
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
			}

			err = dbq.CreateOperation(ctx, operation, operation.OperationOwnerUserID)
			Expect(err).To(BeNil())

			result := db.Operation{Operation_id: operation.Operation_id}
			err = dbq.CheckedGetOperationById(ctx, &result, operation.OperationOwnerUserID)
			Expect(err).To(BeNil())
			Expect(operation.Operation_id).Should(Equal(result.Operation_id))

			result = db.Operation{Operation_id: operation.Operation_id}
			err = dbq.CheckedGetOperationById(ctx, &result, "another-user-test")
			Expect(err).ToNot(BeNil())
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

			rowsAffected, _ := dbq.CheckedDeleteOperationById(ctx, operation.Operation_id, "another-user")
			Expect(rowsAffected).Should(Equal(0))

			rowsAffected, err = dbq.CheckedDeleteOperationById(ctx, operation.Operation_id, operation.OperationOwnerUserID)
			Expect(rowsAffected).Should(Equal(1))
			Expect(err).To(BeNil())
		})
		It("Should test ClusterUser", func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			clusterUser := db.ClusterUser{
				ClusterUserID: "test-my-cluster-user-2",
				UserName:      "cluster-mccluster",
			}
			err = dbq.CreateClusterUser(ctx, &clusterUser)
			Expect(err).To(BeNil())

			retrievedClusterUser := db.ClusterUser{ClusterUserID: clusterUser.ClusterUserID}
			err = dbq.GetClusterUserById(ctx, &retrievedClusterUser)
			Expect(err).To(BeNil())
			Expect(clusterUser.UserName).Should(Equal(retrievedClusterUser.UserName))

			rowsAffected, err := dbq.DeleteClusterUserById(ctx, clusterUser.ClusterUserID)
			Expect(rowsAffected).Should(Equal(1))
			Expect(err).To(BeNil())

			retrievedClusterUser = db.ClusterUser{ClusterUserID: clusterUser.ClusterUserID}
			err = dbq.GetClusterUserById(ctx, &retrievedClusterUser)
			Expect(err).ToNot(BeNil())
			Expect(db.IsResultNotFoundError(err)).To(Equal(true))

			retrievedClusterUser = db.ClusterUser{ClusterUserID: "does-not-exist"}
			err = dbq.GetClusterUserById(ctx, &retrievedClusterUser)
			Expect(err).ToNot(BeNil())
			Expect(db.IsResultNotFoundError(err)).To(Equal(true))
		})

		It("Should test ClusterCredentials", func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			clusterCredentials := db.ClusterCredentials{
				ClustercredentialsCredID:  "test-cluster-creds-test",
				Host:                      "host",
				KubeConfig:                "kube-config",
				KubeConfig_context:        "kube-config-context",
				ServiceAccountBearerToken: "serviceaccount_bearer_token",
				ServiceAccountNs:          "ServiceAccountNs",
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			var gitopsEngineCluster db.GitopsEngineCluster
			var gitopsEngineInstance db.GitopsEngineInstance
			var clusterAccess db.ClusterAccess
			var managedEnvironment db.ManagedEnvironment

			// Create managed environment, and cluster access, so the non-unsafe get works below
			{
				managedEnvironment = db.ManagedEnvironment{
					Managedenvironment_id: "test-managed-env-914",
					ClusterCredentialsID:  clusterCredentials.ClustercredentialsCredID,
					Name:                  "my env",
				}
				err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
				Expect(err).To(BeNil())

				gitopsEngineCluster = db.GitopsEngineCluster{
					PrimaryKeyID:         "test-fake-cluster-914",
					ClusterCredentialsID: clusterCredentials.ClustercredentialsCredID,
				}
				err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
				Expect(err).To(BeNil())

				gitopsEngineInstance = db.GitopsEngineInstance{
					Gitopsengineinstance_id: "test-fake-engine-instance-id",
					NamespaceName:           "test-fake-namespace",
					NamespaceUID:            "test-fake-namespace-914",
					EngineCluster_id:        gitopsEngineCluster.PrimaryKeyID,
				}
				err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
				Expect(err).To(BeNil())

				clusterAccess = db.ClusterAccess{
					ClusterAccessUserID:                 testClusterUser.ClusterUserID,
					ClusterAccessManagedEnvironmentID:   managedEnvironment.Managedenvironment_id,
					ClusterAccessGitopsEngineInstanceID: gitopsEngineInstance.Gitopsengineinstance_id,
				}
				err = dbq.CreateClusterAccess(ctx, &clusterAccess)
				Expect(err).To(BeNil())

			}

			retrievedClusterCredentials := &db.ClusterCredentials{
				ClustercredentialsCredID: clusterCredentials.ClustercredentialsCredID,
			}
			err = dbq.GetClusterCredentialsById(ctx, retrievedClusterCredentials)
			Expect(err).To(BeNil())

			Expect(clusterCredentials.Host).Should(Equal(retrievedClusterCredentials.Host))
			Expect(clusterCredentials.KubeConfig).Should(Equal(retrievedClusterCredentials.KubeConfig))
			Expect(clusterCredentials.KubeConfig_context).Should(Equal(retrievedClusterCredentials.KubeConfig_context))

			retrievedClusterCredentials = &db.ClusterCredentials{
				ClustercredentialsCredID: clusterCredentials.ClustercredentialsCredID,
			}
			err = dbq.CheckedGetClusterCredentialsById(ctx, retrievedClusterCredentials, testClusterUser.ClusterUserID)
			Expect(err).To(BeNil())
			Expect(retrievedClusterCredentials).ToNot(BeNil())

			Expect(clusterCredentials.Host).Should(Equal(retrievedClusterCredentials.Host))
			Expect(clusterCredentials.KubeConfig).Should(Equal(retrievedClusterCredentials.KubeConfig))
			Expect(clusterCredentials.KubeConfig_context).Should(Equal(retrievedClusterCredentials.KubeConfig_context))

			rowsAffected, err := dbq.DeleteClusterAccessById(ctx, clusterAccess.ClusterAccessUserID, clusterAccess.ClusterAccessManagedEnvironmentID, clusterAccess.ClusterAccessGitopsEngineInstanceID)
			Expect(rowsAffected).Should(Equal(1))
			Expect(err).To(BeNil())

			rowsAffected, err = dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id)
			Expect(rowsAffected).Should(Equal(1))
			Expect(err).To(BeNil())

			rowsAffected, err = dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineCluster.PrimaryKeyID)
			Expect(rowsAffected).Should(Equal(1))
			Expect(err).To(BeNil())

			rowsAffected, err = dbq.DeleteManagedEnvironmentById(ctx, managedEnvironment.Managedenvironment_id)
			Expect(rowsAffected).Should(Equal(1))
			Expect(err).To(BeNil())

			rowsAffected, err = dbq.DeleteClusterCredentialsById(ctx, clusterCredentials.ClustercredentialsCredID)
			Expect(rowsAffected).Should(Equal(1))
			Expect(err).To(BeNil())

			retrievedClusterCredentials = &db.ClusterCredentials{
				ClustercredentialsCredID: clusterCredentials.ClustercredentialsCredID,
			}
			err = dbq.GetClusterCredentialsById(ctx, retrievedClusterCredentials)
			Expect(err).ToNot(BeNil())
			Expect(db.IsResultNotFoundError(err)).To(Equal(true))
		})

	})
})
