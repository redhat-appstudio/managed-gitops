package eventloop

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Preprocess Event Loop Test", func() {

	Context("Preprocess event loop responds to channel events", func() {

		It("Should pass events received on input channel to application event loop", func() {

			ctx := context.Background()

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			informer := sharedutil.ListEventReceiver{}

			k8sClientOuter := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()
			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
				Informer:    &informer,
			}

			fakeEventLoopChannel := make(chan eventlooptypes.EventLoopEvent)
			fakeEventLoop := controllerEventLoop{
				eventLoopInputChannel: fakeEventLoopChannel,
			}

			channel := make(chan eventlooptypes.EventLoopEvent)

			go preprocessEventLoopRouter(channel, &fakeEventLoop)

			event := eventlooptypes.EventLoopEvent{
				EventType: eventlooptypes.DeploymentModified,
				Request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: gitopsDepl.Namespace,
						Name:      gitopsDepl.Name,
					},
				},
				ReqResource: managedgitopsv1alpha1.GitOpsDeploymentTypeName,
				Client:      k8sClient,
				WorkspaceID: eventlooptypes.GetWorkspaceIDFromNamespaceID(*workspace),
			}

			fmt.Println("Sending event", event)
			channel <- event

			fmt.Println("Waiting for response")
			response := <-fakeEventLoop.eventLoopInputChannel
			Expect(response.Request).Should(Equal(event.Request))
			Expect(response.EventType).Should(Equal(event.EventType))
			Expect(response.WorkspaceID).Should(Equal(event.WorkspaceID))
			fmt.Println("Received response", response)

			err = k8sClient.Delete(ctx, gitopsDepl)
			Expect(err).To(BeNil())

			event = eventlooptypes.EventLoopEvent{
				EventType: eventlooptypes.DeploymentModified,
				Request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: gitopsDepl.Namespace,
						Name:      gitopsDepl.Name,
					},
				},
				ReqResource: managedgitopsv1alpha1.GitOpsDeploymentTypeName,
				Client:      k8sClient,
				WorkspaceID: eventlooptypes.GetWorkspaceIDFromNamespaceID(*workspace),
			}

			fmt.Println("Sending event", event)
			channel <- event
			fmt.Println("Received response")
			response = <-fakeEventLoop.eventLoopInputChannel
			Expect(response.Request).Should(Equal(event.Request))
			Expect(response.EventType).Should(Equal(event.EventType))
			Expect(response.WorkspaceID).Should(Equal(event.WorkspaceID))
			fmt.Printf("response: %v\n", response)

		})
	})
})
