package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("RepositoryCredentials Tests", func() {
	Context("Given there are DB function for RepositoryCredentials", func() {
		It("should all execute without problems", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			ctx := context.Background()

			// Create a fake customer
			// Acts as Foreign key for 'UserID string `pg:"repo_cred_user_id"`'
			var clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-application",
				User_name:      "test-user-application",
			}

			err = dbq.CreateClusterUser(ctx, clusterUser)
		})
	})
})
