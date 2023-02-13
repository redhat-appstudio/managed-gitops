package hotfix

import (
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

	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.SlowSpecThreshold = time.Duration(1 * time.Minute)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Hotfix Suite", reporterConfig)
}
