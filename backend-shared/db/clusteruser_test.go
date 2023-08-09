package db_test

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("ClusterUser Tests", func() {
	var dbq db.AllDatabaseQueries
	var ctx context.Context
	var user *db.ClusterUser

	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()

		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		user = &db.ClusterUser{
			Clusteruser_id: "test-user-id",
			User_name:      "test-user-name",
		}
		err = dbq.CreateClusterUser(ctx, user)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		defer dbq.CloseDatabase()
	})
	Context("It should execute all DB functions for ClusterUser", func() {
		It("Should execute all ClusterUser Functions", func() {

			retrieveUser := &db.ClusterUser{
				User_name: "test-user-name",
			}
			err := dbq.GetClusterUserByUsername(ctx, retrieveUser)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieveUser.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			Expect(user).Should(Equal(retrieveUser))
			retrieveUser = &db.ClusterUser{
				Clusteruser_id: user.Clusteruser_id,
			}
			err = dbq.GetClusterUserById(ctx, retrieveUser)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieveUser.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			Expect(retrieveUser.Display_name).To(BeEmpty())

			retrieveUser.Display_name = "test-display-name"
			err = dbq.UpdateClusterUser(ctx, retrieveUser)
			Expect(err).ToNot(HaveOccurred())
			err = dbq.GetClusterUserByUsername(ctx, retrieveUser)
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieveUser.Display_name).ToNot(BeEmpty())
			Expect(retrieveUser.Display_name).To(Equal("test-display-name"))

			rowsAffected, err := dbq.DeleteClusterUserById(ctx, retrieveUser.Clusteruser_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))
			err = dbq.GetClusterUserById(ctx, retrieveUser)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

			// Set the invalid value
			user.User_name = strings.Repeat("abc", 100)
			err = dbq.CreateClusterUser(ctx, user)
			Expect(db.IsMaxLengthError(err)).To(BeTrue())

			var specialClusterUser db.ClusterUser
			err = dbq.GetOrCreateSpecialClusterUser(ctx, &specialClusterUser)
			Expect(err).ToNot(HaveOccurred())
			Expect(specialClusterUser.Clusteruser_id).To(Equal(db.SpecialClusterUserName))
			Expect(specialClusterUser.User_name).To(Equal(db.SpecialClusterUserName))

			rowsAffected, err = dbq.DeleteClusterUserById(ctx, specialClusterUser.Clusteruser_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))
		})
	})

	Context("Test Dispose function for clusterUser", func() {
		It("Should test Dispose function with missing database interface for clusterUser", func() {

			var dbq db.AllDatabaseQueries

			err := user.Dispose(ctx, dbq)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing database interface in ClusterUser dispose"))

		})

		It("Should test Dispose function for clusterUser", func() {

			err := user.Dispose(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetClusterUserById(ctx, user)
			Expect(err).To(HaveOccurred())

		})
	})
})
