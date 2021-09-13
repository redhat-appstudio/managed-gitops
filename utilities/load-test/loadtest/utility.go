package loadtest

import (
	"os"
	"sync"

	appclientset "github.com/argoproj/argo-cd/v2/pkg/client/clientset/versioned"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	id string

	// call GetClientVars() to retrieve the Kubernetes client data for E2E test fixtures
	clientInitialized  sync.Once
	internalClientVars *E2EFixtureK8sClient
)

// GetE2EFixtureK8sClient initializes the Kubernetes clients (if needed), and returns the most recently initalized value.
// Note: this requires a local Kubernetes configuration (for example, while running the E2E tests).
func GetE2EFixtureK8sClient() *E2EFixtureK8sClient {

	// Initialize the Kubernetes clients only on first use
	clientInitialized.Do(func() {

		// set-up variables
		config, _ := getKubeConfig("", clientcmd.ConfigOverrides{})

		internalClientVars = &E2EFixtureK8sClient{
			AppClientset:     appclientset.NewForConfigOrDie(config),
			DynamicClientset: dynamic.NewForConfigOrDie(config),
			KubeClientset:    kubernetes.NewForConfigOrDie(config),
		}

	})
	return internalClientVars
}

// getKubeConfig creates new kubernetes client config using specified config path and config overrides variables
func getKubeConfig(configPath string, overrides clientcmd.ConfigOverrides) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = configPath
	clientConfig := clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, &overrides, os.Stdin)

	restConfig, err := clientConfig.ClientConfig()

	if err != nil {
		return nil, err
	}

	return restConfig, nil
}

// E2EFixtureK8sClient contains Kubernetes clients initialized from local k8s configuration
type E2EFixtureK8sClient struct {
	KubeClientset    kubernetes.Interface
	DynamicClientset dynamic.Interface
	AppClientset     appclientset.Interface
}

// func CheckError(err error) {
// 	if err != nil {
// 		debug.PrintStack()
// 	}
// }
