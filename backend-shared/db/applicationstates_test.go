package db_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/fauxargocd"
)

var _ = Describe("ApplicationStates Tests", func() {
	var ctx context.Context
	var dbq db.AllDatabaseQueries
	var managedEnvironment *db.ManagedEnvironment
	var gitopsEngineInstance *db.GitopsEngineInstance
	var application *db.Application
	var applicationState *db.ApplicationState
	var appStatus *fauxargocd.FauxApplicationStatus
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

		appStatus = &fauxargocd.FauxApplicationStatus{
			Health: fauxargocd.HealthStatus{
				Status: fauxargocd.HealthStatusProgressing,
			},
			Sync: fauxargocd.SyncStatus{
				Status: fauxargocd.SyncStatusCodeUnknown,
			},
			Resources: make([]fauxargocd.ResourceStatus, 10),
			Conditions: []fauxargocd.ApplicationCondition{
				{
					Type: fauxargocd.ApplicationConditionComparisonError,
				},
			},
		}

		appStatusBytes, err := util.CompressObject(appStatus)
		Expect(err).ToNot(HaveOccurred())
		Expect(appStatusBytes).ToNot(BeNil())

		applicationState = &db.ApplicationState{
			Applicationstate_application_id: application.Application_id,
			ArgoCD_Application_Status:       appStatusBytes,
		}

		err = dbq.CreateApplicationState(ctx, applicationState)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		dbq.CloseDatabase()
	})
	Context("It should execute all DB functions for ApplicationStates", func() {
		It("Should execute all ApplicationStates Functions", func() {

			fetchObj := &db.ApplicationState{
				Applicationstate_application_id: application.Application_id,
			}
			err := dbq.GetApplicationStateById(ctx, fetchObj)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchObj).Should(Equal(applicationState))

			appStatus.Health.Status = fauxargocd.HealthStatusHealthy
			appStatus.Sync.Status = fauxargocd.SyncStatusCodeSynced
			appStatusBytes, err := util.CompressObject(appStatus)
			Expect(err).ToNot(HaveOccurred())
			Expect(appStatusBytes).ToNot(BeNil())
			applicationState.ArgoCD_Application_Status = appStatusBytes

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
			applicationState.ArgoCD_Application_Status = make([]byte, 262145)

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
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

		})
	})
})
