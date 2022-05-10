package utils

import (
	"context"
	"testing"

	argocdclient "github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/session"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/version"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/utils/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestGetArgoCDLoginCredentials(t *testing.T) {

	opts := zap.Options{
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Ensure we are able to login to get a token

	t.Parallel()

	var err error

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

		k8sClient, err = generateFakeK8sClient(namespace, loginSecret, route)
		assert.NoError(t, err)
	}

	mockVersionClient := &mocks.VersionServiceClient{}

	mockSessionClient := &mocks.SessionServiceClient{}

	mockAppClient := &mocks.Client{}

	mockAppClient.On("NewSettingsClient").Return(mockCloser{}, nil, nil)
	mockAppClient.On("NewSessionClient").Return(mockCloser{}, mockSessionClient, nil)

	// The code being tested only expects a version client that returns a non-error
	mockAppClient.On("NewVersionClient").Return(mockCloser{}, mockVersionClient, nil)
	mockVersionClient.On("Version", mock.Anything, mock.Anything).Return(&version.VersionMessage{Version: ""}, nil)

	mockSessionClient.On("Create", mock.Anything, &session.SessionCreateRequest{
		Username: "admin",
		Password: "a1Y4c0RvdkgxcHFPUTNJYWxSaDRubXlaZ3c3QUJGcmQ=",
		Token:    "",
	}).Return(&session.SessionResponse{Token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2NDUyMDM1NDksImp0aSI6ImM0Nzc2MDgwLTk3NDgtNGRhNS1iODU5LWMwM2QyM2Y3MGZhMiIsImlhdCI6MTY0NTExNzE0OSwiaXNzIjoiYXJnb2NkIiwibmJmIjoxNjQ1MTE3MTQ5LCJzdWIiOiJhZG1pbjpsb2dpbiJ9.7Di4Eb7xdBEF6SiScWbM5JdwE2Z_Kgr2hfzYA-KSLFs"}, nil)

	clientGenerator := mockClientGenerator{
		mockClient: mockAppClient,
	}

	cs := NewCredentialService(&clientGenerator, true)
	assert.NoError(t, err)

	creds, argoClient, err := cs.GetArgoCDLoginCredentials(context.Background(), "openshift-gitops", "12", true, k8sClient)
	assert.NoError(t, err)
	assert.NotEmpty(t, creds)
	assert.NotEmpty(t, argoClient)

	// A second call should work as well
	creds, argoClient, err = cs.GetArgoCDLoginCredentials(context.Background(), "openshift-gitops", "12", true, k8sClient)
	assert.NoError(t, err)
	assert.NotEmpty(t, creds)
	assert.NotEmpty(t, argoClient)

	// Acquire from the cache
	creds, argoClient, err = cs.GetArgoCDLoginCredentials(context.Background(), "openshift-gitops", "12", false, k8sClient)
	assert.NoError(t, err)
	assert.NotEmpty(t, creds)
	assert.NotEmpty(t, argoClient)

}

type mockClientGenerator struct {
	mockClient argocdclient.Client
}

func (mcg *mockClientGenerator) generateClientForServerAddress(server string, optionalAuthToken string, skipTLSTest bool) (argocdclient.Client, error) {
	return mcg.mockClient, nil
}
