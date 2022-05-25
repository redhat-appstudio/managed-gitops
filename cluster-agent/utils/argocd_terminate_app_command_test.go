package utils

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/sync/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/utils/mocks"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Terminate Operation on Argo CD Application", func() {
	Context("Terminate Operation on Argo CD Application Test", func() {
		It("If the operation never terminates, after we ask it to terminate, then an error should be returned.", func() {
			argoCDNamespace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd",
					Namespace: "argocd",
					UID:       uuid.NewUUID(),
				},
			}

			application := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "argocd",
				},
				Status: appv1.ApplicationStatus{
					OperationState: &appv1.OperationState{
						Phase: common.OperationRunning,
					},
				},
			}

			k8sClient, err := generateFakeK8sClient(application)
			Expect(err).To(BeNil())

			mockAppServiceClient := &mocks.ApplicationServiceClient{}
			mockAppClient := &mocks.Client{}

			mockAppClient.On("NewApplicationClient").Return(mockCloser{}, mockAppServiceClient, nil)
			mockAppServiceClient.On("TerminateOperation", mock.Anything, &applicationpkg.OperationTerminateRequest{Name: &application.Name}).Return(nil, nil)

			startTime := time.Now()

			err = terminateOperation(context.Background(), application.Name, argoCDNamespace, mockAppClient, k8sClient,
				time.Duration(2*time.Second), log.FromContext(context.Background()))

			elapsedTime := time.Since(startTime)

			fmt.Fprintf(GinkgoWriter, "elapsed time: %v", elapsedTime)

			if elapsedTime < 2*time.Second {
				Fail("Not enough time passed in terminate operation")
				return
			}

			Expect(strings.Contains(err.Error(), "application operation never terminated: my-app")).To(BeTrue())
		})

		Context("Terminate Operation Goes To Done Test", func() {
			argoCDNamespace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd",
					Namespace: "argocd",
					UID:       uuid.NewUUID(),
				},
			}
			testCases := []struct {
				name       string
				finalPhase common.OperationPhase
			}{
				{
					name:       "error",
					finalPhase: common.OperationError,
				},
				{
					name:       "succeeded",
					finalPhase: common.OperationSucceeded,
				},
				{
					name:       "failed",
					finalPhase: common.OperationFailed,
				},
			}

			for _, testCase := range testCases {
				When(testCase.name, func() {
					It("Confirm that once an operation goes "+testCase.name+" that the terminate operation exits without error.", func() {
						application := &appv1.Application{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "my-app",
								Namespace: "argocd",
							},
							Status: appv1.ApplicationStatus{
								OperationState: &appv1.OperationState{
									Phase: common.OperationRunning,
								},
							},
						}

						k8sClient, err := generateFakeK8sClient(application)
						Expect(err).To(BeNil())

						By("Application exists and has a running operation then application doesn't exist confirm no error")
						mockAppServiceClient := &mocks.ApplicationServiceClient{}
						mockAppClient := &mocks.Client{}

						mockAppClient.On("NewApplicationClient").Return(mockCloser{}, mockAppServiceClient, nil)
						mockAppServiceClient.On("TerminateOperation", mock.Anything, &applicationpkg.OperationTerminateRequest{Name: &application.Name}).Return(nil, nil)

						go func() {
							time.Sleep(time.Second * 2)
							application.Status.OperationState.Phase = testCase.finalPhase

							err := k8sClient.Status().Update(context.Background(), application)
							Expect(err).To(BeNil())
						}()

						startTime := time.Now()

						err = terminateOperation(context.Background(), application.Name, argoCDNamespace, mockAppClient, k8sClient, time.Duration(5*time.Second), log.FromContext(context.Background()))
						Expect(err).To(BeNil())

						elapsedTime := time.Since(startTime)

						fmt.Fprintf(GinkgoWriter, "elapsed time: %v", elapsedTime)

						if elapsedTime < 2*time.Second {
							Fail("Not enough time passeed in terminate operation")
							return
						}
					})

				})
			}
		})

		It("Verify that the terminate operation exits immediately, if the application is deleted (no longer exists)", func() {
			argoCDNamespace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd",
					Namespace: "argocd",
					UID:       uuid.NewUUID(),
				},
			}

			application := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "argocd",
				},
				Status: appv1.ApplicationStatus{
					OperationState: &appv1.OperationState{
						Phase: common.OperationRunning,
					},
				},
			}

			By("Application exists and has a running operation then application doesn't exist confirm no error")
			k8sClient, err := generateFakeK8sClient(application)
			Expect(err).To(BeNil())

			mockAppServiceClient := &mocks.ApplicationServiceClient{}
			mockAppClient := &mocks.Client{}

			mockAppClient.On("NewApplicationClient").Return(mockCloser{}, mockAppServiceClient, nil)
			mockAppServiceClient.On("TerminateOperation", mock.Anything,
				&applicationpkg.OperationTerminateRequest{Name: &application.Name}).Return(nil, nil)

			go func() {
				time.Sleep(time.Second * 2)
				err := k8sClient.Delete(context.Background(), application)
				Expect(err).To(BeNil())
			}()

			startTime := time.Now()

			err = terminateOperation(context.Background(), application.Name, argoCDNamespace, mockAppClient, k8sClient,
				time.Duration(5*time.Second), log.FromContext(context.Background()))
			Expect(err).To(BeNil())

			elapsedTime := time.Since(startTime)

			fmt.Fprintf(GinkgoWriter, "elapsed time: %v", elapsedTime)

			if elapsedTime < 2*time.Second {
				Fail("Not enough time passeed in terminate operation")
				return
			}
		})
	})
})

func generateFakeK8sClient(initObjs ...client.Object) (client.WithWatch, error) {
	scheme := runtime.NewScheme()

	err := routev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = appv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjs...).Build()

	return k8sClient, nil

}

// Mock io.Closer with a nil
type mockCloser struct{}

var _ io.Closer = &mockCloser{}

func (mockCloser) Close() error { return nil }
