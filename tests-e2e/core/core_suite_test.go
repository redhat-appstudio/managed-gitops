package core

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
)

func TestCore(t *testing.T) {
	// A test is "slow" if it takes longer than a minute
	config.DefaultReporterConfig.SlowSpecThreshold = time.Hour.Seconds() * 60
	RegisterFailHandler(Fail)
	RunSpecs(t, "Core Suite")
}
