package hack

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHack(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hack Suite")
}
