package db_test

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Operations Test", func() {
	var timestamp = time.Date(2022, time.March, 11, 12, 3, 49, 514935000, time.UTC)
	var seq = 101

	var (
		gitopsEngineInstance *db.GitopsEngineInstance
		dbq                  db.AllDatabaseQueries
		testClusterUser      = &db.ClusterUser{
			ClusterUserID: "test-user-1",
			UserName:      "test-user-1",
		}

		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())

		_, _, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
		Expect(err).To(BeNil())

		err = dbq.CreateClusterUser(ctx, testClusterUser)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		defer dbq.CloseDatabase()
	})

	It("Should Create, Get, List, Update and Delete an Operation", func() {
		operation := db.Operation{
			Operation_id:         "test-operation-1",
			InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
			ResourceID:           "test-fake-resource-id",
			ResourceType:         "GitopsEngineInstance",
			State:                db.OperationState_Waiting,
			OperationOwnerUserID: testClusterUser.ClusterUserID,
		}

		err := dbq.CreateOperation(ctx, &operation, operation.OperationOwnerUserID)
		Expect(err).To(BeNil())

		operationget := db.Operation{
			Operation_id: operation.Operation_id,
		}

		err = dbq.GetOperationById(ctx, &operationget)
		Expect(err).To(BeNil())
		Expect(operationget.LastStateUpdate).To(BeAssignableToTypeOf(timestamp))
		Expect(operationget.CreatedOn).To(BeAssignableToTypeOf(timestamp))
		operationget.CreatedOn = operation.CreatedOn
		operationget.LastStateUpdate = operation.LastStateUpdate
		Expect(operation).Should(Equal(operationget))

		var operationlist []db.Operation

		err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, operation.ResourceID, operation.ResourceType, &operationlist, operation.OperationOwnerUserID)
		Expect(err).To(BeNil())
		Expect(operationlist[0].LastStateUpdate).To(BeAssignableToTypeOf(timestamp))
		Expect(operationlist[0].CreatedOn).To(BeAssignableToTypeOf(timestamp))
		operationlist[0].CreatedOn = operation.CreatedOn
		operationlist[0].LastStateUpdate = operation.LastStateUpdate

		Expect(operationlist[0]).Should(Equal(operation))
		Expect(len(operationlist)).Should(Equal(1))

		operationupdate := db.Operation{
			Operation_id:         "test-operation-1",
			InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
			ResourceID:           "test-fake-resource-id-update",
			ResourceType:         "GitopsEngineInstance-update",
			State:                db.OperationState_Waiting,
			OperationOwnerUserID: testClusterUser.ClusterUserID,
			SeqID:                int64(seq),
		}
		operationupdate.CreatedOn = operation.CreatedOn
		operationupdate.LastStateUpdate = operation.LastStateUpdate
		err = dbq.UpdateOperation(ctx, &operationupdate)
		Expect(err).To(BeNil())
		err = dbq.GetOperationById(ctx, &operationupdate)
		Expect(err).To(BeNil())
		Expect(operationupdate).ShouldNot(Equal(operation))

		rowsAffected, err := dbq.DeleteOperationById(ctx, operationget.Operation_id)
		Expect(err).To(BeNil())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetOperationById(ctx, &operationget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		operationNotExist := db.Operation{Operation_id: "test-operation-1-not-exist"}
		err = dbq.GetOperationById(ctx, &operationNotExist)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		operation.Operation_id = strings.Repeat("abc", 100)
		err = dbq.CreateOperation(ctx, &operation, operation.OperationOwnerUserID)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

		operationget.OperationOwnerUserID = strings.Repeat("abc", 100)
		err = dbq.UpdateOperation(ctx, &operationget)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})

	Context("list all operations to be garbage collected", func() {
		var sampleOperation *db.Operation
		var validOperations []db.Operation

		BeforeEach(func() {
			By("create a sample operation")
			sampleOperation = &db.Operation{
				Operation_id:         "test-operation-1",
				InstanceID:           gitopsEngineInstance.Gitopsengineinstance_id,
				ResourceID:           "test-fake-resource-id",
				ResourceType:         "GitopsEngineInstance",
				OperationOwnerUserID: testClusterUser.ClusterUserID,
				LastStateUpdate:      time.Now(),
			}

			err := dbq.CreateOperation(ctx, sampleOperation, sampleOperation.OperationOwnerUserID)
			Expect(err).To(BeNil())
		})

		It("operation in waiting state shouldn't be returned", func() {
			Expect(sampleOperation.State).Should(Equal(db.OperationState_Waiting))
			err := dbq.ListOperationsToBeGarbageCollected(ctx, &validOperations)
			Expect(err).To(BeNil())

			Expect(len(validOperations)).Should(Equal(0))
		})

		It("operation without gc time shouldn't be returned", func() {
			err := dbq.GetOperationById(ctx, sampleOperation)
			Expect(err).To(BeNil())

			sampleOperation.State = db.OperationState_Completed
			err = dbq.UpdateOperation(ctx, sampleOperation)
			Expect(err).To(BeNil())

			err = dbq.ListOperationsToBeGarbageCollected(ctx, &validOperations)
			Expect(err).To(BeNil())

			Expect(len(validOperations)).Should(Equal(0))
		})

		It("operation in completed state and non-zero gc time should be returned", func() {
			err := dbq.GetOperationById(ctx, sampleOperation)
			Expect(err).To(BeNil())

			sampleOperation.State = db.OperationState_Completed
			sampleOperation.GCExpirationTime = 100
			err = dbq.UpdateOperation(ctx, sampleOperation)
			Expect(err).To(BeNil())

			err = dbq.ListOperationsToBeGarbageCollected(ctx, &validOperations)
			Expect(err).To(BeNil())

			Expect(len(validOperations)).Should(Equal(1))
			Expect(sampleOperation).Should(readyForGarbageCollection())
		})

		It("operation in failed state and non-zero gc time should be returned", func() {
			err := dbq.GetOperationById(ctx, sampleOperation)
			Expect(err).To(BeNil())

			sampleOperation.State = db.OperationState_Failed
			sampleOperation.GCExpirationTime = 100
			err = dbq.UpdateOperation(ctx, sampleOperation)
			Expect(err).To(BeNil())

			err = dbq.ListOperationsToBeGarbageCollected(ctx, &validOperations)
			Expect(err).To(BeNil())

			Expect(len(validOperations)).Should(Equal(1))
			Expect(sampleOperation).Should(readyForGarbageCollection())
		})

	})
})

func readyForGarbageCollection() types.GomegaMatcher {
	return WithTransform(func(operation *db.Operation) bool {
		return operation.GCExpirationTime > 0 && (operation.State == db.OperationState_Completed || operation.State == db.OperationState_Failed)
	}, BeTrue())
}
