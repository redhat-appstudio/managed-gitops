package util

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
)

func TestUtil(t *testing.T) {
	config.DefaultReporterConfig.SlowSpecThreshold = time.Hour.Seconds() * 30

	RegisterFailHandler(Fail)
	RunSpecs(t, "TaskRetryLoop Suite")
}
