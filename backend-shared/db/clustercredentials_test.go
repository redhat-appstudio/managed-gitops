package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("ClusterCredentials Tests", func() {
	Context("It should execute all DB functions for ClusterCredentials", func() {
		It("Should execute all ClusterCredentials Functions", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()
			clusterCreds := db.ClusterCredentials{
				Host:                      "test-host",
				KubeConfig:                "test-kube_config",
				KubeConfig_context:        "test-kube_config_context",
				ServiceAccountBearerToken: "test-serviceaccount_bearer_token",
				ServiceAccountNs:          "test-serviceaccount_ns",
			}
			err = dbq.CreateClusterCredentials(ctx, &clusterCreds)
			Expect(err).To(BeNil())
			fetchedCluster := db.ClusterCredentials{
				ClustercredentialsCredID: clusterCreds.ClustercredentialsCredID,
			}
			err = dbq.GetClusterCredentialsById(ctx, &fetchedCluster)
			Expect(err).To(BeNil())
			Expect(clusterCreds).To(Equal(fetchedCluster))

			count, err := dbq.DeleteClusterCredentialsById(ctx, clusterCreds.ClustercredentialsCredID)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))
			err = dbq.GetClusterCredentialsById(ctx, &fetchedCluster)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})
	})
})
