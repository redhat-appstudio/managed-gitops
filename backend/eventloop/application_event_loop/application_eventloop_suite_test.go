package application_event_loop_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEventloop_application_event_runner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Application Event Loop Suite")
}
