package eventloop

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// This test scenario requires, tests to run in order, hence using 'Ordered' decorator.
var _ = Describe("Workspace Event Loop Test", Ordered, func() {

	var err error
	var scheme *runtime.Scheme
	var apiNamespace *v1.Namespace
	var argocdNamespace *v1.Namespace
	var kubesystemNamespace *v1.Namespace
	var tAELF *testApplicationEventLoopFactory
	var msgTemp eventlooptypes.EventLoopMessage
	var inputChannel chan eventlooptypes.EventLoopMessage

	Context("Workspace event loop responds to channel events", func() {

		// Start the workspace event loop using single ApplicationEventLoopFactory object,
		// this way all tests can keep track of number of event loops created by other tests.
		BeforeAll(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err = eventlooptypes.GenericTestSetup()

			Expect(err).To(BeNil())

			tAELF = &testApplicationEventLoopFactory{
				outputChannel: make(chan eventlooptypes.EventLoopMessage),
			}

			// Start the workspace event loop with our custom test factory, so that we can capture output
			inputChannel = make(chan eventlooptypes.EventLoopMessage)
			internalStartWorkspaceEventLoopRouter(inputChannel, string(apiNamespace.UID), tAELF)
		})

		It("Should pass events received on input channel to application event loop", func() {

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: apiNamespace.Name,
					UID:       "A",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			k8sClientOuter := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

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
			Expect(tAELF.numberOfEventLoopsCreated).To(Equal(1))
		})

		It("Should not create new application event loop when 2nd event is passed with same ID", func() {
			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: apiNamespace.Name,
					UID:       "A",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			k8sClientOuter := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

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
			Expect(tAELF.numberOfEventLoopsCreated).To(Equal(1))

		})

		It("Should create new application event loop when 3rd event is passed with diferent ID", func() {
			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: apiNamespace.Name,
					UID:       "B",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			k8sClientOuter := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

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
			Expect(tAELF.numberOfEventLoopsCreated).To(Equal(2))

		})

		It("Should not pass an orphaned event to application event loop.", func() {

			gitopsDeplSyncRun := &managedgitopsv1alpha1.GitOpsDeploymentSyncRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl-sync",
					Namespace: apiNamespace.Name,
					UID:       "C",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSyncRunSpec{
					GitopsDeploymentName: "dummy-deployment",
					RevisionID:           "HEAD",
				},
			}

			k8sClientOuter := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDeplSyncRun, apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			// Simulate a GitOpsDeploymentSyncRun event
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.SyncRunModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Name,
							Name:      "my-gitops-depl-sync",
						},
					},
					Client:                  k8sClientOuter,
					ReqResource:             managedgitopsv1alpha1.GitOpsDeploymentSyncRunTypeName,
					AssociatedGitopsDeplUID: "orphaned",
					WorkspaceID:             string(apiNamespace.UID),
				},
			}

			// To share it with the next test
			msgTemp = msg

			// The number of event loops created before we started the test
			originalNumberOfEventLoopsCreated := tAELF.numberOfEventLoopsCreated

			// Create goroutine and pass event,
			//because loop in workspaceEventLoopRouter will keep on waiting for new event to be received and test will get stuck here.
			go func() {

				inputChannel <- msg
				// We don't read from the channel here, because this will cause us to
				// miss the event when we tried to read from it elsewhere.
			}()

			// Consider test case passed if a new application event loop is not created in 5 seconds.
			Consistently(tAELF.numberOfEventLoopsCreated, "5s").Should(Equal(originalNumberOfEventLoopsCreated),
				"the number of event loops shoulnd't change")
		})

		It("Should unorphan previous GitOpsDeploymentSyncRun event if parent GitOpsDeployment event is passed and new application event loop should be created.", func() {

			By("creating a new GitOpsDeployment resource, that the GitOpsDeploymentSyncRun was missing from the previous step.")

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-deployment",
					Namespace: apiNamespace.Name,
					UID:       "D",
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentSpec{
					Source: managedgitopsv1alpha1.ApplicationSource{},
					Type:   managedgitopsv1alpha1.GitOpsDeploymentSpecType_Automated,
				},
			}

			k8sClientOuter := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gitopsDepl, apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			// Simulate a GitOpsDeployment modified event
			msg := eventlooptypes.EventLoopMessage{
				MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
				Event: &eventlooptypes.EventLoopEvent{
					EventType: eventlooptypes.DeploymentModified,
					Request: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: apiNamespace.Namespace,
							Name:      gitopsDepl.Name,
						},
					},
					Client:                  k8sClientOuter,
					ReqResource:             managedgitopsv1alpha1.GitOpsDeploymentTypeName,
					AssociatedGitopsDeplUID: string(gitopsDepl.UID),
					WorkspaceID:             string(apiNamespace.UID),
				},
			}

			// Create goroutine and pass event,
			//because loop in workspaceEventLoopRouter will keep on waiting for new event to be received and test will get stuck here.
			go func() {
				inputChannel <- msg
			}()

			Eventually(func() bool {
				// We use a function here to check if number is 3
				return tAELF.numberOfEventLoopsCreated == 3
			}).Should(BeTrue())

			By("Read the events from the output channel, make sure they are the ones we expect, now that the gitopsdeploymentsyncrun in unorphaned")

			deploymentModifiedMsg := <-tAELF.outputChannel

			// Make sure this is the gitopsdeployment event from above
			Expect(deploymentModifiedMsg).NotTo(BeNil())
			Expect(deploymentModifiedMsg.Event).To(Equal(msg.Event))
			Expect(deploymentModifiedMsg.MessageType).To(Equal(msg.MessageType))

			syncRunModifiedMsg := <-tAELF.outputChannel

			// Make sure this is the gitopsdeploymentsyncrun event from above
			Expect(syncRunModifiedMsg).NotTo(BeNil())
			Expect(syncRunModifiedMsg.Event.Request.Name).To(Equal(msgTemp.Event.Request.Name))
			Expect(syncRunModifiedMsg.MessageType).To(Equal(msgTemp.MessageType))
		})

	})
})

// testApplicationEventLoopFactory is a mock applicationEventQueueLoopFactory, used for unit tests above.
type testApplicationEventLoopFactory struct {
	outputChannel             chan eventlooptypes.EventLoopMessage
	numberOfEventLoopsCreated int
}

var _ applicationEventQueueLoopFactory = &testApplicationEventLoopFactory{}

// Instead of starting a new application event queue loop (like the default implementation of applictionEventQueueLoopFactory)
// we instead just return a previously provided channel.
func (ta *testApplicationEventLoopFactory) startApplicationEventQueueLoop(gitopsDeplID string, workspaceID string,
	sharedResourceEventLoop *shared_resource_loop.SharedResourceEventLoop) chan eventlooptypes.EventLoopMessage {

	// Increase count by 1 if new application event loop is created
	ta.numberOfEventLoopsCreated++
	return ta.outputChannel
}
