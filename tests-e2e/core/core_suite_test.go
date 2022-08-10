package core

import (
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	// GITOPS_IN_KCP is an environment variable that is set when running our e2e tests against KCP
	ENVGitOpsInKCP = "GITOPS_IN_KCP"
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))
})

func TestCore(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	// A test is "slow" if it takes longer than a few minutes
	reporterConfig.SlowSpecThreshold = time.Duration(6 * time.Minute)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Core Suite", reporterConfig)
}

func isRunningAgainstKCP() bool {
	return os.Getenv(ENVGitOpsInKCP) == "true"
}
