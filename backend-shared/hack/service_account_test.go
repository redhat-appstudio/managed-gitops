package hack

import (
	"context"
	"flag"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func TestCreateServiceAccount(t *testing.T) {
	// TEST: Create Service Account
	t.Log("Test to create a service account on remote cluster!\n")

	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// use of local context
	t.Run("New SA", func(t *testing.T) {
		err := CreateServiceAccount(clientset, "argocd-manager", "kube-system")
		assert.NoError(t, err)
		rsa, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotNil(t, rsa)
	})

	t.Run("SA exists already", func(t *testing.T) {
		err := CreateServiceAccount(clientset, "argocd-manager", "kube-system")
		assert.NoError(t, err)
		rsa, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotNil(t, rsa)
	})

	t.Run("Invalid name", func(t *testing.T) {
		err := CreateServiceAccount(clientset, "", "kube-system")
		assert.NoError(t, err)
		rsa, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
		assert.Error(t, err)
		assert.Nil(t, rsa)
	})

	t.Run("Invalid namespace", func(t *testing.T) {
		err := CreateServiceAccount(clientset, "argocd-manager", "invalid")
		assert.NoError(t, err)
		rsa, err := clientset.CoreV1().ServiceAccounts("invalid").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotNil(t, rsa)
	})

	// wait.Poll will keep checking whether a (app variable).Status.Health.Status condition is met
	// err = wait.Poll(1*time.Second, 2*time.Minute, func() (bool, error) {

	// list all Argo CD applications in the namespace
	// appList, err := GetE2EFixtureK8sClient().AppClientset.ArgoprojV1alpha1().Applications(namespace).List(context.TODO(), v1.ListOptions{})
	// if err != nil {
	// 	t.Errorf("error, %v", err)
	// }
	// // for each application, check if (app variable).Status.Health.Status
	// for _, app := range appList.Items {

	// 	// if status is empty, return false
	// 	if app.Status.Health.Status == "" {
	// 		return false, nil
	// 	}
	// }
	// 	return true, nil
	// })

	// ns := &corev1.Namespace{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name: "kube-system",
	// 	},
	// }
	// sa := &corev1.ServiceAccount{
	// 	TypeMeta: metav1.TypeMeta{
	// 		APIVersion: "v1",
	// 		Kind:       "ServiceAccount",
	// 	},
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name:      "argocd-manager",
	// 		Namespace: "kube-system",
	// 	},
	// }
	// To create a fake clientset, use fake.NewSimpleClientset(ns)

	t.Log("\nService Account Creation Successful!\n")
}
