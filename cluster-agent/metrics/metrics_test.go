package metrics

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Test for Operation DB metrics counter", func() {
	Context("Prometheus metrics responds to count of operation DB rows", func() {

		BeforeEach(func() {

			TestOnly_resetAllMetricsCount()

			failedVal := testutil.ToFloat64(OperationStateFailed)
			Expect(failedVal).To(Equal(float64(0)))

			completedVal := testutil.ToFloat64(OperationStateCompleted)
			Expect(completedVal).To(Equal(float64(0)))
		})

		It("tests IncreaseOperationDBState function on operation DB rows", func() {

			By("verify IncreaseOperationDBState by passing state as Completed")
			IncreaseOperationDBState(db.OperationState_Completed)
			Expect(int(numberOfSucceededOperations_currentCount)).To(Equal(1))

			runCollectOperationMetrics()

			completedVal := testutil.ToFloat64(OperationStateCompleted)
			Expect(completedVal).To(Equal(float64(1)))

		})

		It("tests SetOperationDBState function on operation DB rows", func() {

			By("verify IncreaseOperationDBState by passing state as Failed")
			IncreaseOperationDBState(db.OperationState_Failed)

			Expect(int(numberOfFailedOperations_currentCount)).To(Equal(1))

			runCollectOperationMetrics()

			failedVal := testutil.ToFloat64(OperationStateFailed)
			Expect(failedVal).To(Equal(float64(1)))

		})

	})
})
