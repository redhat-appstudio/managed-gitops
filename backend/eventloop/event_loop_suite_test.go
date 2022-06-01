package eventloop

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEventLoop(t *testing.T) {
	RegisterFailHandler(Fail)

	_, reporterConfig := GinkgoConfiguration()

	reporterConfig.SlowSpecThreshold = time.Duration(6 * time.Second)

	RunSpecs(t, "Event Loop Tests")
}
