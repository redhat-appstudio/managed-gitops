package utils

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	argocdclient "github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/go-logr/logr"
	"github.com/golang/protobuf/ptypes/empty"
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CredentialService is used to retrieve the login credentials for an Argo CD cluster. The primary mechanism to interact with this
// service is by calling the 'GetArgoCDLoginCredentials' method.
//
// Behind the scenes, the CredentialsService will:
// - locate the Argo CD admin secret, and login using that secret
// - create a new session based on that secret, storing the session key in an cache
// - returning credentials, and an Argo CD client instance, to the caller
//
// The credentials, and Argo CD client instance, can be used to invoke GRPC APIs on the Argo CD instance.
type CredentialService struct {
	input chan credentialRequest

	acdClientGenerator clientGenerator
	skipTLSTest        bool
}

// GetArgoCDLoginCredentials is used to retrieve the login credentials for an Argo CD cluster. This method is safe to call from multiple threads.
func (cs *CredentialService) GetArgoCDLoginCredentials(ctx context.Context, namespaceName string, namespaceUID string, skipCache bool, k8sClient client.Client) (argoCDCredentials, argocdclient.Client, error) {

	respChan := make(chan credentialResponse)

	req := credentialRequest{
		output:        respChan,
		k8sClient:     k8sClient,
		namespaceName: namespaceName,
		namespaceUID:  namespaceUID,
		skipCache:     skipCache,
		ctx:           ctx,
	}

	cs.input <- req

	var resp credentialResponse

	// Stop waiting if the context is cancelled
	select {
	case <-ctx.Done():
		return argoCDCredentials{}, nil, ctx.Err()
	case resp = <-respChan:
	}

	if resp.err != nil {
		return argoCDCredentials{}, nil, resp.err
	}

	return resp.creds, resp.argocdClient, nil
}

// Wrapper over 'generateDefaultClientForServerAddress' to implement clientGenerator interface
type defaultClientGenerator struct {
}

func (dcg *defaultClientGenerator) generateClientForServerAddress(server string, optionalAuthToken string, skipTLSTest bool) (argocdclient.Client, error) {
	return generateDefaultClientForServerAddress(server, optionalAuthToken, false)
}

// NewCredentialService is used to create a new instance of the Credential service.
//
// Parameters:
//   - acdClientGenerator: used to specify a custom interface, used to create connections to the Argo CD GRPC client
//     (optional: should usually be 'nil', unless a custom implementation is needed, for example, for mocking)
//   - skipTLSTest: whether to test the GRPC endpoint for TLS, before attempting to use it.
//     (should be true, unless running within automated tests, which do not simulate TLS)
func NewCredentialService(acdClientGenerator clientGenerator, skipTLSTest bool) *CredentialService {

	if acdClientGenerator == nil {
		acdClientGenerator = &defaultClientGenerator{}
	}

	res := CredentialService{
		input:              make(chan credentialRequest),
		acdClientGenerator: acdClientGenerator,
		skipTLSTest:        skipTLSTest,
	}

	go res.credentialHandler(res.input)

	return &res

}

type credentialRequest struct {
	k8sClient     client.Client
	namespaceName string
	namespaceUID  string
	ctx           context.Context

	// If true, the value in the cache will be ignored and replaced with a new value from login
	skipCache bool

	// server string
	output chan credentialResponse
}

type credentialResponse struct {
	creds        argoCDCredentials
	argocdClient argocdclient.Client
	err          error
}

func (cs *CredentialService) credentialHandler(input chan credentialRequest) {

	log := log.FromContext(context.Background())

	credentials := map[string]argoCDCredentials{}

	for {

		req := <-input

		credentialsKey := req.namespaceName + "-" + req.namespaceUID

		var resp credentialResponse

		// Return value from the cache, if it exists (as long as the cache is not invalidated)
		cacheValue, exists := credentials[credentialsKey]
		if exists && !req.skipCache {
			isPanic, err := sharedutil.CatchPanic(func() error {
				// Verify the cached login still works
				acdClient, err := cs.testLogin(req.ctx, cacheValue.ServerAddress, cacheValue.Passsword)
				if err != nil {
					log.Error(err, "cached login was invalid, so invalidating and reacquiring.")
					exists = false
				} else {
					resp = credentialResponse{
						creds:        cacheValue,
						argocdClient: acdClient,
					}
				}
				return nil
			})

			if isPanic {
				// On generic panic, invalidate the cache and return
				delete(credentials, credentialsKey)
				resp.err = err
				req.output <- resp
				continue
			}

			if err != nil {
				log.Error(err, "cached login was invalid, so invalidating and reacquiring.")
				exists = false
			}
		}

		if !exists || req.skipCache {

			_, err := sharedutil.CatchPanic(func() error {

				creds, acdClient, err := cs.getCredentialsFromNamespace(req, cs.skipTLSTest, log.WithValues("request namespace", req.namespaceName))
				if err != nil {
					resp = credentialResponse{
						err: err,
					}
					return nil
				}

				resp = credentialResponse{
					creds:        *creds,
					argocdClient: acdClient,
				}

				return nil
			})

			if err != nil {
				resp.err = err
			} else if resp.creds.Passsword != "" {
				// Store in cache on non-error
				credentials[credentialsKey] = resp.creds
			}
		}

		req.output <- resp

	}

}

func (cs *CredentialService) getCredentialsFromNamespace(req credentialRequest, skipTLSTest bool, log logr.Logger) (*argoCDCredentials, argocdclient.Client, error) {

	var err error
	var argoCDAdminPasswords []string

	argoCDAdminPasswords, err = getArgoCDAdminPasswords(req.ctx, req.namespaceName, req.k8sClient)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get Argo CD initial admin password: %v", err)
	}

	if len(argoCDAdminPasswords) == 0 {
		return nil, nil, fmt.Errorf("no Argo CD admin passwords found in " + req.namespaceName)
	}

	// Retrieve the Argo CD host name from the Route
	serverHostName := ""
	{

		routeList := &routev1.RouteList{}

		err := req.k8sClient.List(req.ctx, routeList, &client.ListOptions{Namespace: req.namespaceName})
		if err != nil {
			return nil, nil, fmt.Errorf("unable to list Routes in namespace %s : %v", req.namespaceName, err)
		}

		for _, route := range routeList.Items {

			// Skip known irrelevant routes
			if strings.HasPrefix(route.Name, "cluster") || strings.HasPrefix(route.Name, "kam") {
				continue
			}

			// Only use HTTPS string port
			if route.Spec.Port != nil && route.Spec.Port.TargetPort.Type == intstr.String &&
				route.Spec.Port.TargetPort.StrVal == "https" {

				serverHostName = route.Spec.Host
				break
			}

		}
	}
	if serverHostName == "" {
		return nil, nil, fmt.Errorf("Unable to locate Route in " + req.namespaceName)
	}

	acdClient, err := cs.acdClientGenerator.generateClientForServerAddress(serverHostName, "", skipTLSTest)
	if err != nil {
		return nil, nil, err
	}

	if acdClient == nil {
		return nil, nil, fmt.Errorf("argo CD client was nil")
	}

	// Attempt to login with every password we found, skipping failures
	for _, password := range argoCDAdminPasswords {

		userToken, err := argoCDLoginCommand("admin", password, acdClient)

		if err == nil && len(userToken) > 0 {
			acdClient, err := cs.acdClientGenerator.generateClientForServerAddress(serverHostName, userToken, skipTLSTest)
			if err != nil {
				return nil, nil, err
			}

			if acdClient == nil {
				return nil, nil, fmt.Errorf("argo CD client was nil")
			}

			return &argoCDCredentials{
				ServerAddress: serverHostName,
				Username:      "admin",
				Passsword:     userToken,
			}, acdClient, nil
		}

		if err != nil {
			log.Info("invalid login was skipped in " + req.namespaceName)
		}
	}

	return nil, nil, fmt.Errorf("unable to log in to Argo CD instance in " + req.namespaceName)

}

func getArgoCDAdminPasswords(ctx context.Context, namespace string, k8sClient client.Client) ([]string, error) {

	potentialPasswords := []string{}

	secretList := &corev1.SecretList{}

	if err := k8sClient.List(ctx, secretList, &client.ListOptions{Namespace: namespace}); err != nil {
		return potentialPasswords, err
	}

	for _, secret := range secretList.Items {

		if secret.Type != "Opaque" {
			continue
		}

		if !(strings.HasSuffix(secret.Name, "-cluster") || strings.HasSuffix(secret.Name, "-secret")) {
			continue
		}

		val, exists := secret.Data["admin.password"]
		if exists {
			potentialPasswords = append(potentialPasswords, string(val))
		}

		val, exists = secret.Data["password"]
		if exists {
			potentialPasswords = append(potentialPasswords, string(val))
		}
	}

	return potentialPasswords, nil

}

// Attempt to login using the login, to verify it is correct. This is useful for verifying cached logins.
func (cs *CredentialService) testLogin(ctx context.Context, serverAddr string, authToken string) (acdClient argocdclient.Client, err error) {

	acdClient, err = cs.acdClientGenerator.generateClientForServerAddress(serverAddr, authToken, false)
	if err != nil {
		return nil, fmt.Errorf("unable to create argocdclient: %v", err)
	}

	conn, verIf, err := acdClient.NewVersionClient()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve acd client: %v", err)
	}
	defer func() {
		closeErr := conn.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	_, err = verIf.Version(ctx, &empty.Empty{})
	if err != nil {
		return nil, fmt.Errorf("unable to invoke version api: %v", err)
	}

	return acdClient, nil

}

type argoCDCredentials struct {
	ServerAddress string
	Username      string
	Passsword     string
}

type clientGenerator interface {
	generateClientForServerAddress(server string, optionalAuthToken string, skipTLSTest bool) (argocdclient.Client, error)
}
