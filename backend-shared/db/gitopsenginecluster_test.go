package db_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Gitopsenginecluster Test", func() {
	var dbq db.AllDatabaseQueries
	var ctx context.Context
	var clusterCredentials db.ClusterCredentials
	var gitopsEngineCluster db.GitopsEngineCluster
	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()
		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		clusterCredentials = db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-1",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		gitopsEngineCluster = db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-1",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		defer dbq.CloseDatabase()
	})

	It("Should Create, Get and Delete a GitopsEngineCluster", func() {

		gitopsEngineClusterget := db.GitopsEngineCluster{
			Gitopsenginecluster_id: gitopsEngineCluster.Gitopsenginecluster_id,
		}

		err := dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(err).ToNot(HaveOccurred())
		Expect(gitopsEngineCluster).Should(Equal(gitopsEngineClusterget))

		rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineCluster.Gitopsenginecluster_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineClusterget = db.GitopsEngineCluster{
			Gitopsenginecluster_id: "does-not-exist"}
		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineCluster.Clustercredentials_id = strings.Repeat("abc", 100)
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})

	Context("Test Dispose function for gitopsEngineCluster", func() {
		It("Should test Dispose function with missing database interface for gitopsEngineCluster", func() {

			var dbq db.AllDatabaseQueries

			err := gitopsEngineCluster.Dispose(ctx, dbq)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing database interface in GitOpsEngineCluster dispose"))

		})

		It("Should test Dispose function for gitopsEngineCluster", func() {

			err := gitopsEngineCluster.Dispose(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineCluster)
			Expect(err).To(HaveOccurred())

		})
	})
})
