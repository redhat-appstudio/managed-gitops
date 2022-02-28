package utils

import (
	"context"
	"io"
	"testing"
	"time"

	applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/sync/common"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/utils/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestTerminateOperation(t *testing.T) {

	// If the operation never terminates, after we ask it to terminate, then an error should be returned.

	t.Parallel()

	var err error

	argoCDNamespace := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: "argocd", UID: uuid.NewUUID()}}

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
	if !assert.NoError(t, err) {
		return
	}

	mockAppServiceClient := &mocks.ApplicationServiceClient{}

	mockAppClient := &mocks.Client{}
	mockAppClient.On("NewApplicationClient").Return(mockCloser{}, mockAppServiceClient, nil)

	mockAppServiceClient.On("TerminateOperation", mock.Anything, &applicationpkg.OperationTerminateRequest{Name: &application.Name}).Return(nil, nil)

	startTime := time.Now()

	err = terminateOperation(context.Background(), application.Name, argoCDNamespace, mockAppClient, k8sClient,
		time.Duration(2*time.Second), log.FromContext(context.Background()))

	elapsedTime := time.Since(startTime)

	t.Logf("elapsed time: %v", elapsedTime)

	if elapsedTime < 2*time.Second {
		t.Error("Not enough time passeed in terminate operation")
		return
	}

	assert.EqualError(t, err, "application operation never terminated: my-app")

}

func TestTerminateOperation_OperationGoesToDone(t *testing.T) {

	argoCDNamespace := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: "argocd", UID: uuid.NewUUID()}}

	// Confirm that once an operation goes to error/succeeded/failed, that the
	// terminate operation exits without error.

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

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var err error

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
			if err != nil {
				t.Error(err)
				return
			}

			// application exists and has a running operation
			// then application doesn't exist
			// confirm no error

			mockAppServiceClient := &mocks.ApplicationServiceClient{}

			mockAppClient := &mocks.Client{}
			mockAppClient.On("NewApplicationClient").Return(mockCloser{}, mockAppServiceClient, nil)

			mockAppServiceClient.On("TerminateOperation", mock.Anything, &applicationpkg.OperationTerminateRequest{Name: &application.Name}).Return(nil, nil)

			go func() {

				time.Sleep(time.Second * 2)
				application.Status.OperationState.Phase = testCase.finalPhase

				if err := k8sClient.Status().Update(context.Background(), application); err != nil {
					t.Error(err)
					return
				}

			}()

			startTime := time.Now()

			err = terminateOperation(context.Background(), application.Name, argoCDNamespace, mockAppClient, k8sClient, time.Duration(5*time.Second), log.FromContext(context.Background()))
			if err != nil {
				t.Error(err)
				return
			}

			elapsedTime := time.Since(startTime)

			t.Logf("elapsed time: %v", elapsedTime)

			if elapsedTime < 2*time.Second {
				t.Error("Not enough time passeed in terminate operation")
				return
			}
		})
	}

}

func TestTerminateOperation_ExitIfAppDoesntExist(t *testing.T) {

	argoCDNamespace := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: "argocd", UID: uuid.NewUUID()}}

	// Verify that the terminate operation exits immediately, if the application is deleted (no longer exists)

	var err error

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

	// application exists and has a running operation
	// then application doesn't exist
	// confirm no error

	k8sClient, err := generateFakeK8sClient(application)
	if !assert.NoError(t, err) {
		return
	}

	mockAppServiceClient := &mocks.ApplicationServiceClient{}

	mockAppClient := &mocks.Client{}
	mockAppClient.On("NewApplicationClient").Return(mockCloser{}, mockAppServiceClient, nil)

	mockAppServiceClient.On("TerminateOperation", mock.Anything,
		&applicationpkg.OperationTerminateRequest{Name: &application.Name}).Return(nil, nil)

	go func() {

		time.Sleep(time.Second * 2)
		err := k8sClient.Delete(context.Background(), application)

		assert.NoError(t, err)

	}()

	startTime := time.Now()

	err = terminateOperation(context.Background(), application.Name, argoCDNamespace, mockAppClient, k8sClient,
		time.Duration(5*time.Second), log.FromContext(context.Background()))
	if err != nil {
		t.Error(err)
		return
	}

	elapsedTime := time.Since(startTime)

	t.Logf("elapsed time: %v", elapsedTime)

	if elapsedTime < 2*time.Second {
		t.Error("Not enough time passeed in terminate operation")
		return
	}

}

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
