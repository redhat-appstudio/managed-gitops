package util

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestUtil(t *testing.T) {

	_, reporterConfig := GinkgoConfiguration()

	reporterConfig.SlowSpecThreshold = time.Duration(30 * time.Second)

	// Enable controller-runtime log output
	opts := zap.Options{
		Development: true,
		Level:       zapcore.DebugLevel,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	RegisterFailHandler(Fail)
	RunSpecs(t, "TaskRetryLoop Suite")
}
