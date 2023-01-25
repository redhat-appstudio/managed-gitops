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
	Context("It should execute all SyncOperation Functions", func() {
		It("Should execute all SyncOperation Functions", func() {
			var testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user",
				User_name:      "test-user",
			}

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

			operation := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "fake resource id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbq.CreateOperation(ctx, operation, operation.Operation_owner_user_id)

			Expect(err).To(BeNil())

			insertRow := db.SyncOperation{
				SyncOperation_id:    "test-sync",
				Application_id:      application.Application_id,
				DeploymentNameField: "testDeployment",
				Revision:            "testRev",
				DesiredState:        "Terminated",
			}

			err = dbq.CreateSyncOperation(ctx, &insertRow)

			Expect(err).To(BeNil())
			fetchRow := db.SyncOperation{
				SyncOperation_id: "test-sync",
			}
			err = dbq.GetSyncOperationById(ctx, &fetchRow)
			Expect(err).To(BeNil())
			Expect(fetchRow.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			fetchRow.Created_on = insertRow.Created_on
			Expect(fetchRow).Should(Equal(insertRow))

			updatedSyncOperation := insertRow
			updatedSyncOperation.DesiredState = "Running"

			err = dbq.UpdateSyncOperation(ctx, &updatedSyncOperation)
			Expect(err).To(BeNil())

			err = dbq.GetSyncOperationById(ctx, &fetchRow)
			Expect(err).To(BeNil())
			Expect(fetchRow.DesiredState).Should(Equal(updatedSyncOperation.DesiredState))

			rowCount, err := dbq.DeleteSyncOperationById(ctx, insertRow.SyncOperation_id)
			Expect(err).To(BeNil())
			Expect(rowCount).Should(Equal(1))
			fetchRow = db.SyncOperation{
				SyncOperation_id: "test-sync",
			}

			err = dbq.GetSyncOperationById(ctx, &fetchRow)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))

			// Set the invalid value
			insertRow.DeploymentNameField = strings.Repeat("abc", 100)
			err = dbq.CreateSyncOperation(ctx, &insertRow)
			Expect(db.IsMaxLengthError(err)).To(Equal(true))

		})
	})
})
