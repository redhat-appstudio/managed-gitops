package eventloop

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Workspace Event Loop Test", func() {

	Context("Workspace event loop responds to channel events", func() {

		It("Should pass events received on input channel to application event loop", func() {
			scheme, argocdNamespace, kubesystemNamespace, apiNamespace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			tAELF := &testApplicationEventLoopFactory{
				outputChannel: make(chan eventlooptypes.EventLoopMessage),
			}

			// Start the workspace event loop with our custom test factory, so that we can capture output
			inputChannel := make(chan eventlooptypes.EventLoopMessage)
			internalStartWorkspaceEventLoopRouter(inputChannel, string(apiNamespace.UID), tAELF)

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: apiNamespace.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, apiNamespace, argocdNamespace, kubesystemNamespace).Build()

			// Simulate a GitOpsDeployment modified event
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Name,
							Name:      apiNamespace.Name,
						},
					},
					Client:                  k8sClientOuter,
					ReqResource:             managedgitopsv1alpha1.GitOpsDeploymentTypeName,
					AssociatedGitopsDeplUID: string(gitopsDepl.UID),
					WorkspaceID:             string(apiNamespace.UID),
				},
			}

			// Sending an event to the workspace event loop should cause an event to be sent on the output channel
			inputChannel <- msg
			result := <-tAELF.outputChannel

			Expect(result).NotTo(BeNil())
			Expect(result.Event).To(Equal(msg.Event))
			Expect(result.MessageType).To(Equal(msg.MessageType))

		})

	})
})

// testApplicationEventLoopFactory is a mock applicationEventQueueLoopFactory, used for unit tests above.
type testApplicationEventLoopFactory struct {
	outputChannel chan eventlooptypes.EventLoopMessage
}

var _ applicationEventQueueLoopFactory = &testApplicationEventLoopFactory{}

// Instead of starting a new application event queue loop (like the default implementation of applictionEventQueueLoopFactory)
// we instead just return a previously provided channel.
func (ta *testApplicationEventLoopFactory) startApplicationEventQueueLoop(gitopsDeplID string, workspaceID string,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop) chan eventlooptypes.EventLoopMessage {

	return ta.outputChannel
}
