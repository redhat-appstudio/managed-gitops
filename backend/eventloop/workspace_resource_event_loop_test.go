package eventloop

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/shared_resource_loop"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Test Workspace Resource Loop", func() {
	Context("Testing WorkspaceResourceLoop", func() {
		var (
			workspaceChan chan workspaceEventLoopMessage
			inputChan     chan workspaceResourceLoopMessage
			k8sClient     client.Client
			ns            *corev1.Namespace
		)
		BeforeEach(func() {
			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			ns = apiNamespace

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			sharedResourceLoop := shared_resource_loop.NewSharedResourceLoop()
			workspaceChan = make(chan workspaceEventLoopMessage)
			wel := newWorkspaceResourceLoopWithFactory(sharedResourceLoop, workspaceChan, MockSRLK8sClientFactory{fakeClient: k8sClient}, apiNamespace.Name, string(apiNamespace.UID))
			Expect(wel).ToNot(BeNil())

			inputChan = wel.inputChannel
		})

		It("should handle a valid message of type ManagedEnvironment", func() {
			By("create a resource loop message for GitOpsDeploymentManagedEnvironment")
			wrlm := workspaceResourceLoopMessage{
				apiNamespaceClient: k8sClient,
				messageType:        workspaceResourceLoopMessageType_processManagedEnvironment,
				payload: eventlooptypes.EventLoopMessage{
					MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event: &eventlooptypes.EventLoopEvent{
						ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
						Client:      k8sClient,
						EventType:   eventlooptypes.ManagedEnvironmentModified,
						Request: reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: ns.Name,
								Name:      "sample",
							},
						},
					},
				},
			}

			inputChan <- wrlm

			By("verify if the GitOpsDeploymentManagedEnvironment was managed by the resource loop")
			response := <-workspaceChan
			Expect(response).ToNot(BeNil())
			Expect(response.messageType).To(Equal(workspaceEventLoopMessageType_managedEnvProcessed_Event))
			Expect(response.payload).To(Equal(wrlm.payload))

		})

		It("should handle a valid message of type RepositoryCredentials", func() {
			dbQueries, err := db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())
			defer dbQueries.CloseDatabase()

			ctx := context.Background()

			By("create a sample GitOpsDeploymentRepositoryCredential CR")
			cr := &managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gitopsdeploymenrepositorycredential",
					Namespace: ns.Name,
					UID:       uuid.NewUUID(),
				},
				Spec: managedgitopsv1alpha1.GitOpsDeploymentRepositoryCredentialSpec{
					Repository: "https://fakegithub.com/test/test-repository",
					Secret:     "test-secret",
				}}

			err = k8sClient.Create(ctx, cr)
			Expect(err).ToNot(HaveOccurred())

			// Create new Secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: ns.Name,
				},
				Data: map[string][]byte{
					"username": []byte("test-username"),
					"password": []byte("test-password"),
				},
			}
			err = k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			By("send a message to the resource loop")
			wrlm := workspaceResourceLoopMessage{
				apiNamespaceClient: k8sClient,
				messageType:        workspaceResourceLoopMessageType_processRepositoryCredential,
				payload: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: cr.Namespace,
						Name:      cr.Name,
					},
				},
			}

			inputChan <- wrlm

			By("verify if the resource loop has handled the RepositoryCredential")
			apiCRToDBList := &[]db.APICRToDatabaseMapping{}
			Eventually(func() bool {
				err = dbQueries.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx, db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential, cr.Name, cr.Namespace, string(ns.UID), db.APICRToDatabaseMapping_DBRelationType_RepositoryCredential, apiCRToDBList)
				Expect(err).ToNot(HaveOccurred())
				for _, apiCRToDB := range *apiCRToDBList {
					if apiCRToDB.APIResourceName == cr.Name && apiCRToDB.APIResourceNamespace == cr.Namespace {
						return true
					}
				}
				return false
			}, "30s", "50ms").Should(BeTrue())

			for _, apiCRToDB := range *apiCRToDBList {
				if apiCRToDB.APIResourceName == cr.Name && apiCRToDB.APIResourceNamespace == cr.Namespace {
					Expect(apiCRToDB.DBRelationType).To(Equal(db.APICRToDatabaseMapping_DBRelationType_RepositoryCredential))
					Expect(apiCRToDB.APIResourceType).To(Equal(db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentRepositoryCredential))
					Expect(apiCRToDB.APIResourceUID).To(Equal(string(cr.UID)))
				}
			}

			err = k8sClient.Delete(ctx, cr)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Delete(ctx, secret)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Test internalProcessWorkspaceResourceMessage function", func() {
		var (
			ctx                context.Context
			workspaceChan      chan workspaceEventLoopMessage
			k8sClient          client.Client
			dbQueries          db.AllDatabaseQueries
			sharedResourceLoop *shared_resource_loop.SharedResourceEventLoop
			mockClientFactory  shared_resource_loop.SRLK8sClientFactory
			testLog            logr.Logger
			ns                 *corev1.Namespace
		)

		BeforeEach(func() {
			ctx = context.Background()
			testLog = log.FromContext(ctx)

			err := db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())
			ns = apiNamespace

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			sharedResourceLoop = shared_resource_loop.NewSharedResourceLoop()
			workspaceChan = make(chan workspaceEventLoopMessage)
			mockClientFactory = MockSRLK8sClientFactory{fakeClient: k8sClient}
		})

		AfterEach(func() {
			dbQueries.CloseDatabase()
		})

		It("should return an error with no retry if the k8sClient is nil", func() {
			msg := workspaceResourceLoopMessage{
				messageType: workspaceResourceLoopMessageType_processRepositoryCredential,
				payload: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "test-ns",
						Name:      "test",
					},
				},
			}
			shouldRetry, err := internalProcessWorkspaceResourceMessage(ctx, msg, sharedResourceLoop, workspaceChan, dbQueries, mockClientFactory, testLog)

			Expect(shouldRetry).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("invalid namespace client"))
		})

		It("should return an error with no retry for an invalid message", func() {
			msg := workspaceResourceLoopMessage{
				messageType:        workspaceResourceLoopMessageType("unknown-message-type"),
				apiNamespaceClient: k8sClient,
				payload: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "test-ns",
						Name:      "test",
					},
				},
			}
			shouldRetry, err := internalProcessWorkspaceResourceMessage(ctx, msg, sharedResourceLoop, workspaceChan, dbQueries, mockClientFactory, testLog)

			Expect(shouldRetry).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("SEVERE: unrecognized sharedResourceLoopMessageType: unknown-message-type"))
		})

		resourceLoopErr := func(msg string) error {
			return fmt.Errorf("failed to process workspace resource message: %v", msg)
		}

		Context("Test ManagedEnvironment messages", func() {
			var (
				msg workspaceResourceLoopMessage
			)

			BeforeEach(func() {
				msg = workspaceResourceLoopMessage{
					apiNamespaceClient: k8sClient,
					messageType:        workspaceResourceLoopMessageType_processManagedEnvironment,
					payload: eventlooptypes.EventLoopMessage{
						MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
						Event: &eventlooptypes.EventLoopEvent{
							ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
							Client:      k8sClient,
							EventType:   eventlooptypes.ManagedEnvironmentModified,
							Request: reconcile.Request{
								NamespacedName: types.NamespacedName{
									Namespace: ns.Name,
									Name:      "sample",
								},
							},
						},
					},
				}
			})

			It("should return an error with no retry if the ManagedEnvironment msg has an invalid payload", func() {
				msg.payload = "invalid-payload"

				shouldRetry, err := internalProcessWorkspaceResourceMessage(ctx, msg, sharedResourceLoop, workspaceChan, dbQueries, mockClientFactory, testLog)

				Expect(shouldRetry).To(BeFalse())
				Expect(err).To(HaveOccurred())
				expectedErr := "invalid ManagedEnvironment payload in processWorkspaceResourceMessage"
				Expect(err).To(Equal(resourceLoopErr(expectedErr)))
			})

			It("should not return an error with no retry if the ManagedEnvironment namespace is not found", func() {
				msg.payload = eventlooptypes.EventLoopMessage{
					MessageType: eventlooptypes.ApplicationEventLoopMessageType_Event,
					Event: &eventlooptypes.EventLoopEvent{
						ReqResource: eventlooptypes.GitOpsDeploymentManagedEnvironmentTypeName,
						Client:      k8sClient,
						Request: reconcile.Request{
							NamespacedName: types.NamespacedName{
								Namespace: "invalid-ns",
								Name:      "sample",
							},
						},
					},
				}

				shouldRetry, err := internalProcessWorkspaceResourceMessage(ctx, msg, sharedResourceLoop, workspaceChan, dbQueries, mockClientFactory, testLog)

				Expect(shouldRetry).To(BeFalse())
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return an error with retry if the shared resource loop fails to process ManagedEnvironment", func() {
				expiredCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-2*time.Hour))
				cancel()

				shouldRetry, err := internalProcessWorkspaceResourceMessage(expiredCtx, msg, sharedResourceLoop, workspaceChan, dbQueries, mockClientFactory, testLog)

				Expect(shouldRetry).To(BeTrue())
				Expect(err).To(HaveOccurred())

				Expect(err.Error()).To(SatisfyAny(
					Equal(resourceLoopErr("unable to reconcile shared managed env: context cancelled in GetOrCreateSharedManagedEnv").Error()),
					ContainSubstring("error on retrieving GetClusterUserByUsername: context deadline exceeded")))

			})
		})

		Context("Test RepositoryCredential messages", func() {
			var (
				msg workspaceResourceLoopMessage
			)

			BeforeEach(func() {
				msg = workspaceResourceLoopMessage{
					messageType:        workspaceResourceLoopMessageType_processRepositoryCredential,
					apiNamespaceClient: k8sClient,
					payload: reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: ns.Name,
							Name:      "test",
						},
					},
				}
			})

			It("should return an error with no retry if the RepositoryCredential msg has an invalid payload", func() {
				msg.payload = "invalid-payload"
				shouldRetry, err := internalProcessWorkspaceResourceMessage(ctx, msg, sharedResourceLoop, workspaceChan, dbQueries, mockClientFactory, testLog)

				Expect(shouldRetry).To(BeFalse())
				Expect(err).To(HaveOccurred())
				expectedErr := "invalid RepositoryCredential payload in processWorkspaceResourceMessage"
				Expect(err).To(Equal(resourceLoopErr(expectedErr)))
			})

			It("should not return an error with no retry if the RepositoryCredential namespace is not found", func() {
				msg.payload = reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "test-ns",
						Name:      "test",
					},
				}
				shouldRetry, err := internalProcessWorkspaceResourceMessage(ctx, msg, sharedResourceLoop, workspaceChan, dbQueries, mockClientFactory, testLog)

				Expect(shouldRetry).To(BeFalse())
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return an error with retry if the shared resource loop fails to process RepositoryCredential", func() {
				expiredCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-2*time.Hour))
				cancel()

				shouldRetry, err := internalProcessWorkspaceResourceMessage(expiredCtx, msg, sharedResourceLoop, workspaceChan, dbQueries, mockClientFactory, testLog)

				Expect(shouldRetry).To(BeTrue())
				Expect(err).To(HaveOccurred())

				Expect(err.Error()).To(SatisfyAny(
					Equal(resourceLoopErr("unable to reconcile repository credential. Error: context cancelled in ReconcileRepositoryCredential").Error()),
					ContainSubstring("error on retrieving GetClusterUserByUsername: context deadline exceeded")))

			})
		})
	})
})
