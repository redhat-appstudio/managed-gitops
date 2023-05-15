package hotfix

import (
	"flag"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))
})

func TestHotfix(t *testing.T) {

	suiteConfig, _ := GinkgoConfiguration()

	// Define a flag for the poll progress after interval
	var pollProgressAfter time.Duration
	flag.DurationVar(&pollProgressAfter, "poll-progress-after", 1*time.Minute, "Interval for polling progress after")

	// Parse the flags
	flag.Parse()

	// Set the poll progress after interval in the suite configuration
	suiteConfig.PollProgressAfter = pollProgressAfter

	RegisterFailHandler(Fail)
	RunSpecs(t, "Hotfix Suite", suiteConfig)
}
