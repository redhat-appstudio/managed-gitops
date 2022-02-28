package utils

import (
	"testing"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/session"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/utils/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestArgoCDLoginCommand(t *testing.T) {

	// Ensure we are able to login to get a token

	t.Parallel()

	mockSessionClient := &mocks.SessionServiceClient{}

	mockAppClient := &mocks.Client{}

	mockAppClient.On("NewSettingsClient").Return(mockCloser{}, nil, nil)
	mockAppClient.On("NewSessionClient").Return(mockCloser{}, mockSessionClient, nil)

	mockSessionClient.On("Create", mock.Anything, &session.SessionCreateRequest{
		Username: "admin",
		Password: "password",
		Token:    "",
	}).Return(&session.SessionResponse{Token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2NDUyMDM1NDksImp0aSI6ImM0Nzc2MDgwLTk3NDgtNGRhNS1iODU5LWMwM2QyM2Y3MGZhMiIsImlhdCI6MTY0NTExNzE0OSwiaXNzIjoiYXJnb2NkIiwibmJmIjoxNjQ1MTE3MTQ5LCJzdWIiOiJhZG1pbjpsb2dpbiJ9.7Di4Eb7xdBEF6SiScWbM5JdwE2Z_Kgr2hfzYA-KSLFs"}, nil)

	_, err := argoCDLoginCommand("admin", "password", mockAppClient)
	assert.NoError(t, err)

}
