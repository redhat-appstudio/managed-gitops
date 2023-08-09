package db_test

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("ClusterAccess Tests", func() {

	var ctx context.Context
	var clusterAccess db.ClusterAccess
	var dbq db.AllDatabaseQueries

	BeforeEach(func() {

		ctx = context.Background()

		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		var clusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-application",
			User_name:      "test-user-application",
		}
		err = dbq.CreateClusterUser(ctx, clusterUser)
		Expect(err).ToNot(HaveOccurred())

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-5",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		managedEnvironment := db.ManagedEnvironment{
			Managedenvironment_id: "test-managed-env-5",
			Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			Name:                  "my env",
		}

		gitopsEngineCluster := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-5",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}

		gitopsEngineInstance := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id",
			Namespace_name:          "test-fake-namespace",
			Namespace_uid:           "test-fake-namespace-5",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}

		clusterAccess = db.ClusterAccess{
			Clusteraccess_user_id:                   clusterUser.Clusteruser_id,
			Clusteraccess_managed_environment_id:    managedEnvironment.Managedenvironment_id,
			Clusteraccess_gitops_engine_instance_id: gitopsEngineInstance.Gitopsengineinstance_id,
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateManagedEnvironment(ctx, &managedEnvironment)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateClusterAccess(ctx, &clusterAccess)
		Expect(err).ToNot(HaveOccurred())

	})

	AfterEach(func() {
		defer dbq.CloseDatabase()
	})

	Context("It should execute all DB functions for ClusterAccess", func() {
		It("Should execute all ClusterAccess Functions", func() {

			fetchRow := db.ClusterAccess{Clusteraccess_user_id: clusterAccess.Clusteraccess_user_id,
				Clusteraccess_managed_environment_id:    clusterAccess.Clusteraccess_managed_environment_id,
				Clusteraccess_gitops_engine_instance_id: clusterAccess.Clusteraccess_gitops_engine_instance_id}
			err := dbq.GetClusterAccessByPrimaryKey(ctx, &fetchRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchRow.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			Expect(fetchRow).Should(Equal(clusterAccess))

			affectedRows, err := dbq.DeleteClusterAccessById(ctx, fetchRow.Clusteraccess_user_id, fetchRow.Clusteraccess_managed_environment_id, fetchRow.Clusteraccess_gitops_engine_instance_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(affectedRows).To(Equal(1))

			err = dbq.GetClusterAccessByPrimaryKey(ctx, &fetchRow)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			clusterAccess.Clusteraccess_user_id = strings.Repeat("abc", 100)
			err = dbq.CreateClusterAccess(ctx, &clusterAccess)
			Expect(db.IsMaxLengthError(err)).To(BeTrue())
		})
	})

	Context("Test Dispose function for clusterAccess", func() {
		It("Should test Dispose function with missing database interface", func() {

			var dbq db.AllDatabaseQueries

			err := clusterAccess.Dispose(ctx, dbq)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing database interface in ClusterAccess dispose"))

		})

		It("Should test Dispose function for clusterAccess", func() {

			err := clusterAccess.Dispose(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetClusterAccessByPrimaryKey(ctx, &clusterAccess)
			Expect(err).To(HaveOccurred())

		})
	})
})
