package utils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))
})

func TestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Utils Suite")
}
