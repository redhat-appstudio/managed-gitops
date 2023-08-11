package eventloop

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("isManagedEnvironmentConnectionUserError function Test", func() {
	Context("Testing isManagedEnvironmentConnectionUserError function.", func() {

		It("should return True if error is related to connection issue.", func() {
			ctx := context.Background()
			log := log.FromContext(ctx)

			rootErr := fmt.Errorf("Get \"https://api.ireland.burr-on-aws.com:6443/api?timeout=32s\": dial tcp: lookup api.ireland.burr-on-aws.com on 172.30.0.10:53: no such host")
			err := fmt.Errorf("%s: %w", shared_resource_loop.UnableToCreateRestConfigError, rootErr)

			result := isManagedEnvironmentConnectionUserError(err, log)
			Expect(result).To(BeTrue())
		})

		It("should return False if error is not related to connection issue.", func() {
			ctx := context.Background()
			log := log.FromContext(ctx)

			rootErr := fmt.Errorf("some error")
			err := fmt.Errorf("another error: %w", rootErr)

			result := isManagedEnvironmentConnectionUserError(err, log)
			Expect(result).To(BeFalse())
		})
	})
})
