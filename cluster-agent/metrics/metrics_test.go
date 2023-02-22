package metrics

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Test for Operation DB metrics counter", func() {
	Context("Prometheus metrics responds to count of operation DB rows", func() {
		It("tests SetOperationDBState function on operation DB rows", func() {

			ClearMetrics()

			numberOfOperationDBRowsInCompletedSate := testutil.ToFloat64(OperationStateCompleted)
			numberOfOperationDBRowsInFailedState := testutil.ToFloat64(OperationStateFailed)

			By("verify SetOperationDBState by passing state as Completed")
			SetOperationDBState(db.OperationState_Completed)

			newNumberOfOperationDBRowsInCompletedSate := testutil.ToFloat64(OperationStateCompleted)
			newNumberOfOperationDBRowsInFailedState := testutil.ToFloat64(OperationStateFailed)
			Expect(newNumberOfOperationDBRowsInCompletedSate).To(Equal(numberOfOperationDBRowsInCompletedSate + 1))
			Expect(newNumberOfOperationDBRowsInFailedState).To(Equal(numberOfOperationDBRowsInFailedState))

			By("verify SetOperationDBState by passing state as Failed")
			SetOperationDBState(db.OperationState_Failed)

			newNumberOfOperationDBRowsInCompletedSate = testutil.ToFloat64(OperationStateCompleted)
			newNumberOfOperationDBRowsInFailedState = testutil.ToFloat64(OperationStateFailed)
			Expect(newNumberOfOperationDBRowsInCompletedSate).To(Equal(numberOfOperationDBRowsInCompletedSate + 1))
			Expect(newNumberOfOperationDBRowsInFailedState).To(Equal(numberOfOperationDBRowsInFailedState + 1))

		})
	})
})
