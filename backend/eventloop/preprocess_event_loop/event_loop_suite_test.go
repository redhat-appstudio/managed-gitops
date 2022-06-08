package preprocess_event_loop

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEventLoop(t *testing.T) {
	RegisterFailHandler(Fail)

	_, reporterConfig := GinkgoConfiguration()

	reporterConfig.SlowSpecThreshold = time.Duration(10 * time.Second)

	RunSpecs(t, "Pre-process Event Loop Tests")
}
