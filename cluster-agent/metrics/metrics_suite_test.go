package metrics

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDBMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cluster-agent metrics Suite")
}
