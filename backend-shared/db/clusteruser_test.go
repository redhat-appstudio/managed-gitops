package db_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("ClusterUser Tests", func() {
	Context("It should execute all DB functions for ClusterUser", func() {
		It("Should execute all ClusterUser Functions", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			user := &db.ClusterUser{
				ClusterUserID: "test-user-id",
				UserName:      "test-user-name",
			}
			err = dbq.CreateClusterUser(ctx, user)
			Expect(err).To(BeNil())

			retrieveUser := &db.ClusterUser{
				UserName: "test-user-name",
			}
			err = dbq.GetClusterUserByUsername(ctx, retrieveUser)
			Expect(err).To(BeNil())
			Expect(user).Should(Equal(retrieveUser))
			retrieveUser = &db.ClusterUser{
				ClusterUserID: user.ClusterUserID,
			}
			err = dbq.GetClusterUserById(ctx, retrieveUser)
			Expect(err).To(BeNil())
			Expect(user).Should(Equal(retrieveUser))
			rowsAffected, err := dbq.DeleteClusterUserById(ctx, retrieveUser.ClusterUserID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))
			err = dbq.GetClusterUserById(ctx, retrieveUser)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

			// Set the invalid value
			user.UserName = strings.Repeat("abc", 100)
			err = dbq.CreateClusterUser(ctx, user)
			Expect(db.IsMaxLengthError(err)).To(Equal(true))

			var specialClusterUser db.ClusterUser
			err = dbq.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser)
			Expect(err).To(BeNil())
			Expect(specialClusterUser.ClusterUserID).To(Equal(db.SpecialClusterUserName))
			Expect(specialClusterUser.UserName).To(Equal(db.SpecialClusterUserName))

			rowsAffected, err = dbq.DeleteClusterUserById(ctx, specialClusterUser.ClusterUserID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))
		})
	})
})
