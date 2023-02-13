package addtestvalues

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInitializeValues(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test for initializing db values Suite")
}
