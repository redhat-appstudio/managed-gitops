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

			numberOfOperationDBRowsInCompletedState := testutil.ToFloat64(OperationStateCompleted)
			numberOfOperationDBRowsInFailedState := testutil.ToFloat64(OperationStateFailed)

			By("verify IncreaseOperationDBState by passing state as Completed")
			IncreaseOperationDBState(db.OperationState_Completed)

			newNumberOfOperationDBRowsInCompletedState := testutil.ToFloat64(OperationStateCompleted)
			newNumberOfOperationDBRowsInFailedState := testutil.ToFloat64(OperationStateFailed)
			Expect(newNumberOfOperationDBRowsInCompletedState).To(Equal(numberOfOperationDBRowsInCompletedState + 1))
			Expect(newNumberOfOperationDBRowsInFailedState).To(Equal(numberOfOperationDBRowsInFailedState))

			By("verify IncreaseOperationDBState by passing state as Failed")
			IncreaseOperationDBState(db.OperationState_Failed)

			newNumberOfOperationDBRowsInCompletedState = testutil.ToFloat64(OperationStateCompleted)
			newNumberOfOperationDBRowsInFailedState = testutil.ToFloat64(OperationStateFailed)
			Expect(newNumberOfOperationDBRowsInCompletedState).To(Equal(numberOfOperationDBRowsInCompletedState + 1))
			Expect(newNumberOfOperationDBRowsInFailedState).To(Equal(numberOfOperationDBRowsInFailedState + 1))

		})
	})
})
