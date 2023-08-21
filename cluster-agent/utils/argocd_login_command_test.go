package utils

import (
	"fmt"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/session"
	"github.com/argoproj/argo-cd/v2/util/grpc"
	. "github.com/onsi/ginkgo/v2"
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
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Test generateDefaultClientForServerAddress", func() {
		var (
			server = "http://argocd-server"
			token  = "sample-token-ds32n"
		)
		It("should return a valid argocd client when skipTLS is enabled", func() {
			skipTLS := true
			client, err := generateDefaultClientForServerAddress(server, token, tlsConfig{skipTLSTest: skipTLS})
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())

			expectedClientOptions := apiclient.ClientOptions{
				ServerAddr:  server,
				AuthToken:   token,
				Insecure:    true,
				PlainText:   false,
				GRPCWeb:     false,
				PortForward: false,
			}
			Expect(client.ClientOptions()).To(Equal(expectedClientOptions))
		})

		It("should return a valid argocd client when skipTLS is disabled", func() {
			skipTLS := false
			mockTestTLS := func(addr string, dialtime time.Duration) (*grpc.TLSTestResult, error) {
				tlsResult := &grpc.TLSTestResult{
					TLS: true,
				}
				return tlsResult, nil
			}

			client, err := generateDefaultClientForServerAddress(server, token, tlsConfig{
				skipTLSTest: skipTLS,
				testTLS:     mockTestTLS,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())

			expectedClientOptions := apiclient.ClientOptions{
				ServerAddr:  server,
				AuthToken:   token,
				Insecure:    true,
				PlainText:   false,
				GRPCWeb:     false,
				PortForward: false,
			}
			Expect(client.ClientOptions()).To(Equal(expectedClientOptions))
		})

		It("should return an error when skipTLS is false and the TLS verification fails", func() {
			skipTLS := false
			expectedErr := fmt.Errorf("random TLS error")
			mockTestTLS := func(addr string, dialtime time.Duration) (*grpc.TLSTestResult, error) {
				return nil, expectedErr
			}

			client, err := generateDefaultClientForServerAddress(server, token, tlsConfig{
				skipTLSTest: skipTLS,
				testTLS:     mockTestTLS,
			})

			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(expectedErr))
			Expect(client).To(BeNil())
		})

		It("should return an error when skipTLS is false but the server doesn't support TLS", func() {
			skipTLS := false
			expectedErr := fmt.Errorf("server is not configured with TLS")
			mockTestTLS := func(addr string, dialtime time.Duration) (*grpc.TLSTestResult, error) {
				return &grpc.TLSTestResult{
					TLS: false,
				}, nil
			}

			client, err := generateDefaultClientForServerAddress(server, token, tlsConfig{
				skipTLSTest: skipTLS,
				testTLS:     mockTestTLS,
			})

			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(expectedErr))
			Expect(client).To(BeNil())
		})

		It("should not return an error when insecure is true and the server certificate had errors", func() {
			skipTLS := false
			mockTestTLS := func(addr string, dialtime time.Duration) (*grpc.TLSTestResult, error) {
				return &grpc.TLSTestResult{
					TLS:         true,
					InsecureErr: fmt.Errorf("error with certificate"),
				}, nil
			}

			client, err := generateDefaultClientForServerAddress(server, token, tlsConfig{
				skipTLSTest: skipTLS,
				testTLS:     mockTestTLS,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())

			expectedClientOptions := apiclient.ClientOptions{
				ServerAddr:  server,
				AuthToken:   token,
				Insecure:    true,
				PlainText:   false,
				GRPCWeb:     false,
				PortForward: false,
			}
			Expect(client.ClientOptions()).To(Equal(expectedClientOptions))
		})
	})
})
