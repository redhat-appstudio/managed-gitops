package db_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Gitopsenginecluster Test", func() {
	It("Should Create, Get and Delete a GitopsEngineCluster", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cluster-creds-test-1",
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}

		gitopsEngineClusterput := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-1",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterput)
		Expect(err).To(BeNil())

		gitopsEngineClusterget := db.GitopsEngineCluster{
			Gitopsenginecluster_id: gitopsEngineClusterput.Gitopsenginecluster_id,
		}

		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(err).To(BeNil())
		Expect(gitopsEngineClusterput).Should(Equal(gitopsEngineClusterget))

		rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineClusterput.Gitopsenginecluster_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineClusterget = db.GitopsEngineCluster{
			Gitopsenginecluster_id: "does-not-exist"}
		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineClusterput.Clustercredentials_id = strings.Repeat("abc", 100)
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterput)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})
})
