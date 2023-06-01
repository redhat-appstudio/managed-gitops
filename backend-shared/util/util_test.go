package util

import (
	"context"
	"os"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("General Util Unit Tests", func() {

	Context("Testing the SelfHealInterval() function", func() {
		const defaultValue = time.Duration(10) * time.Minute
		var logger logr.Logger

		BeforeEach(func() {
			logger = log.FromContext(context.Background())
		})

		When("Environment variable SELF_HEAL_INTERVAL is set to a number", func() {
			It("Should return a time.Duration of the equivalent number of minutes", func() {
				defer os.Unsetenv(SelfHealIntervalEnVar)

				os.Setenv(SelfHealIntervalEnVar, "5")
				actual := SelfHealInterval(defaultValue, logger)
				Expect(actual).To(Equal(time.Duration(5) * time.Minute))
			})
		})

		When("Environment variable SELF_HEAL_INTERVAL is set to something non-numeric", func() {
			It("Should return the default time.Duration", func() {
				defer os.Unsetenv(SelfHealIntervalEnVar)

				os.Setenv(SelfHealIntervalEnVar, "five")
				actual := SelfHealInterval(defaultValue, logger)
				Expect(actual).To(Equal(defaultValue))
			})
		})

		When("Environment variable SELF_HEAL_INTERVAL is not set", func() {
			It("Should return the default time.Duration", func() {
				_, isSet := os.LookupEnv(SelfHealIntervalEnVar)
				Expect(isSet).To(BeFalse())
				actual := SelfHealInterval(defaultValue, logger)
				Expect(actual).To(Equal(defaultValue))
			})
		})
	})
})
