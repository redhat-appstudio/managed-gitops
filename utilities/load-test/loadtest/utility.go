package loadtest

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

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

// Kubectl_apply is basically a go utility that performs kubetcl apply -n namespace -f  *yaml
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

// The PodRestartcount is used to return the count of the pods (Basically, RESTART count from kubectl get pods)
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

// The Get_pod_info tells us the memory usage of the pod along with container name
func Get_pod_info(kubeconfig string, master string, namespace string, podName string) map[string]interface{} {
	podResource := make(map[string]interface{})
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
		podResource[(podMetrics.ObjectMeta.Name + "\t" + podMetrics.Containers[0].Name)] = podMetrics.Containers[0].Usage["memory"].ToUnstructured()

	} else {
		podMetrics, err := mc.MetricsV1beta1().PodMetricses(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			panic(err)
		}
		for _, elements := range podMetrics.Items {
			podResource[(elements.ObjectMeta.Name + "\t" + elements.Containers[0].Name)] = elements.Containers[0].Usage["memory"].ToUnstructured()
		}
	}
	return podResource
}

// The PodInfoParse is used to properly parse over pod memory info and print in it pretty tabular format for better view
func PodInfoParse(podInfo map[string]interface{}) {
	w := new(tabwriter.Writer)

	// Format in tab-separated columns with a tab stop of 8.
	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	fmt.Fprintln(w, "Pod Name\tContainer Name\tMemory Usage (in Ki)")
	for key, value := range podInfo {
		int_value, _ := strconv.Atoi((strings.Split(fmt.Sprint(value), "Ki")[0]))
		fmt.Fprintln(w, key, "\t", int_value)
	}
	w.Flush()
}

// The PodMemoryDiff function is used to tell the memory of pod difference b/w before and after a process
func PodMemoryDiff(podInfoOld map[string]interface{}, podInfoNew map[string]interface{}) map[string]interface{} {
	PodMemory := make(map[string]interface{})
	for podInfoName := range podInfoNew {
		oldPodMemory, _ := strconv.Atoi((strings.Split(fmt.Sprint(podInfoOld[podInfoName]), "Ki")[0]))
		newPodMemory, _ := strconv.Atoi((strings.Split(fmt.Sprint(podInfoNew[podInfoName]), "Ki")[0]))
		PodMemory[podInfoName] = newPodMemory - oldPodMemory
	}
	return PodMemory
}
