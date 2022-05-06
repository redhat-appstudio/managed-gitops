package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("Gitopsengineinstance Test", func() {
	It("Should Create, Get and Delete a GitopsEngineInstance", func() {
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

		gitopsEngineCluster := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-fake-cluster-1",
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}

		gitopsEngineInstanceput := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id",
			Namespace_name:          "test-fake-namespace",
			Namespace_uid:           "test-fake-namespace-1",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}
		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).To(BeNil())

		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceput)
		Expect(err).To(BeNil())

		gitopsEngineInstanceget := db.GitopsEngineInstance{
			Gitopsengineinstance_id: gitopsEngineInstanceput.Gitopsengineinstance_id,
		}

		err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceget)
		Expect(err).To(BeNil())
		Expect(gitopsEngineInstanceput).Should(Equal(gitopsEngineInstanceget))

		rowsAffected, err := dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstanceput.Gitopsengineinstance_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))
	})
})
