package db_test

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("SyncOperation Tests", func() {
	var ctx context.Context
	var dbq db.AllDatabaseQueries
	var application *db.Application
	var operation *db.Operation
	var insertRow db.SyncOperation

	BeforeEach(func() {
		var testClusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user",
			User_name:      "test-user",
		}

		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()
		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
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

		operation = &db.Operation{
			Operation_id:            "test-operation",
			Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
			Resource_id:             "fake resource id",
			Resource_type:           "GitopsEngineInstance",
			State:                   db.OperationState_Waiting,
			Operation_owner_user_id: testClusterUser.Clusteruser_id,
		}

		err = dbq.CreateOperation(ctx, operation, operation.Operation_owner_user_id)
		Expect(err).ToNot(HaveOccurred())

		insertRow = db.SyncOperation{
			SyncOperation_id:    "test-sync",
			Application_id:      application.Application_id,
			DeploymentNameField: "testDeployment",
			Revision:            "testRev",
			DesiredState:        "Terminated",
		}

		err = dbq.CreateSyncOperation(ctx, &insertRow)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		defer dbq.CloseDatabase()
	})

	Context("It should execute all SyncOperation Functions", func() {
		It("Should execute all SyncOperation Functions", func() {
			fetchRow := db.SyncOperation{
				SyncOperation_id: "test-sync",
			}
			err := dbq.GetSyncOperationById(ctx, &fetchRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchRow.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			fetchRow.Created_on = insertRow.Created_on
			Expect(fetchRow).Should(Equal(insertRow))

			updatedSyncOperation := insertRow
			updatedSyncOperation.DesiredState = "Running"

			err = dbq.UpdateSyncOperation(ctx, &updatedSyncOperation)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetSyncOperationById(ctx, &fetchRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchRow.DesiredState).Should(Equal(updatedSyncOperation.DesiredState))

			rowCount, err := dbq.DeleteSyncOperationById(ctx, insertRow.SyncOperation_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowCount).Should(Equal(1))
			fetchRow = db.SyncOperation{
				SyncOperation_id: "test-sync",
			}

			err = dbq.GetSyncOperationById(ctx, &fetchRow)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

			// Set the invalid value
			insertRow.DeploymentNameField = strings.Repeat("abc", 100)
			err = dbq.CreateSyncOperation(ctx, &insertRow)
			Expect(db.IsMaxLengthError(err)).To(BeTrue())

		})
	})

	Context("Test Dispose function for SyncOperation", func() {
		It("Should test Dispose function with missing database interface", func() {
			var dbq db.AllDatabaseQueries

			err := insertRow.DisposeAppScoped(ctx, dbq)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing database interface in syncoperation dispose"))

		})

		It("Should test Dispose function for SyncOperation", func() {
			err := insertRow.DisposeAppScoped(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetSyncOperationById(ctx, &insertRow)
			Expect(err).To(HaveOccurred())

		})
	})
})
