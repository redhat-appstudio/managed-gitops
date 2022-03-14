package shared_resource_loop_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSharedResourceLoop(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SharedResourceLoop Suite")
}
