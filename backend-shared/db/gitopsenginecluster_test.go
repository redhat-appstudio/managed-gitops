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
			ClustercredentialsCredID:  "test-cluster-creds-test-1",
			Host:                      "host",
			KubeConfig:                "kube-config",
			KubeConfig_context:        "kube-config-context",
			ServiceAccountBearerToken: "serviceaccount_bearer_token",
			ServiceAccountNs:          "ServiceAccountNs",
		}

		gitopsEngineClusterput := db.GitopsEngineCluster{
			PrimaryKeyID:         "test-fake-cluster-1",
			ClusterCredentialsID: clusterCredentials.ClustercredentialsCredID,
		}

		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterput)
		Expect(err).To(BeNil())

		gitopsEngineClusterget := db.GitopsEngineCluster{
			PrimaryKeyID: gitopsEngineClusterput.PrimaryKeyID,
		}

		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(err).To(BeNil())
		Expect(gitopsEngineClusterput).Should(Equal(gitopsEngineClusterget))

		rowsAffected, err := dbq.DeleteGitopsEngineClusterById(ctx, gitopsEngineClusterput.PrimaryKeyID)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineClusterget = db.GitopsEngineCluster{
			PrimaryKeyID: "does-not-exist"}
		err = dbq.GetGitopsEngineClusterById(ctx, &gitopsEngineClusterget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineClusterput.ClusterCredentialsID = strings.Repeat("abc", 100)
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineClusterput)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})
})
