package metrics

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

var _ = Describe("Test for Operation CR metrics counter", func() {

	Context("Prometheus metrics responds to count of operation CRs on a cluster", func() {

		It("Test SetNumberOfOperationsCR function", func() {

			ClearOperationMetrics()

			numberOfOperationsCRMetrics := testutil.ToFloat64(OperationCR)

			SetNumberOfOperationsCR(2)

			newNumberOfOperationsCRMetrics := testutil.ToFloat64(OperationCR)

			Expect(newNumberOfOperationsCRMetrics).To(Equal(numberOfOperationsCRMetrics + 2))

		})
	})
})
