package db_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("AppProjectManagedEnvironment Test", func() {
	var seq = 101

	var (
		ctx context.Context
		dbq db.AllDatabaseQueries
	)
	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()
		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		dbq.CloseDatabase()
	})

	createSampleDataForAppProjectManagedEnv := func() *db.AppProjectManagedEnvironment {
		_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbq)
		Expect(err).ToNot(HaveOccurred())

		var clusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-application",
			User_name:      "test-user-application",
		}
		err = dbq.CreateClusterUser(ctx, clusterUser)
		Expect(err).ToNot(HaveOccurred())

		By("Verify whether AppProjectManagedEnvironment is created")
		appProjectManagedEnv := &db.AppProjectManagedEnvironment{
			AppprojectManagedenvID: "test-app-project-managed-env",
			Clusteruser_id:         clusterUser.Clusteruser_id,
			Managed_environment_id: managedEnvironment.Managedenvironment_id,
			SeqID:                  int64(seq),
		}

		err = dbq.CreateAppProjectManagedEnvironment(ctx, appProjectManagedEnv)
		Expect(err).ToNot(HaveOccurred())

		return appProjectManagedEnv
	}

	It("Should Create, Get, Update and Delete an AppProjectManagedEnvironment", func() {
		appProjectManagedEnv := createSampleDataForAppProjectManagedEnv()

		By("Verify whether AppProjectManagedEnvironment is retrieved")
		appProjectManagedEnvget := db.AppProjectManagedEnvironment{
			Managed_environment_id: appProjectManagedEnv.Managed_environment_id,
		}

		err := dbq.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnvget)
		Expect(err).ToNot(HaveOccurred())
		Expect(*appProjectManagedEnv).Should(Equal(appProjectManagedEnvget))

		By("Verify whether AppProjectManagedEnvironment is deleted")
		rowsAffected, err := dbq.DeleteAppProjectManagedEnvironmentByManagedEnvId(ctx, appProjectManagedEnv)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, appProjectManagedEnv)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		appProjectManagedEnvget = db.AppProjectManagedEnvironment{
			Managed_environment_id: "does-not-exist",
		}
		err = dbq.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnvget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

	})

	Context("Test CountAppProjectManagedEnvironmentByClusterUserID function", func() {
		It("should return 0 if there are no clusterusers", func() {
			appProjEnv := &db.AppProjectManagedEnvironment{}
			rows, err := dbq.CountAppProjectManagedEnvironmentByClusterUserID(ctx, appProjEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(BeZero())
		})

		It("should return an error if the DB query fails", func() {
			var appProjEnv db.AppProjectManagedEnvironment
			rows, err := dbq.CountAppProjectManagedEnvironmentByClusterUserID(getExpiredContext(), &appProjEnv)
			Expect(err).To(HaveOccurred())
			Expect(rows).To(BeZero())
		})

		It("should return the number of rows of AppProjectManagedEnvironment for a given clusteruser", func() {
			By("add sample AppProjectManagedEnvironment with a given clusteruser")
			expectedRowCount := 1
			appProjectManagedEnv := createSampleDataForAppProjectManagedEnv()

			By("verify if the row count matches with the expected AppProjectManagedEnvironment count")
			rowCount, err := dbq.CountAppProjectManagedEnvironmentByClusterUserID(ctx, appProjectManagedEnv)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowCount).To(Equal(expectedRowCount))
		})
	})

	Context("Test ListAppProjectManagedEnvironmentByClusterUserId function", func() {
		It("should return AppProjectManagedEnvironment with the given clusterUserID", func() {
			appProjectManagedEnv := createSampleDataForAppProjectManagedEnv()

			var appProjectManagedEnvs []db.AppProjectManagedEnvironment
			err := dbq.ListAppProjectManagedEnvironmentByClusterUserId(ctx, appProjectManagedEnv.Clusteruser_id, &appProjectManagedEnvs)
			Expect(err).ToNot(HaveOccurred())
			Expect(appProjectManagedEnvs).To(HaveLen(1))
			Expect(appProjectManagedEnvs[0]).To(Equal(*appProjectManagedEnv))

		})

		It("should return an error if the DB query fails", func() {
			var appProjectManagedEnvs []db.AppProjectManagedEnvironment
			err := dbq.ListAppProjectManagedEnvironmentByClusterUserId(getExpiredContext(), "sample-id", &appProjectManagedEnvs)
			Expect(err).To(HaveOccurred())
			Expect(appProjectManagedEnvs).To(BeEmpty())
		})

		It("should return an error if an empty clusterID is passed", func() {
			var appProjectManagedEnvs []db.AppProjectManagedEnvironment
			err := dbq.ListAppProjectManagedEnvironmentByClusterUserId(ctx, "", &appProjectManagedEnvs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("primary key is empty"))
		})

		It("should return an empty slice if there are no AppProjectManagedEnvironment with the given clusterUserID", func() {
			var appProjectManagedEnvs []db.AppProjectManagedEnvironment
			err := dbq.ListAppProjectManagedEnvironmentByClusterUserId(ctx, "sample-id", &appProjectManagedEnvs)
			Expect(err).ToNot(HaveOccurred())
			Expect(appProjectManagedEnvs).To(BeEmpty())
		})
	})
})

// Returns a context that is already expired. Only to be used in unit tests.
func getExpiredContext() context.Context {
	expiredTime := time.Now().Add(-2 * time.Hour)
	ctx, cancel := context.WithDeadline(context.Background(), expiredTime)
	cancel()
	return ctx
}
