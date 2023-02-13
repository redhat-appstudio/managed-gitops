package verifytestvalues

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVerifyDBValues(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test for verifying db values Suite")
}
