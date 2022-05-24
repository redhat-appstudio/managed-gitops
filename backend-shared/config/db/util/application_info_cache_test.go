package util_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	util "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
)

var _ = Describe("application_info_cache Test", func() {
	Context("Tests all the functions for ApplicationInfoCache", func() {

		It("Test to Create, Update and Delete an ApplicationState", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			asc := util.NewApplicationInfoCache()
			defer asc.DebugOnly_Shutdown(ctx)

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

			// An entry for the application should be there in order to create an entry for applicationState
			err = dbq.CreateApplication(ctx, application)
			Expect(err).To(BeNil())

			testAppState := db.ApplicationState{
				Applicationstate_application_id: application.Application_id,
				Health:                          "Healthy",
				Sync_Status:                     "Synced",
			}
			errCreate := asc.CreateApplicationState(ctx, testAppState)
			Expect(errCreate).To(BeNil())

			dbAppStateObj := db.ApplicationState{
				Applicationstate_application_id: testAppState.Applicationstate_application_id,
			}
			errGet := dbq.GetApplicationStateById(ctx, &dbAppStateObj)
			Expect(errGet).To(BeNil())
			Expect(testAppState).To(Equal(dbAppStateObj))

			_, fromCache, err := asc.GetApplicationStateById(ctx, testAppState.Applicationstate_application_id)
			Expect(err).To(BeNil())
			Expect(fromCache).To(BeTrue())

			testAppState.Health = "Unhealthy"
			errUpdate := asc.UpdateApplicationState(ctx, testAppState)
			Expect(errUpdate).To(BeNil())

			appState, isFromCache, errGet := asc.GetApplicationStateById(ctx, testAppState.Applicationstate_application_id)
			Expect(errGet).To(BeNil())
			Expect(isFromCache).To(BeTrue())
			Expect(testAppState).To(Equal(appState))

			testDeleteAppState := db.ApplicationState{
				Applicationstate_application_id: testAppState.Applicationstate_application_id,
			}

			rowsAffected, errDelete := asc.DeleteApplicationStateById(ctx, testDeleteAppState.Applicationstate_application_id)
			Expect(errDelete).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))

			// check for entry in db, which should report an error
			errGet = dbq.GetApplicationStateById(ctx, &testDeleteAppState)
			Expect(db.IsResultNotFoundError(errGet)).To(BeTrue())

		})
		It("Tests GetApplicationStateById", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			asc := util.NewApplicationInfoCache()
			defer asc.DebugOnly_Shutdown(ctx)

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)

			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			// check if the application state entry is in the cache?
			testId := "test-get-appState"

			//testId doesn't exist, the test should report an error
			appState, isFromCache, errGet := asc.GetApplicationStateById(ctx, testId)
			Expect(errGet).ToNot(BeNil())
			// since no entry in the valuefromcache should be false, appState should be empty
			Expect(isFromCache).To(BeFalse())
			Expect(appState).To(Equal(db.ApplicationState{}))

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			application := &db.Application{
				Application_id:          testId,
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			// An entry for the application should be there in order to create an entry for applicationState
			err = dbq.CreateApplication(ctx, application)
			Expect(err).To(BeNil())

			// create an applicationstate then try to get it
			testAppState := db.ApplicationState{
				Applicationstate_application_id: testId,
				Health:                          "Healthy",
				Sync_Status:                     "Synced",
			}
			err = dbq.CreateApplicationState(ctx, &testAppState)
			Expect(err).To(BeNil())

			getAppState, isFromCache, errGet := asc.GetApplicationStateById(ctx, testAppState.Applicationstate_application_id)
			// ideally the appState should now report an ApplicationState obj
			// isFromCache should be false since initially the value will be coming from db
			Expect(errGet).To(BeNil())
			Expect(isFromCache).To(BeFalse())
			Expect(testAppState).To(Equal(getAppState))

			//calling the same GetApplicationStateById again should come from cache, hence isFromCache should be True
			_, isFromCache, errGet = asc.GetApplicationStateById(ctx, testAppState.Applicationstate_application_id)
			Expect(errGet).To(BeNil())
			Expect(isFromCache).To(BeTrue())

			// checking if the entry for the above exists in the database
			errGet = dbq.GetApplicationStateById(ctx, &testAppState)
			Expect(errGet).To(BeNil())
			Expect(testAppState).To(Equal(getAppState))
		})

		It("Tests GetApplicationById", func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx := context.Background()
			asc := util.NewApplicationInfoCache()
			defer asc.DebugOnly_Shutdown(ctx)

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)

			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			// check if the application state entry is in the cache?
			testId := "test-get-appState"

			//testId doesn't exist, the test should report an error
			app, valuefromCache, errGet := asc.GetApplicationById(ctx, testId)
			Expect(errGet).ToNot(BeNil())
			// since no entry in the valuefromcache should be false, appState should be empty
			Expect(valuefromCache).To(BeFalse())
			Expect(app).To(Equal(db.Application{}))

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			testapplication := db.Application{
				Application_id:          testId,
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &testapplication)
			Expect(err).To(BeNil())

			getApp, isFromCache, errGet := asc.GetApplicationById(ctx, testapplication.Application_id)
			// ideally the appState should now report an Application obj
			Expect(errGet).To(BeNil())
			Expect(isFromCache).To(BeFalse())
			Expect(getApp).To(Equal(testapplication))

			//calling the same GetApplicationById again should come from cache, hence ifFromCache should be True
			_, isFromCache, errGet = asc.GetApplicationById(ctx, testapplication.Application_id)
			Expect(errGet).To(BeNil())
			Expect(isFromCache).To(BeTrue())

			// checking if the entry for the above exists in the database
			errGet = dbq.GetApplicationById(ctx, &testapplication)
			Expect(errGet).To(BeNil())
			Expect(getApp).To(Equal(testapplication))
		})

	})

})
