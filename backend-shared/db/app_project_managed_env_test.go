package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("AppProjectManagedEnvironment Test", func() {
	var seq = 101
	It("Should Create, Get, Update and Delete an AppProjectManagedEnvironment", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		_, managedEnvironment, _, _, _, err := db.CreateSampleData(dbq)
		Expect(err).To(BeNil())

		var clusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-application",
			User_name:      "test-user-application",
		}
		err = dbq.CreateClusterUser(ctx, clusterUser)
		Expect(err).To(BeNil())

		By("Verify whether AppProjectManagedEnvironment is created")
		appProjectManagedEnv := db.AppProjectManagedEnvironment{
			AppProjectManagedenvID: "test-app-project-managed-env",
			Clusteruser_id:         clusterUser.Clusteruser_id,
			Managed_environment_id: managedEnvironment.Managedenvironment_id,
			SeqID:                  int64(seq),
		}

		err = dbq.CreateAppProjectManagedEnvironment(ctx, &appProjectManagedEnv)
		Expect(err).To(BeNil())

		By("Verify whether AppProjectManagedEnvironment is retrived")
		appProjectManagedEnvget := db.AppProjectManagedEnvironment{
			Managed_environment_id: appProjectManagedEnv.Managed_environment_id,
		}

		err = dbq.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnvget)
		Expect(err).To(BeNil())
		Expect(appProjectManagedEnv).Should(Equal(appProjectManagedEnvget))

		By("Verify whether AppProjectManagedEnvironment is deleted")
		rowsAffected, err := dbq.DeleteAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnv)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnv)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		appProjectManagedEnvget = db.AppProjectManagedEnvironment{
			Managed_environment_id: "does-not-exist",
		}
		err = dbq.GetAppProjectManagedEnvironmentByManagedEnvId(ctx, &appProjectManagedEnvget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

	})

})
