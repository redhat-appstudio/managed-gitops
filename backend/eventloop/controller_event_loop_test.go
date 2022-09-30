package eventloop

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Controller Event Loop Test", func() {

	Context("Controller event loop responds to channel events", func() {

		It("Should pass events received on input channel to mock workspace event loop router", func() {

			mockOutputChannelFactory := &mockWorkspaceEventLoopFactory{
				mockChannel: make(chan workspaceEventLoopMessage),
			}

			loop := newControllerEventLoopWithFactory(mockOutputChannelFactory)

			loop.EventLoopInputChannel <- eventlooptypes.EventLoopEvent{
				EventType: eventlooptypes.DeploymentModified,
				Request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "",
						Name:      "",
					},
				},
				Client:      nil,
				ReqResource: eventlooptypes.GitOpsDeploymentTypeName,
				WorkspaceID: "",
			}

			resp := <-mockOutputChannelFactory.mockChannel
			Expect(resp).NotTo(BeNil())

		})
	})
})

// mockWorkspaceEventLoopFactory is a mock of workspaceEventLoopRouterFactory
type mockWorkspaceEventLoopFactory struct {
	mockChannel chan workspaceEventLoopMessage
}

var _ workspaceEventLoopRouterFactory = &mockWorkspaceEventLoopFactory{}

func (cetf *mockWorkspaceEventLoopFactory) startWorkspaceEventLoopRouter(workspaceID string) WorkspaceEventLoopRouterStruct {
	// Rather than starting a new workspace event loop, instead just return a pre-provided channel
	return WorkspaceEventLoopRouterStruct{
		channel: cetf.mockChannel,
	}
}
