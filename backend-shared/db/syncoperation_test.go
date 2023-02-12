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
				ClusterUserID: "test-user",
				UserName:      "test-user",
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
				ApplicationID:          "test-my-application",
				Name:                   "my-application",
				SpecField:              "{}",
				EngineInstanceInstID:   gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id: managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, application)

			Expect(err).To(BeNil())

			operation := &db.Operation{
				Operation_id:         "test-operation",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "fake resource id",
				Resource_type:        "GitopsEngineInstance",
				State:                db.OperationState_Waiting,
				OperationOwnerUserID: testClusterUser.ClusterUserID,
			}

			err = dbq.CreateOperation(ctx, operation, operation.OperationOwnerUserID)

			Expect(err).To(BeNil())

			insertRow := db.SyncOperation{
				SyncOperationID:     "test-sync",
				ApplicationID:       application.ApplicationID,
				DeploymentNameField: "testDeployment",
				Revision:            "testRev",
				DesiredState:        "Terminated",
			}

			err = dbq.CreateSyncOperation(ctx, &insertRow)

			Expect(err).To(BeNil())
			fetchRow := db.SyncOperation{
				SyncOperationID: "test-sync",
			}
			err = dbq.GetSyncOperationById(ctx, &fetchRow)
			Expect(err).To(BeNil())
			Expect(fetchRow.CreatedOn.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
			fetchRow.CreatedOn = insertRow.CreatedOn
			Expect(fetchRow).Should(Equal(insertRow))

			updatedSyncOperation := insertRow
			updatedSyncOperation.DesiredState = "Running"

			err = dbq.UpdateSyncOperation(ctx, &updatedSyncOperation)
			Expect(err).To(BeNil())

			err = dbq.GetSyncOperationById(ctx, &fetchRow)
			Expect(err).To(BeNil())
			Expect(fetchRow.DesiredState).Should(Equal(updatedSyncOperation.DesiredState))

			rowCount, err := dbq.DeleteSyncOperationById(ctx, insertRow.SyncOperationID)
			Expect(err).To(BeNil())
			Expect(rowCount).Should(Equal(1))
			fetchRow = db.SyncOperation{
				SyncOperationID: "test-sync",
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
