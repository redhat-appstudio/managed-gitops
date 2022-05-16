package utils

import (
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/session"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/utils/mocks"
	"github.com/stretchr/testify/mock"
)

// These tests are expected to run in parallel with other test cases, We can run the ginkgo tests parallelly by passing "ginkgo -p" command in CLI.
var _ = Describe("ArgoCD Login Command", func() {
	Context("ArgoCD Login Command Test", func() {
		It("Ensure we are able to login to get a token”", func() {

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
			Expect(err).To(BeNil())
		})
	})
})
