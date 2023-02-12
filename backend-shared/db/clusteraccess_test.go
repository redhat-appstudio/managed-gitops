package db_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("ClusterAccess Tests", func() {
	Context("It should execute all DB functions for ClusterAccess", func() {
		It("Should execute all ClusterAccess Functions", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			ctx := context.Background()

			var clusterUser = &db.ClusterUser{
				ClusterUserID: "test-user-application",
				UserName:      "test-user-application",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			clusterCredentials := db.ClusterCredentials{
				ClustercredentialsCredID:  "test-cluster-creds-test-5",
				Host:                      "host",
				KubeConfig:                "kube-config",
				KubeConfig_context:        "kube-config-context",
				ServiceAccountBearerToken: "serviceaccount_bearer_token",
				ServiceAccountNs:          "ServiceAccountNs",
			}

			managedEnvironment := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env-5",
				ClusterCredentialsID:  clusterCredentials.ClustercredentialsCredID,
				Name:                  "my env",
			}

			gitopsEngineCluster := db.GitopsEngineCluster{
				PrimaryKeyID:         "test-fake-cluster-5",
				ClusterCredentialsID: clusterCredentials.ClustercredentialsCredID,
			}

			gitopsEngineInstance := db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance-id",
				NamespaceName:           "test-fake-namespace",
				NamespaceUID:            "test-fake-namespace-5",
				EngineClusterID:         gitopsEngineCluster.PrimaryKeyID,
			}

			clusterAccess := db.ClusterAccess{
				ClusterAccessUserID:                 clusterUser.ClusterUserID,
				ClusterAccessManagedEnvironmentID:   managedEnvironment.Managedenvironment_id,
				ClusterAccessGitopsEngineInstanceID: gitopsEngineInstance.Gitopsengineinstance_id,
			}

			err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
			Expect(err).To(BeNil())

			err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
			Expect(err).To(BeNil())

			err = dbq.CreateClusterAccess(ctx, &clusterAccess)
			Expect(err).To(BeNil())
			fetchRow := db.ClusterAccess{ClusterAccessUserID: clusterAccess.ClusterAccessUserID,
				ClusterAccessManagedEnvironmentID:   clusterAccess.ClusterAccessManagedEnvironmentID,
				ClusterAccessGitopsEngineInstanceID: clusterAccess.ClusterAccessGitopsEngineInstanceID}
			err = dbq.GetClusterAccessByPrimaryKey(ctx, &fetchRow)
			Expect(err).To(BeNil())
			Expect(fetchRow).Should(Equal(clusterAccess))

			affectedRows, err := dbq.DeleteClusterAccessById(ctx, fetchRow.ClusterAccessUserID, fetchRow.ClusterAccessManagedEnvironmentID, fetchRow.ClusterAccessGitopsEngineInstanceID)
			Expect(err).To(BeNil())
			Expect(affectedRows).To(Equal(1))

			err = dbq.GetClusterAccessByPrimaryKey(ctx, &fetchRow)
			Expect(db.IsResultNotFoundError(err)).To(Equal(true))

			clusterAccess.ClusterAccessUserID = strings.Repeat("abc", 100)
			err = dbq.CreateClusterAccess(ctx, &clusterAccess)
			Expect(db.IsMaxLengthError(err)).To(BeTrue())
		})
	})
})
