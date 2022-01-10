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

	// use of local context
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

	t.Run("New SA", func(t *testing.T) {
		err := CreateServiceAccount(clientset, "argocd-manager", "kube-system")
		assert.NoError(t, err)
		rsa, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotNil(t, rsa)
		errDelSA := clientset.CoreV1().ServiceAccounts("kube-system").Delete(context.Background(), "argocd-manager", metav1.DeleteOptions{})
		assert.NoError(t, errDelSA)
	})

	t.Run("SA exists already", func(t *testing.T) {
		// create first service account
		err := CreateServiceAccount(clientset, "argocd-manager", "kube-system")
		assert.NoError(t, err)
		// try to create another service account within same namespace and similar defination, should give an error
		errDup := CreateServiceAccount(clientset, "argocd-manager", "kube-system")
		assert.Error(t, errDup)
		rsa, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotNil(t, rsa)
		errDelSA := clientset.CoreV1().ServiceAccounts("kube-system").Delete(context.Background(), "argocd-manager", metav1.DeleteOptions{})
		assert.NoError(t, errDelSA)
	})

	t.Run("Invalid name", func(t *testing.T) {
		err := CreateServiceAccount(clientset, "", "kube-system")
		// if error in service account name exists, the test should pass
		assert.Error(t, err)
		// to cross verify if "argocd-manager" still exists then GET
		rsa, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
		assert.Error(t, err)
		assert.Nil(t, rsa.Secrets)
	})

	t.Run("Invalid namespace", func(t *testing.T) {
		err := CreateServiceAccount(clientset, "argocd-manager", "invalid")
		// if error in namespace exists, the test should pass
		assert.Error(t, err)
		// to cross verify if "argocd-manager" still exists in an "invalid" namespace then GET, which should fail
		rsa, err := clientset.CoreV1().ServiceAccounts("invalid").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
		assert.Error(t, err)
		assert.Nil(t, rsa.Secrets)
	})

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
