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
	var dbq db.AllDatabaseQueries
	var ctx context.Context
	var clusterCredentials db.ClusterCredentials
	var gitopsEngineCluster db.GitopsEngineCluster
	var gitopsEngineInstance db.GitopsEngineInstance

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

		gitopsEngineInstance = db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-fake-engine-instance-id",
			Namespace_name:          "test-fake-namespace",
			Namespace_uid:           "test-fake-namespace-1",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}
		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		defer dbq.CloseDatabase()
	})

	It("Should Create, Get and Delete a GitopsEngineInstance", func() {
		gitopsEngineInstanceget := db.GitopsEngineInstance{
			Gitopsengineinstance_id: gitopsEngineInstance.Gitopsengineinstance_id,
		}

		err := dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceget)
		Expect(err).ToNot(HaveOccurred())
		Expect(gitopsEngineInstance).Should(Equal(gitopsEngineInstanceget))

		rowsAffected, err := dbq.DeleteGitopsEngineInstanceById(ctx, gitopsEngineInstance.Gitopsengineinstance_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineInstanceget = db.GitopsEngineInstance{
			Gitopsengineinstance_id: "does-not-exist",
		}
		err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstanceget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		gitopsEngineInstance.EngineCluster_id = strings.Repeat("abc", 100)
		err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstance)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})

	It("Should list GitopsEngineInstances for a GitOpsEngineCluster", func() {

		By("creating a GitOpsEngineInstance/Cluster that should NOT be returned by the List function")
		var instanceDbCluster2_shouldNotMatch db.GitopsEngineInstance
		{
			gitopsEngineCluster2 := db.GitopsEngineCluster{
				Gitopsenginecluster_id: "test-cred-" + string(uuid.NewUUID()),
				Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
			}
			err := dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster2)
			Expect(err).ToNot(HaveOccurred())

			instanceDbCluster2_shouldNotMatch = db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-ins-id-" + string(uuid.NewUUID()),
				Namespace_name:          "test-fake-namespace-1",
				Namespace_uid:           "test-fake-namespace-1",
				EngineCluster_id:        gitopsEngineCluster2.Gitopsenginecluster_id,
			}
			err = dbq.CreateGitopsEngineInstance(ctx, &instanceDbCluster2_shouldNotMatch)
			Expect(err).ToNot(HaveOccurred())
		}

		By("creating a new GitOpsEngineCluster with 2 Instances, each in different Namespace")
		gitopsEngineCluster := db.GitopsEngineCluster{
			Gitopsenginecluster_id: "test-cred-" + string(uuid.NewUUID()),
			Clustercredentials_id:  clusterCredentials.Clustercredentials_cred_id,
		}
		err := dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).ToNot(HaveOccurred())

		instanceDb := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-ins-id-" + string(uuid.NewUUID()),
			Namespace_name:          "test-fake-namespace-1",
			Namespace_uid:           "test-fake-namespace-1",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}
		err = dbq.CreateGitopsEngineInstance(ctx, &instanceDb)
		Expect(err).ToNot(HaveOccurred())

		instanceDb2 := db.GitopsEngineInstance{
			Gitopsengineinstance_id: "test-ins-id-" + string(uuid.NewUUID()),
			Namespace_name:          "test-fake-namespace-2",
			Namespace_uid:           "test-fake-namespace-2",
			EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
		}
		err = dbq.CreateGitopsEngineInstance(ctx, &instanceDb2)
		Expect(err).ToNot(HaveOccurred())

		listResults := &[]db.GitopsEngineInstance{}
		err = dbq.ListGitopsEngineInstancesForCluster(ctx, gitopsEngineCluster, listResults)
		Expect(err).ToNot(HaveOccurred())

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

	Context("Test Dispose function for Gitopsengineinstance", func() {
		It("Should test Dispose function with missing database interface for Gitopsengineinstance", func() {

			var dbq db.AllDatabaseQueries

			err := gitopsEngineInstance.Dispose(ctx, dbq)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing database interface in GitopsEngineInstance dispose"))

		})

		It("Should test Dispose function for gitopsEngineCluster", func() {

			err := gitopsEngineInstance.Dispose(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetGitopsEngineInstanceById(ctx, &gitopsEngineInstance)
			Expect(err).To(HaveOccurred())

		})
	})
})
