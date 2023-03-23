package argocdmetrics

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestArgoCDMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cluster-agent argocd metrics Suite")
}
