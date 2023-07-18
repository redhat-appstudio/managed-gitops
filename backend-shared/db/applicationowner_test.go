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
			Expect(err).To(BeNil())

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			ctx := context.Background()

			var clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user-application",
				User_name:      "test-user-application",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			application := db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &application)
			Expect(err).To(BeNil())

			applicationOwner := db.ApplicationOwner{
				ApplicationOwnerApplicationID: application.Application_id,
				ApplicationOwnerUserID:        clusterUser.Clusteruser_id,
			}

			err = dbq.CreateApplicationOwner(ctx, &applicationOwner)
			Expect(err).To(BeNil())

			err = dbq.GetApplicationOwnerByPrimaryKey(ctx, &applicationOwner)
			Expect(err).To(BeNil())

			rowsAffected, err := dbq.DeleteApplicationOwner(ctx, applicationOwner.ApplicationOwnerApplicationID, applicationOwner.ApplicationOwnerUserID)
			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal(1))

			err = dbq.GetApplicationOwnerByPrimaryKey(ctx, &applicationOwner)
			Expect(err).ToNot(BeNil())

		})
	})
})
