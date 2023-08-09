package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("ApplicationStates Tests", func() {
	var ctx context.Context
	var dbq db.AllDatabaseQueries
	var managedEnvironment *db.ManagedEnvironment
	var gitopsEngineInstance *db.GitopsEngineInstance
	var application *db.Application
	var applicationState *db.ApplicationState
	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()

		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		_, managedEnvironment, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
		Expect(err).ToNot(HaveOccurred())

		application = &db.Application{
			Application_id:          "test-my-application",
			Name:                    "my-application",
			Spec_field:              "{}",
			Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id:  managedEnvironment.Managedenvironment_id,
		}

		err = dbq.CreateApplication(ctx, application)
		Expect(err).ToNot(HaveOccurred())

		applicationState = &db.ApplicationState{
			Applicationstate_application_id: application.Application_id,
			Health:                          "Progressing",
			Sync_Status:                     "Unknown",
			Resources:                       make([]byte, 10),
			ReconciledState:                 "test-reconciledState",
			Conditions:                      []byte("sample"),
		}

		err = dbq.CreateApplicationState(ctx, applicationState)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		defer dbq.CloseDatabase()
	})
	Context("It should execute all DB functions for ApplicationStates", func() {
		It("Should execute all ApplicationStates Functions", func() {

			fetchObj := &db.ApplicationState{
				Applicationstate_application_id: application.Application_id,
			}
			err := dbq.GetApplicationStateById(ctx, fetchObj)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchObj).Should(Equal(applicationState))

			applicationState.Health = "Healthy"
			applicationState.Sync_Status = "Synced"
			err = dbq.UpdateApplicationState(ctx, applicationState)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetApplicationStateById(ctx, fetchObj)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchObj).Should(Equal(applicationState))

			rowsAffected, err := dbq.DeleteApplicationStateById(ctx, fetchObj.Applicationstate_application_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).To(Equal(1))
			err = dbq.GetApplicationStateById(ctx, fetchObj)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// Set the invalid value
			applicationState.Resources = make([]byte, 262145)

			err = dbq.CreateApplicationState(ctx, applicationState)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Test DisposeAppScoped function for ApplicationState", func() {
		It("Should test DisposeAppScoped function with missing database interface for ApplicationState", func() {

			var dbq db.AllDatabaseQueries

			err := applicationState.DisposeAppScoped(ctx, dbq)
			Expect(err).To(HaveOccurred())

		})

		It("Should test DisposeAppScoped function for ApplicationState", func() {

			err := applicationState.DisposeAppScoped(ctx, dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetApplicationStateById(ctx, applicationState)
			Expect(err).To(HaveOccurred())

		})
	})
})
