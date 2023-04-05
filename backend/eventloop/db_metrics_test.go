package eventloop

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend/metrics"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Metrics DB Reconciler Test", func() {
	Context("Testing Reconcile for OperationDB table entries.", func() {

		var log logr.Logger
		var ctx context.Context
		var dbq db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var gitopsEngineInstance *db.GitopsEngineInstance
		var clusterAccess *db.ClusterAccess

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx = context.Background()
			log = logger.FromContext(ctx)
			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, _, _, gitopsEngineInstance, clusterAccess, err = db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

		})

		It("should count the total number of operation DB rows and count number of operation DB rows based on Operation states", func() {
			defer dbq.CloseDatabase()

			metrics.ClearDBMetrics()

			totalNumberOfOperationDBRows := testutil.ToFloat64(metrics.OperationDBRows)
			numberOfOperationDBRowsInWaitingState := testutil.ToFloat64(metrics.OperationDBRowsInWaitingState)
			numberOfOperationDBRowsIn_InProgressState := testutil.ToFloat64(metrics.OperationDBRowsIn_InProgressState)
			numberOfOperationDBRowsInCompletedState := testutil.ToFloat64(metrics.OperationDBRowsInCompletedState)
			numberOfOperationDBRowsInFailedState := testutil.ToFloat64(metrics.OperationDBRowsInErrorState)
			totalNumberOfOperationDBRowsInCompletedState := testutil.ToFloat64(metrics.TotalOperationDBRowsInCompletedState)
			totalNumberOfOperationDBRowsInNonCompleteState := testutil.ToFloat64(metrics.TotalOperationDBRowsInNonCompleteState)

			By("created four operation rows in database to test the count through the metrics")
			OperationDB1 := db.Operation{
				Operation_id:            "test-operation-1",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: clusterAccess.Clusteraccess_user_id,
				Last_state_update:       time.Now(),
			}
			err := dbq.CreateOperation(ctx, &OperationDB1, OperationDB1.Operation_owner_user_id)
			Expect(err).To(BeNil())

			OperationDB2 := db.Operation{
				Operation_id:            "test-operation-2",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: clusterAccess.Clusteraccess_user_id,
				Last_state_update:       time.Now(),
			}
			err = dbq.CreateOperation(ctx, &OperationDB2, OperationDB2.Operation_owner_user_id)
			Expect(err).To(BeNil())

			OperationDB2.State = db.OperationState_In_Progress
			err = dbq.UpdateOperation(ctx, &OperationDB2)
			Expect(err).To(BeNil())

			OperationDB3 := db.Operation{
				Operation_id:            "test-operation-3",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: clusterAccess.Clusteraccess_user_id,
				Last_state_update:       time.Now(),
			}
			err = dbq.CreateOperation(ctx, &OperationDB3, OperationDB3.Operation_owner_user_id)
			Expect(err).To(BeNil())

			err = dbq.GetOperationById(ctx, &OperationDB3)
			Expect(err).To(BeNil())

			OperationDB3.State = db.OperationState_Completed
			err = dbq.UpdateOperation(ctx, &OperationDB3)
			Expect(err).To(BeNil())

			OperationDB4 := db.Operation{
				Operation_id:            "test-operation-4",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: clusterAccess.Clusteraccess_user_id,
				Last_state_update:       time.Now(),
			}
			err = dbq.CreateOperation(ctx, &OperationDB4, OperationDB4.Operation_owner_user_id)
			Expect(err).To(BeNil())

			err = dbq.GetOperationById(ctx, &OperationDB3)
			Expect(err).To(BeNil())

			OperationDB4.State = db.OperationState_Failed
			err = dbq.UpdateOperation(ctx, &OperationDB4)
			Expect(err).To(BeNil())

			operationDbReconcile(ctx, dbq, k8sClient, log)

			newTotalNumberOfOperationDBRows := testutil.ToFloat64(metrics.OperationDBRows)
			newNumberOfOperationDBRowsInWaitingState := testutil.ToFloat64(metrics.OperationDBRowsInWaitingState)
			newNumberOfOperationDBRowsIn_InProgressState := testutil.ToFloat64(metrics.OperationDBRowsIn_InProgressState)
			newNumberOfOperationDBRowsInCompletedState := testutil.ToFloat64(metrics.OperationDBRowsInCompletedState)
			newNumberOfOperationDBRowsInFailedState := testutil.ToFloat64(metrics.OperationDBRowsInErrorState)
			newTotalNumberOfOperationDBRowsInCompletedState := testutil.ToFloat64(metrics.TotalOperationDBRowsInCompletedState)
			newTotalNumberOfOperationDBRowsInNonCompleteState := testutil.ToFloat64(metrics.TotalOperationDBRowsInNonCompleteState)

			var operations, waiting, inProgress, completed, failed []db.Operation
			err = dbq.UnsafeListAllOperations(ctx, &operations)
			Expect(err).To(BeNil())

			for _, op := range operations {
				switch op.State {
				case db.OperationState_Waiting:
					waiting = append(waiting, op)
				case db.OperationState_In_Progress:
					inProgress = append(inProgress, op)
				case db.OperationState_Completed:
					completed = append(completed, op)
				case db.OperationState_Failed:
					failed = append(failed, op)
				}
			}

			Expect(newTotalNumberOfOperationDBRows).To(Equal(totalNumberOfOperationDBRows + float64(len(operations))))
			Expect(newNumberOfOperationDBRowsInWaitingState).To(Equal(numberOfOperationDBRowsInWaitingState + float64(len(waiting))))
			Expect(newNumberOfOperationDBRowsIn_InProgressState).To(Equal(numberOfOperationDBRowsIn_InProgressState + float64(len(inProgress))))
			Expect(newNumberOfOperationDBRowsInCompletedState).To(Equal(numberOfOperationDBRowsInCompletedState + float64(len(completed))))
			Expect(newNumberOfOperationDBRowsInFailedState).To(Equal(numberOfOperationDBRowsInFailedState + float64(len(failed))))
			Expect(newTotalNumberOfOperationDBRowsInCompletedState).To(Equal(totalNumberOfOperationDBRowsInCompletedState + float64(len(completed)) + float64(len(failed))))
			Expect(newTotalNumberOfOperationDBRowsInNonCompleteState).To(Equal(totalNumberOfOperationDBRowsInNonCompleteState + float64(len(waiting)) + float64(len(inProgress))))

		})

	})
})
