package db_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"k8s.io/apimachinery/pkg/util/uuid"
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

		gitopsEngineInstanceget = db.GitopsEngineInstance{
			Gitopsengineinstance_id: "does-not-exist",
		}
		err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineInstanceput.EngineCluster_id = strings.Repeat("abc", 100)
		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceput)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})

	It("Should list GitopsEngineInstances for a GitOpsEngineCluster", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(false, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		clusterCredentials := db.ClusterCredentials{
			Clustercredentials_cred_id:  "test-cred-" + string(uuid.NewUUID()),
			Host:                        "host",
			Kube_config:                 "kube-config",
			Kube_config_context:         "kube-config-context",
			Serviceaccount_bearer_token: "serviceaccount_bearer_token",
			Serviceaccount_ns:           "Serviceaccount_ns",
		}
		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())

		By("creating a GitOpsEngineInstance/Cluster that should NOT be returned by the List function")
		var instanceDbCluster2_shouldNotMatch db.GitopsEngineInstance
		{
			gitopsEngineCluster2 := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-cred-" + string(uuid.NewUUID()),
				Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
			}
			err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster2)
			Expect(err).To(BeNil())

			instanceDbCluster2_shouldNotMatch = db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-ins-id-" + string(uuid.NewUUID()),
				Namespace_name:          "test-fake-namespace-1",
				Namespace_uid:           "test-fake-namespace-1",
				EngineCluster_id:        gitopsEngineCluster2.Gitopsenginecluster_id,
			}
			err = dbq.CreateGitopsEngineInstance(ctx, &instanceDbCluster2_shouldNotMatch)
			Expect(err).To(BeNil())
		}

		By("creating a new GitOpsEngineCluster with 2 Instances, each in different Namespace")
		gitopsEngineCluster := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-cred-" + string(uuid.NewUUID()),
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).To(BeNil())

		instanceDb := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-ins-id-" + string(uuid.NewUUID()),
			Namespace_name:          "test-fake-namespace-1",
			Namespace_uid:           "test-fake-namespace-1",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}
		err = dbq.CreateGitopsEngineInstance(ctx, &instanceDb)
		Expect(err).To(BeNil())

		instanceDb2 := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-ins-id-" + string(uuid.NewUUID()),
			Namespace_name:          "test-fake-namespace-2",
			Namespace_uid:           "test-fake-namespace-2",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}
		err = dbq.CreateGitopsEngineInstance(ctx, &instanceDb2)
		Expect(err).To(BeNil())

		listResults := &[]db.GitopsEngineInstance{}
		err = dbq.ListGitopsEngineInstancesForCluster(ctx, gitopsEngineCluster, listResults)
		Expect(err).To(BeNil())

		for _, listResult := range *listResults {
			Expect(listResult.Gitopsengineinstance_id).ToNot(Equal(instanceDbCluster2_shouldNotMatch.Gitopsengineinstance_id),
				"the GitOpsEngineInstance which is on the cluster, should not be returned by the results")
		}

		gitopsEngineInstancesToMatch := []db.GitopsEngineInstance{instanceDb, instanceDb2}

		for _, gitopsEngineInstancesToMatch := range gitopsEngineInstancesToMatch {
			matchFound := false

			for _, listResult := range *listResults {

				if listResult.Gitopsengineinstance_id == gitopsEngineInstancesToMatch.Gitopsengineinstance_id {
					matchFound = true
					break
				}

			}
			Expect(matchFound).To(BeTrue(), "both instances should be found in the list results")
		}

	})
})
