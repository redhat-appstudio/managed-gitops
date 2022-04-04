package util

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestUtil(t *testing.T) {
	config.DefaultReporterConfig.SlowSpecThreshold = time.Hour.Seconds() * 30

	// Enable controller-runtime log output
	opts := zap.Options{
		Development: true,
		Level:       zapcore.DebugLevel,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	RegisterFailHandler(Fail)
	RunSpecs(t, "TaskRetryLoop Suite")
}
