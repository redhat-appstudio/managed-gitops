package utils

import (
	"context"

	applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/session"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/argoproj/gitops-engine/pkg/sync/common"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/utils/mocks"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ArgoCD AppSync Command", func() {
	Context("ArgoCD AppSync Command Test", func() {
		It("Synchronize application on the given Argo CD appliatication, in the given namespace.", func() {
			var err error

			nowTime := metav1.Now()

			app := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "openshift-gitops",
				},
				Status: appv1.ApplicationStatus{
					ReconciledAt: &nowTime,
					Health:       appv1.HealthStatus{Status: health.HealthStatusHealthy},
					Sync: appv1.SyncStatus{
						Status: appv1.SyncStatusCodeSynced,
					},
					OperationState: &appv1.OperationState{
						Phase:      common.OperationSucceeded,
						FinishedAt: &nowTime,
					},
				},
			}

			var k8sClient client.Client

			{

				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "openshift-gitops",
						Namespace: "openshift-gitops",
					},
				}

				loginSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "openshift-gitops-cluster",
						Namespace: "openshift-gitops",
					},
					Data: map[string][]byte{
						"admin.password": []byte("a1Y4c0RvdkgxcHFPUTNJYWxSaDRubXlaZ3c3QUJGcmQ="),
					},
				}

				route := &routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "openshift-gitops-server",
						Namespace: "openshift-gitops",
					},
					Spec: routev1.RouteSpec{
						Port: &routev1.RoutePort{
							TargetPort: intstr.FromString("https"),
						},
						Host: "route-host",
					},
					Status: routev1.RouteStatus{},
				}

				k8sClient, err = generateFakeK8sClient(namespace, loginSecret, route, app)
				Expect(err).To(BeNil())
			}

			mockAppClient := &mocks.Client{}
			mockSessionClient := &mocks.SessionServiceClient{}

			By(" 1) The sync logic will login and create a session token")
			mockAppClient.On("NewSettingsClient").Return(mockCloser{}, nil, nil)
			mockAppClient.On("NewSessionClient").Return(mockCloser{}, mockSessionClient, nil)
			mockSessionClient.On("Create", mock.Anything, &session.SessionCreateRequest{
				Username: "admin",
				Password: "a1Y4c0RvdkgxcHFPUTNJYWxSaDRubXlaZ3c3QUJGcmQ=",
				Token:    "",
			}).Return(&session.SessionResponse{Token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2NDUyMDM1NDksImp0aSI6ImM0Nzc2MDgwLTk3NDgtNGRhNS1iODU5LWMwM2QyM2Y3MGZhMiIsImlhdCI6MTY0NTExNzE0OSwiaXNzIjoiYXJnb2NkIiwibmJmIjoxNjQ1MTE3MTQ5LCJzdWIiOiJhZG1pbjpsb2dpbiJ9.7Di4Eb7xdBEF6SiScWbM5JdwE2Z_Kgr2hfzYA-KSLFs"}, nil)

			By("2) The Sync function of AppServiceClient will be called with the app name, revision, and other parameters")

			mockAppServiceClient := &mocks.ApplicationServiceClient{}
			mockAppClient.On("NewApplicationClient").Return(mockCloser{}, mockAppServiceClient, nil)
			appName := "my-app"
			mockAppServiceClient.On("Sync", mock.Anything, mock.MatchedBy(func(asr *applicationpkg.ApplicationSyncRequest) bool {
				return *asr.Name == appName && *asr.Revision == "master" && !*asr.Prune
			})).Return(nil, nil)

			By(" 3) After Sync, a Get occurs for the app, then a watch is setup to wait for the sync operation to finish. We provide the post-sync version of the app, to both")

			mockAppServiceClient.On("Get", mock.Anything, &applicationpkg.ApplicationQuery{Name: &appName}).Return(app, nil)
			awe := make(chan *appv1.ApplicationWatchEvent)
			go func() {
				By("Simulate the Application status updating to its final version (operation is complete)")

				awe <- &appv1.ApplicationWatchEvent{
					Application: *app,
				}
				By("After the above event is sent, close the channel")

				close(awe)
			}()
			mockAppClient.On("WatchApplicationWithRetry", mock.Anything, app.Name, mock.Anything).Return(awe)

			clientGenerator := mockClientGenerator{
				mockClient: mockAppClient,
			}

			cs := NewCredentialService(&clientGenerator, true)
			err = AppSync(context.Background(), appName, "master", "openshift-gitops", k8sClient, cs, true)
			Expect(err).To(BeNil())
		})
	})
})
