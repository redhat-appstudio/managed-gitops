package gitopsservice

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInitializeValues(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test for investigating memory and CPU usage of GitOps Service")
}
