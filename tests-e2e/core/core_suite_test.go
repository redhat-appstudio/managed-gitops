package core

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
})

func TestCore(t *testing.T) {
	// A test is "slow" if it takes longer than a minute
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.SlowSpecThreshold = time.Duration(60 * time.Second)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Core Suite")
}
