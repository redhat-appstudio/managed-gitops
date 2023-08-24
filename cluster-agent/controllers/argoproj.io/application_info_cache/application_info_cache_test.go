package application_info_cache

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("application_info_cache Test", func() {
	Context("Tests all the functions for ApplicationInfoCache", func() {

		It("Test to Create, Update and Delete an ApplicationState", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx := context.Background()
			aic := NewApplicationInfoCache()
			defer aic.DebugOnly_Shutdown(ctx)

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)

			Expect(err).ToNot(HaveOccurred())
			defer dbq.CloseDatabase()

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			application := &db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			// An entry for the application should be there in order to create an entry for applicationState
			err = dbq.CreateApplication(ctx, application)
			Expect(err).ToNot(HaveOccurred())

			testAppState := db.ApplicationState{
				Applicationstate_application_id: application.Application_id,
				Health:                          "Healthy",
				Sync_Status:                     "Synced",
				ReconciledState:                 "32",
			}
			errCreate := aic.CreateApplicationState(ctx, testAppState)
			Expect(errCreate).ToNot(HaveOccurred())

			dbAppStateObj := db.ApplicationState{
				Applicationstate_application_id: testAppState.Applicationstate_application_id,
			}
			errGet := dbq.GetApplicationStateById(ctx, &dbAppStateObj)
			Expect(errGet).ToNot(HaveOccurred())
			Expect(testAppState).To(Equal(dbAppStateObj))

			_, fromCache, err := aic.GetApplicationStateById(ctx, testAppState.Applicationstate_application_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(fromCache).To(BeTrue())

			testAppState.Health = "Unhealthy"
			errUpdate := aic.UpdateApplicationState(ctx, testAppState)
			Expect(errUpdate).ToNot(HaveOccurred())

			appState, isFromCache, errGet := aic.GetApplicationStateById(ctx, testAppState.Applicationstate_application_id)
			Expect(errGet).ToNot(HaveOccurred())
			Expect(isFromCache).To(BeTrue())
			Expect(testAppState).To(Equal(appState))

			testDeleteAppState := db.ApplicationState{
				Applicationstate_application_id: testAppState.Applicationstate_application_id,
			}

			rowsAffected, errDelete := aic.DeleteApplicationStateById(ctx, testDeleteAppState.Applicationstate_application_id)
			Expect(errDelete).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			// check for entry in db, which should report an error
			errGet = dbq.GetApplicationStateById(ctx, &testDeleteAppState)
			Expect(db.IsResultNotFoundError(errGet)).To(BeTrue())

		})
		It("Tests GetApplicationStateById", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx := context.Background()
			aic := NewApplicationInfoCache()
			defer aic.DebugOnly_Shutdown(ctx)

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)

			Expect(err).ToNot(HaveOccurred())
			defer dbq.CloseDatabase()

			// check if the application state entry is in the cache?
			testId := "test-get-appState"

			//testId doesn't exist, the test should report an error
			appState, isFromCache, errGet := aic.GetApplicationStateById(ctx, testId)
			Expect(errGet).To(HaveOccurred())
			// since no entry in the valuefromcache should be false, appState should be empty
			Expect(isFromCache).To(BeFalse())
			Expect(appState).To(Equal(db.ApplicationState{}))

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			application := &db.Application{
				Application_id:          testId,
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			// An entry for the application should be there in order to create an entry for applicationState
			err = dbq.CreateApplication(ctx, application)
			Expect(err).ToNot(HaveOccurred())

			// create an applicationstate then try to get it
			testAppState := db.ApplicationState{
				Applicationstate_application_id: testId,
				Health:                          "Healthy",
				Sync_Status:                     "Synced",
				ReconciledState:                 "32",
			}
			err = dbq.CreateApplicationState(ctx, &testAppState)
			Expect(err).ToNot(HaveOccurred())

			getAppState, isFromCache, errGet := aic.GetApplicationStateById(ctx, testAppState.Applicationstate_application_id)
			// ideally the appState should now report an ApplicationState obj
			// isFromCache should be false since initially the value will be coming from db
			Expect(errGet).ToNot(HaveOccurred())
			Expect(isFromCache).To(BeFalse())
			Expect(testAppState).To(Equal(getAppState))

			//calling the same GetApplicationStateById again should come from cache, hence isFromCache should be True
			_, isFromCache, errGet = aic.GetApplicationStateById(ctx, testAppState.Applicationstate_application_id)
			Expect(errGet).ToNot(HaveOccurred())
			Expect(isFromCache).To(BeTrue())

			// checking if the entry for the above exists in the database
			errGet = dbq.GetApplicationStateById(ctx, &testAppState)
			Expect(errGet).ToNot(HaveOccurred())
			Expect(testAppState).To(Equal(getAppState))
		})

		It("Tests GetApplicationById", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx := context.Background()
			aic := NewApplicationInfoCache()
			defer aic.DebugOnly_Shutdown(ctx)

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)

			Expect(err).ToNot(HaveOccurred())
			defer dbq.CloseDatabase()

			// check if the application state entry is in the cache?
			testId := "test-get-appState"

			//testId doesn't exist, the test should report an error
			app, valuefromCache, errGet := aic.GetApplicationById(ctx, testId)
			Expect(errGet).To(HaveOccurred())
			// since no entry in the valuefromcache should be false, appState should be empty
			Expect(valuefromCache).To(BeFalse())
			Expect(app).To(Equal(db.Application{}))

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			testapplication := db.Application{
				Application_id:          testId,
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &testapplication)
			Expect(err).ToNot(HaveOccurred())

			getApp, isFromCache, errGet := aic.GetApplicationById(ctx, testapplication.Application_id)
			// ideally the appState should now report an Application obj
			Expect(errGet).ToNot(HaveOccurred())
			Expect(isFromCache).To(BeFalse())
			// Different date format workaround
			// Ensure created on times are the same
			Expect(testapplication.Created_on.Round(1 * time.Minute).UTC()).Should(Equal(getApp.Created_on.Round(1 * time.Minute).UTC()))
			// Then set them to be equal
			testapplication.Created_on = getApp.Created_on
			// And then compare
			Expect(getApp).To(Equal(testapplication))

			//calling the same GetApplicationById again should come from cache, hence ifFromCache should be True
			_, isFromCache, errGet = aic.GetApplicationById(ctx, testapplication.Application_id)
			Expect(errGet).ToNot(HaveOccurred())
			Expect(isFromCache).To(BeTrue())

			// checking if the entry for the above exists in the database
			errGet = dbq.GetApplicationById(ctx, &testapplication)
			Expect(errGet).ToNot(HaveOccurred())
			Expect(getApp).To(Equal(testapplication))
		})

	})

})
