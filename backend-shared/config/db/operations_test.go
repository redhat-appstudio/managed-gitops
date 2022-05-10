package db_test

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("Operations Test", func() {
	var timestamp = time.Date(2022, time.March, 11, 12, 3, 49, 514935000, time.UTC)
	var seq = 101
	It("Should Create, Get, List, Update and Delete an Operation", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

		_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
		Expect(err).To(BeNil())
		var testClusterUser = &db.ClusterUser{
			Clusteruser_id: "test-user-1",
			User_name:      "test-user-1",
		}
		operation := db.Operation{
			Operation_id:            "test-operation-1",
			Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
			Resource_id:             "test-fake-resource-id",
			Resource_type:           "GitopsEngineInstance",
			State:                   db.OperationState_Waiting,
			Operation_owner_user_id: testClusterUser.Clusteruser_id,
		}
		err = dbq.CreateClusterUser(ctx, testClusterUser)
		Expect(err).To(BeNil())

		err = dbq.CreateOperation(ctx, &operation, operation.Operation_owner_user_id)
		Expect(err).To(BeNil())

		operationget := db.Operation{
			Operation_id: operation.Operation_id,
		}

		err = dbq.GetOperationById(ctx, &operationget)
		Expect(err).To(BeNil())
		Expect(operationget.Last_state_update).To(BeAssignableToTypeOf(timestamp))
		Expect(operationget.Created_on).To(BeAssignableToTypeOf(timestamp))
		operationget.Created_on = operation.Created_on
		operationget.Last_state_update = operation.Last_state_update
		Expect(operation).Should(Equal(operationget))

		var operationlist []db.Operation

		err = dbq.ListOperationsByResourceIdAndTypeAndOwnerId(ctx, operation.Resource_id, operation.Resource_type, &operationlist, operation.Operation_owner_user_id)
		Expect(err).To(BeNil())
		Expect(operationlist[0].Last_state_update).To(BeAssignableToTypeOf(timestamp))
		Expect(operationlist[0].Created_on).To(BeAssignableToTypeOf(timestamp))
		operationlist[0].Created_on = operation.Created_on
		operationlist[0].Last_state_update = operation.Last_state_update

		Expect(operationlist[0]).Should(Equal(operation))
		Expect(len(operationlist)).Should(Equal(1))

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
		err = dbq.CreateOperation(ctx, &operation, operation.Operation_owner_user_id)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

		operationget.Operation_owner_user_id = strings.Repeat("abc", 100)
		err = dbq.UpdateOperation(ctx, &operationget)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})
})
