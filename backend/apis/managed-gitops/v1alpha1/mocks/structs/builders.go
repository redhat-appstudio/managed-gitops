package structs

import (
	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakekubeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

type mocks struct {
	FakeKubeClient client.Client
	MockCtrl       *gomock.Controller
}

// SetupDefaultMocks is an easy way to set up all the default mocks
func SetupDefaultMocks(t *testing.T, localObjects []runtime.Object) *mocks {
	mockKubeClient := fakekubeclient.NewFakeClient(localObjects...) // nolint: staticcheck
	mockCtrl := gomock.NewController(t)

	return &mocks{
		FakeKubeClient: mockKubeClient,
		MockCtrl:       mockCtrl,
	}
}
