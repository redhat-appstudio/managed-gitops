package db_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("ClusterCredentials Tests", func() {
	var clusterCreds db.ClusterCredentials
	var ctx context.Context
	var dbq db.AllDatabaseQueries

	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()

		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		clusterCreds = db.ClusterCredentials{
			Host:                        "test-host",
			Kube_config:                 "test-kube_config",
			Kube_config_context:         "test-kube_config_context",
			Serviceaccount_bearer_token: "test-serviceaccount_bearer_token",
			Serviceaccount_ns:           "test-serviceaccount_ns",
		}
		err = dbq.CreateClusterCredentials(ctx, &clusterCreds)
		Expect(err).ToNot(HaveOccurred())

	})
	AfterEach(func() {
		dbq.CloseDatabase()
	})
	Context("It should execute all DB functions for ClusterCredentials", func() {
		It("Should execute all ClusterCredentials Functions", func() {

			fetchedCluster := db.ClusterCredentials{
				Clustercredentials_cred_id: clusterCreds.Clustercredentials_cred_id,
			}
			err := dbq.GetClusterCredentialsById(ctx, &fetchedCluster)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchedCluster.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			Expect(clusterCreds).To(Equal(fetchedCluster))

			count, err := dbq.DeleteClusterCredentialsById(ctx, clusterCreds.Clustercredentials_cred_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(1))
			err = dbq.GetClusterCredentialsById(ctx, &fetchedCluster)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})

		It("Should Get ClusterCredentials in batch.", func() {

			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())

			defer dbq.CloseDatabase()

			By("Create multiple ClusterCredentials entries.")

			clusterCreds := db.ClusterCredentials{
				Clustercredentials_cred_id:  "test-" + uuid.NewString(),
				Host:                        "test-host-",
				Kube_config:                 "test-kube_config",
				Kube_config_context:         "test-kube_config_context",
				Serviceaccount_bearer_token: "test-serviceaccount_bearer_token",
				Serviceaccount_ns:           "test-serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCreds)
			Expect(err).ToNot(HaveOccurred())

			clusterCreds.Clustercredentials_cred_id = "test-" + uuid.NewString()
			err = dbq.CreateClusterCredentials(ctx, &clusterCreds)
			Expect(err).ToNot(HaveOccurred())

			clusterCreds.Clustercredentials_cred_id = "test-" + uuid.NewString()
			err = dbq.CreateClusterCredentials(ctx, &clusterCreds)
			Expect(err).ToNot(HaveOccurred())

			clusterCreds.Clustercredentials_cred_id = "test-" + uuid.NewString()
			err = dbq.CreateClusterCredentials(ctx, &clusterCreds)
			Expect(err).ToNot(HaveOccurred())

			clusterCreds.Clustercredentials_cred_id = "test-" + uuid.NewString()
			err = dbq.CreateClusterCredentials(ctx, &clusterCreds)
			Expect(err).ToNot(HaveOccurred())

			By("Get data in batch.")

			var listOfClusterCredFromDB []db.ClusterCredentials
			err = dbq.GetClusterCredentialsBatch(ctx, &listOfClusterCredFromDB, 2, 0)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfClusterCredFromDB).To(HaveLen(2))

			err = dbq.GetClusterCredentialsBatch(ctx, &listOfClusterCredFromDB, 3, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfClusterCredFromDB).To(HaveLen(3))
		})
	})

	Context("Test Dispose function for clusterCredentials", func() {
		It("Should test Dispose function with missing database interface for clusterCredentials", func() {

			var dbq db.AllDatabaseQueries

			err := clusterCreds.Dispose(ctx, dbq)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing database interface in ClusterCredentials dispose"))

		})

		It("Should test Dispose function for clusterCredentials", func() {

			err := clusterCreds.Dispose(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetClusterCredentialsById(ctx, &clusterCreds)
			Expect(err).To(HaveOccurred())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

		})
	})
})
