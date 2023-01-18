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

	It("Should list unique namespaces from GitopsEngineInstance", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
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

		// Create duplicate entry with differet namespace
		clusterCredentials.Clustercredentials_cred_id = "test-cred-" + string(uuid.NewUUID())
		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())

		gitopsEngineCluster.Gitopsenginecluster_id = "test-cred-" + string(uuid.NewUUID())
		gitopsEngineCluster.Clustercredentials_id = clusterCredentials.Clustercredentials_cred_id
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).To(BeNil())

		instanceDb.Gitopsengineinstance_id = "test-ins-id-" + string(uuid.NewUUID())
		instanceDb.Namespace_name = "test-fake-namespace-2"
		instanceDb.Namespace_uid = "test-fake-namespace-2"
		instanceDb.EngineCluster_id = gitopsEngineCluster.Gitopsenginecluster_id
		err = dbq.CreateGitopsEngineInstance(ctx, &instanceDb)
		Expect(err).To(BeNil())

		// Create duplicate entry with same namespace

		clusterCredentials.Clustercredentials_cred_id = "test-cred-" + string(uuid.NewUUID())
		err = dbq.CreateClusterCredentials(ctx, &clusterCredentials)
		Expect(err).To(BeNil())

		gitopsEngineCluster.Gitopsenginecluster_id = "test-cred-" + string(uuid.NewUUID())
		gitopsEngineCluster.Clustercredentials_id = clusterCredentials.Clustercredentials_cred_id
		err = dbq.CreateGitopsEngineCluster(ctx, &gitopsEngineCluster)
		Expect(err).To(BeNil())

		instanceDb.Gitopsengineinstance_id = "test-ins-id-" + string(uuid.NewUUID())
		instanceDb.Namespace_name = "test-fake-namespace-2"
		instanceDb.Namespace_uid = "test-fake-namespace-2"
		instanceDb.EngineCluster_id = gitopsEngineCluster.Gitopsenginecluster_id
		err = dbq.CreateGitopsEngineInstance(ctx, &instanceDb)
		Expect(err).To(BeNil())

		By("Get unique namespaces.")
		var instancesArray []db.GitopsEngineInstance
		err = dbq.ListAllGitopsEngineInstanceNamespaces(ctx, &instancesArray)
		Expect(err).To(BeNil())
		// According to this tests there should be 2 namespaces in DB, but there are left over entries by other tests,
		// hence number of elements in response can not be used for verification
		Expect(checkUnique(instancesArray)).To(BeTrue())
	})
})

func checkUnique(instancesArray []db.GitopsEngineInstance) bool {
	// Since Map can have unique keys, append namespace names as key
	// By using Map extra logic to check uniqueness required for Array/Slice can be avoided.
	tempMap := make(map[string]string)
	for _, instance := range instancesArray {
		tempMap[instance.Namespace_name] = ""
	}

	// If number of keys in Map and number of elements in Array are same it meand all entries in Array had unique namespace name.
	return len(tempMap) == len(instancesArray)
}
