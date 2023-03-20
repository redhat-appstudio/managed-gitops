package metrics

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Test for Operation DB metrics counter", func() {
	Context("Prometheus metrics responds to count of operation DB rows", func() {

		BeforeEach(func() {
			NumberOfSucceededOperations_currentCount = 0
			NumberOfFailedOperations_currentCount = 0
		})

		It("tests SetOperationDBState function on operation DB rows", func() {

			By("verify IncreaseOperationDBState by passing state as Completed")
			IncreaseOperationDBState(db.OperationState_Completed)

			Expect(int(NumberOfSucceededOperations_currentCount)).To(Equal(1))

			By("verify IncreaseOperationDBState by passing state as Failed")
			IncreaseOperationDBState(db.OperationState_Failed)

			Expect(int(NumberOfFailedOperations_currentCount)).To(Equal(1))

		})
	})
})
