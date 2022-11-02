package utils

import (
	"context"
	"fmt"
	"time"

	argocdclient "github.com/argoproj/argo-cd/v2/pkg/apiclient"
	sessionpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/session"
	grpc_util "github.com/argoproj/argo-cd/v2/util/grpc"
	"github.com/argoproj/argo-cd/v2/util/io"
	"github.com/golang-jwt/jwt/v4"
)

// This file is loosely based on the 'argocd login' CLI command (https://github.com/argoproj/argo-cd/blob/0a46d37fc6af9fe0aa963bdd845e3d799aa0320d/cmd/argocd/commands/login.go#L60)

func generateDefaultClientForServerAddress(server string, optionalAuthToken string, skipTLSTest bool) (argocdclient.Client, error) {

	globalClientOpts := argocdclient.ClientOptions{
		ConfigPath:           "",
		ServerAddr:           server,
		Insecure:             true, // TODO: GITOPSRVCE-178: once support for TLS certification validation is implemented, the value should be used here.
		PlainText:            false,
		ClientCertFile:       "",
		ClientCertKeyFile:    "",
		GRPCWeb:              false,
		GRPCWebRootPath:      "",
		PortForward:          false,
		PortForwardNamespace: "",
		Headers:              []string{},
	}

	if globalClientOpts.PortForward {
		server = "port-forward"
	} else if globalClientOpts.Core {
		server = "kubernetes"
	} else if skipTLSTest {
		// skip test
	} else {
		dialTime := 30 * time.Second
		tlsTestResult, err := grpc_util.TestTLS(server, dialTime)
		if err != nil {
			return nil, err
		}
		if !tlsTestResult.TLS {
			if !globalClientOpts.PlainText {
				return nil, fmt.Errorf("server is not configured with TLS")
			}
		} else if tlsTestResult.InsecureErr != nil {
			if !globalClientOpts.Insecure {
				return nil, fmt.Errorf("WARNING: server certificate had error: %s", tlsTestResult.InsecureErr)
			}
		}
	}
	clientOpts := argocdclient.ClientOptions{
		ConfigPath:           "",
		ServerAddr:           server,
		AuthToken:            optionalAuthToken,
		Insecure:             globalClientOpts.Insecure,
		PlainText:            globalClientOpts.PlainText,
		ClientCertFile:       globalClientOpts.ClientCertFile,
		ClientCertKeyFile:    globalClientOpts.ClientCertKeyFile,
		GRPCWeb:              globalClientOpts.GRPCWeb,
		GRPCWebRootPath:      globalClientOpts.GRPCWebRootPath,
		PortForward:          globalClientOpts.PortForward,
		PortForwardNamespace: globalClientOpts.PortForwardNamespace,
		Headers:              globalClientOpts.Headers,
	}

	acdClient, err := argocdclient.NewClient(&clientOpts)

	return acdClient, err

}

func argoCDLoginCommand(username string, password string, acdClient argocdclient.Client) (string, error) {

	// Perform the login
	var tokenString string
	// var refreshToken string

	setConn, _, err := acdClient.NewSettingsClient()
	if err != nil {
		return "", err
	}
	defer io.Close(setConn)

	tokenString, err = passwordLogin(acdClient, username, password)
	if err != nil {
		return "", err
	}

	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	claims := jwt.MapClaims{}
	_, _, err = parser.ParseUnverified(tokenString, &claims)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func passwordLogin(acdClient argocdclient.Client, username, password string) (string, error) {
	sessConn, sessionIf, err := acdClient.NewSessionClient()
	if err != nil {
		return "", err
	}
	defer io.Close(sessConn)
	sessionRequest := sessionpkg.SessionCreateRequest{
		Username: username,
		Password: password,
	}
	createdSession, err := sessionIf.Create(context.Background(), &sessionRequest)
	if err != nil {
		return "", err
	}

	return createdSession.Token, nil
}
