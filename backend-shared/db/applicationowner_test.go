package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("ApplicationOwner Tests", func() {
	Context("It should execute all DB functions for ApplicationOwner", func() {
		It("Should execute all ApplicationOwner Functions", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())
			defer dbq.CloseDatabase()

			ctx := context.Background()

			var clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-application",
				User_name:      "test-user-application",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).ToNot(HaveOccurred())

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			application := db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &application)
			Expect(err).ToNot(HaveOccurred())

			applicationOwner := db.ApplicationOwner{
				ApplicationOwnerApplicationID: application.Application_id,
				ApplicationOwnerUserID:        clusterUser.Clusteruser_id,
			}

			err = dbq.CreateApplicationOwner(ctx, &applicationOwner)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetApplicationOwnerByApplicationID(ctx, &applicationOwner)
			Expect(err).ToNot(HaveOccurred())

			rowsAffected, err := dbq.DeleteApplicationOwner(ctx, applicationOwner.ApplicationOwnerApplicationID)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).To(Equal(1))

			err = dbq.GetApplicationOwnerByApplicationID(ctx, &applicationOwner)
			Expect(err).To(HaveOccurred())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

		})
	})
})
