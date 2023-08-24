package application_info_cache

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApplicationInfoCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Application Info Cache Suite")
}
