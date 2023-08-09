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
			Clusteruser_id: "test-user-1",
			User_name:      "test-user-1",
		}

		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		_, _, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateClusterUser(ctx, testClusterUser)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		defer dbq.CloseDatabase()
	})

	It("Should Create, Get, List, Update and Delete an Operation", func() {
		operation := db.Operation{
			Operation_id:            "test-operation-1",
			Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
			Resource_id:             "test-fake-resource-id",
			Resource_type:           "GitopsEngineInstance",
			State:                   db.OperationState_Waiting,
			Operation_owner_user_id: testClusterUser.Clusteruser_id,
		}

		err := dbq.CreateOperation(ctx, &operation, operation.Operation_owner_user_id)
		Expect(err).ToNot(HaveOccurred())

		operationget := db.Operation{
			Operation_id: operation.Operation_id,
		}

		err = dbq.GetOperationById(ctx, &operationget)
		Expect(err).ToNot(HaveOccurred())
		Expect(operationget.Last_state_update).To(BeAssignableToTypeOf(timestamp))
		Expect(operationget.Created_on).To(BeAssignableToTypeOf(timestamp))
		operationget.Created_on = operation.Created_on
		operationget.Last_state_update = operation.Last_state_update
		Expect(operation).Should(Equal(operationget))

		var operationlist []db.Operation

		err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, operation.Resource_id, operation.Resource_type, &operationlist, operation.Operation_owner_user_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(operationlist[0].Last_state_update).To(BeAssignableToTypeOf(timestamp))
		Expect(operationlist[0].Created_on).To(BeAssignableToTypeOf(timestamp))
		operationlist[0].Created_on = operation.Created_on
		operationlist[0].Last_state_update = operation.Last_state_update

		Expect(operationlist[0]).Should(Equal(operation))
		Expect(operationlist).Should(HaveLen(1))

		operationupdate := db.Operation{
			Operation_id:            "test-operation-1",
			Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
			Resource_id:             "test-fake-resource-id-update",
			Resource_type:           "GitopsEngineInstance-update",
			State:                   db.OperationState_Waiting,
			Operation_owner_user_id: testClusterUser.Clusteruser_id,
			SeqID:                   int64(seq),
		}
		operationupdate.Created_on = operation.Created_on
		operationupdate.Last_state_update = operation.Last_state_update
		err = dbq.UpdateOperation(ctx, &operationupdate)
		Expect(err).ToNot(HaveOccurred())
		err = dbq.GetOperationById(ctx, &operationupdate)
		Expect(err).ToNot(HaveOccurred())
		Expect(operationupdate).ShouldNot(Equal(operation))

		rowsAffected, err := dbq.DeleteOperationById(ctx, operationget.Operation_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetOperationById(ctx, &operationget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		operationNotExist := db.Operation{Operation_id: "test-operation-1-not-exist"}
		err = dbq.GetOperationById(ctx, &operationNotExist)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		operation.Operation_id = strings.Repeat("abc", 100)
		err = dbq.CreateOperation(ctx, &operation, operation.Operation_owner_user_id)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

		operationget.Operation_owner_user_id = strings.Repeat("abc", 100)
		err = dbq.UpdateOperation(ctx, &operationget)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})

	Context("list all operations to be garbage collected", func() {
		var sampleOperation *db.Operation
		var validOperations []db.Operation

		BeforeEach(func() {
			By("create a sample operation")
			sampleOperation = &db.Operation{
				Operation_id:            "test-operation-1",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
				Last_state_update:       time.Now(),
			}

			err := dbq.CreateOperation(ctx, sampleOperation, sampleOperation.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())
		})

		It("operation in waiting state shouldn't be returned", func() {
			Expect(sampleOperation.State).Should(Equal(db.OperationState_Waiting))
			err := dbq.ListOperationsToBeGarbageCollected(ctx, &validOperations)
			Expect(err).ToNot(HaveOccurred())

			Expect(validOperations).Should(BeEmpty())
		})

		It("operation without gc time shouldn't be returned", func() {
			err := dbq.GetOperationById(ctx, sampleOperation)
			Expect(err).ToNot(HaveOccurred())

			sampleOperation.State = db.OperationState_Completed
			err = dbq.UpdateOperation(ctx, sampleOperation)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.ListOperationsToBeGarbageCollected(ctx, &validOperations)
			Expect(err).ToNot(HaveOccurred())

			Expect(validOperations).Should(BeEmpty())
		})

		It("operation in completed state and non-zero gc time should be returned", func() {
			err := dbq.GetOperationById(ctx, sampleOperation)
			Expect(err).ToNot(HaveOccurred())

			sampleOperation.State = db.OperationState_Completed
			sampleOperation.GC_expiration_time = 100
			err = dbq.UpdateOperation(ctx, sampleOperation)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.ListOperationsToBeGarbageCollected(ctx, &validOperations)
			Expect(err).ToNot(HaveOccurred())

			Expect(validOperations).Should(HaveLen(1))
			Expect(sampleOperation).Should(readyForGarbageCollection())
		})

		It("operation in failed state and non-zero gc time should be returned", func() {
			err := dbq.GetOperationById(ctx, sampleOperation)
			Expect(err).ToNot(HaveOccurred())

			sampleOperation.State = db.OperationState_Failed
			sampleOperation.GC_expiration_time = 100
			err = dbq.UpdateOperation(ctx, sampleOperation)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.ListOperationsToBeGarbageCollected(ctx, &validOperations)
			Expect(err).ToNot(HaveOccurred())

			Expect(validOperations).Should(HaveLen(1))
			Expect(sampleOperation).Should(readyForGarbageCollection())
		})

	})

	Context("Test Dispose function for Operation", func() {
		var operation *db.Operation
		var dbq db.AllDatabaseQueries
		var err error
		BeforeEach(func() {
			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())

			operation = &db.Operation{
				Operation_id:            "test-operation-1",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
				Last_state_update:       time.Now(),
			}

			err = dbq.CreateOperation(ctx, operation, operation.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			defer dbq.CloseDatabase()
		})

		It("Should test Dispose function with missing database interface", func() {
			var dbq db.AllDatabaseQueries

			err := operation.DisposeAppScoped(ctx, dbq)
			Expect(err).To(HaveOccurred())
		})

		It("Should test Dispose function for Operation", func() {
			err := operation.DisposeAppScoped(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetOperationById(ctx, operation)
			Expect(err).To(HaveOccurred())

		})
	})
})

func readyForGarbageCollection() types.GomegaMatcher {
	return WithTransform(func(operation *db.Operation) bool {
		return operation.GC_expiration_time > 0 && (operation.State == db.OperationState_Completed || operation.State == db.OperationState_Failed)
	}, BeTrue())
}
