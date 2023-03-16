package metrics

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

var _ = Describe("Test for Operation DB metrics counter", func() {
	Context("Prometheus metrics responds to count of operation DB rows", func() {

		BeforeEach(func() {
			ClearDBMetrics()
		})

		It("Test SetTotalCountOfOperationDBRows function on operation DB rows", func() {

			totalNumberOfOperationDBRows := testutil.ToFloat64(OperationDBRows)

			By("verify SetTotalCountOfOperationDBRows function by passing count as 4")
			SetTotalCountOfOperationDBRows(4)

			newTotalNumberOfOperationDBRows := testutil.ToFloat64(OperationDBRows)

			Expect(newTotalNumberOfOperationDBRows).To(Equal(totalNumberOfOperationDBRows + 4))

		})

		It("Test SetCountOfOperationDBRows function on operation DB row for Waiting State", func() {

			numberOfOperationDBRowsInWaitingState := testutil.ToFloat64(OperationDBRowsInWaitingState)

			By("verify SetCountOfOperationDBRows function by passing state as 'Waiting' and count as 2")
			SetCountOfOperationDBRows("Waiting", 2)

			newNumberOfOperationDBRowsInWaitingState := testutil.ToFloat64(OperationDBRowsInWaitingState)

			Expect(newNumberOfOperationDBRowsInWaitingState).To(Equal(numberOfOperationDBRowsInWaitingState + 2))

		})

		It("Test SetCountOfOperationDBRows function on operation DB row for In_Progress State", func() {

			numberOfOperationDBRowsIn_InProgressState := testutil.ToFloat64(OperationDBRowsIn_InProgressState)

			By("verify SetCountOfOperationDBRows function by passing state as 'In_Progress' and count as 2")
			SetCountOfOperationDBRows("In_Progress", 2)

			newNumberOfOperationDBRowsIn_InProgressState := testutil.ToFloat64(OperationDBRowsIn_InProgressState)

			Expect(newNumberOfOperationDBRowsIn_InProgressState).To(Equal(numberOfOperationDBRowsIn_InProgressState + 2))

		})

		It("Test SetCountOfOperationDBRows function on operation DB row for Completed State", func() {

			numberOfOperationDBRowsInCompletedState := testutil.ToFloat64(OperationDBRowsInCompletedState)
			By("verify SetCountOfOperationDBRows function by passing state as 'In_Progress' and count as 2")
			SetCountOfOperationDBRows("Completed", 2)

			newNumberOfOperationDBRowsInCompletedState := testutil.ToFloat64(OperationDBRowsInCompletedState)

			Expect(newNumberOfOperationDBRowsInCompletedState).To(Equal(numberOfOperationDBRowsInCompletedState + 2))

		})

		It("Test SetCountOfOperationDBRows function on operation DB row for Failed State", func() {

			numberOfOperationDBRowsInFailedState := testutil.ToFloat64(OperationDBRowsInErrorState)

			By("verify SetCountOfOperationDBRows function by passing state as 'Failed' and count as 2")
			SetCountOfOperationDBRows("Failed", 2)

			newNumberOfOperationDBRowsInFailedState := testutil.ToFloat64(OperationDBRowsInErrorState)

			Expect(newNumberOfOperationDBRowsInFailedState).To(Equal(numberOfOperationDBRowsInFailedState + 2))

		})

		It("Test SetCountOfOperationDBRowsInCompleteState function on operation DB rows", func() {
			totalNumberOfOperationDBRowsInCompletedState := testutil.ToFloat64(TotalOperationDBRowsInCompletedState)

			SetCountOfOperationDBRowsInCompleteState(2)

			newTotalNumberOfOperationDBRowsInCompletedState := testutil.ToFloat64(TotalOperationDBRowsInCompletedState)

			Expect(newTotalNumberOfOperationDBRowsInCompletedState).To(Equal(totalNumberOfOperationDBRowsInCompletedState + 2))

		})

		It("Test SetCountOfOperationDBRowsInNonCompleteState function on operation DB rows", func() {

			totalNumberOfOperationDBRowsInNonCompleteState := testutil.ToFloat64(TotalOperationDBRowsInNonCompleteState)

			SetCountOfOperationDBRowsInNonCompleteState(2)

			newTotalNumberOfOperationDBRowsInNonCompleteState := testutil.ToFloat64(TotalOperationDBRowsInNonCompleteState)

			Expect(newTotalNumberOfOperationDBRowsInNonCompleteState).To(Equal(totalNumberOfOperationDBRowsInNonCompleteState + 2))

		})

	})
})
