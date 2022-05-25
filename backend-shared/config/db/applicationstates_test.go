package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("ApplicationStates Tests", func() {
	Context("It should execute all DB functions for ApplicationStates", func() {
		It("Should execute all ApplicationStates Functions", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()
			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			application := &db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, application)
			Expect(err).To(BeNil())

			applicationState := &db.ApplicationState{
				Applicationstate_application_id: application.Application_id,
				Health:                          "Progressing",
				Sync_Status:                     "Unknown",
				Resources:                       make([]byte, 10),
			}

			err = dbq.CreateApplicationState(ctx, applicationState)
			Expect(err).To(BeNil())

			fetchObj := &db.ApplicationState{
				Applicationstate_application_id: application.Application_id,
			}
			err = dbq.GetApplicationStateById(ctx, fetchObj)
			Expect(err).To(BeNil())
			Expect(fetchObj).Should(Equal(applicationState))

			applicationState.Health = "Healthy"
			applicationState.Sync_Status = "Synced"
			err = dbq.UpdateApplicationState(ctx, applicationState)
			Expect(err).To(BeNil())

			err = dbq.GetApplicationStateById(ctx, fetchObj)
			Expect(err).To(BeNil())
			Expect(fetchObj).Should(Equal(applicationState))

			rowsAffected, err := dbq.DeleteApplicationStateById(ctx, fetchObj.Applicationstate_application_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).To(Equal(1))
			err = dbq.GetApplicationStateById(ctx, fetchObj)
			Expect(db.IsResultNotFoundError(err)).To(Equal(true))

			// Set the invalid value
			applicationState.Resources = make([]byte, 262145)

			err = dbq.CreateApplicationState(ctx, applicationState)
			Expect(err).NotTo(BeNil())
		})
	})
})
