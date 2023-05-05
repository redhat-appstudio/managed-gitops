package util

import (
	"flag"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestUtil(t *testing.T) {

	suiteConfig, _ := GinkgoConfiguration()

	// Define a flag for the poll progress after interval
	var pollProgressAfter time.Duration
	// A test is "slow" if it takes longer than a few minutes
	flag.DurationVar(&pollProgressAfter, "poll-progress-after", 30*time.Second, "Interval for polling progress after")

	// Parse the flags
	flag.Parse()

	// Set the poll progress after interval in the suite configuration
	suiteConfig.PollProgressAfter = pollProgressAfter

	// Enable controller-runtime log output
	opts := zap.Options{
		Development: true,
		Level:       zapcore.DebugLevel,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	RegisterFailHandler(Fail)
	RunSpecs(t, "TaskRetryLoop Suite")
}
