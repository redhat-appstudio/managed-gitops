package loadtest

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"

	appclientset "github.com/argoproj/argo-cd/v2/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
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

func Kubectl_apply(namespace string, URL string) {
	prg := "kubectl apply -n %s -f %s"
	// namespace := "argocd"
	// URL := "https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml"
	s := fmt.Sprintf(prg, namespace, URL)

	// To print which command is running
	fmt.Println("Running: ", s)

	// To get the output of the command
	out, err := exec.Command("kubectl", "apply", "-n", namespace, "-f", URL).Output()
	if err != nil {
		log.Fatal(err)
	}

	// To actually run the command (runs in background)
	cmd_run := exec.Command("kubectl", "apply", "-n", namespace, "-f", URL)
	err_run := cmd_run.Run()

	if err_run != nil {
		log.Fatal(err_run)
	}
	fmt.Println(string(out), "Command Run Successful!")
}

func PodRestartcount(namespace string) map[string]string {
	var allrestartInfo = make(map[string]string)

	out, err := exec.Command("kubectl", "get", "pods", "-n", namespace).Output()
	if err != nil {
		log.Fatal(err)
	}

	res := string(out)

	for index, i := range strings.Split(res, "\n") {
		if index != 0 && index < len(strings.Split(res, "\n"))-1 {
			allrestartInfo[strings.Fields(i)[0]] = strings.Fields(i)[3]
		}
	}

	return allrestartInfo
}

func Get_pod_info(kubeconfig string, master string, namespace string, podName string) {
	config, err := clientcmd.BuildConfigFromFlags(master, kubeconfig)
	if err != nil {
		panic(err)
	}

	mc, err := metrics.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	if podName != "" {
		// To get memory info of the specific pod passed as an argument
		podMetrics, err := mc.MetricsV1beta1().PodMetricses(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			panic(err)
		}
		fmt.Println(podMetrics.ObjectMeta.Name, podMetrics.Containers[0].Name, podMetrics.Containers[0].Usage["memory"].ToUnstructured())
	} else {
		podMetrics, err := mc.MetricsV1beta1().PodMetricses(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			panic(err)
		}
		for _, elements := range podMetrics.Items {
			// to just get pod name without hash appended replace elements.ObjectMeta.Name with elements.Containers[0].Name
			fmt.Println(elements.ObjectMeta.Name, elements.Containers[0].Name, elements.Containers[0].Usage["memory"].ToUnstructured())
		}
	}

}

// func CheckError(err error) {
// 	if err != nil {
// 		debug.PrintStack()
// 	}
// }
