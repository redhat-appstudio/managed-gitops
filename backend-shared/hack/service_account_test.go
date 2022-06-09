package hack

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("Service Account Tests", func() {
	Context("Setup service account test", func() {
		// TEST: Create Service Account
		GinkgoWriter.Printf("Test to create a service account on remote cluster!\n")

		config, err := ctrl.GetConfig()
		if err != nil {
			GinkgoWriter.Printf("Skipping service account creation tests, because a K8s config could not be found.")
			return
		}

		// create the clientset
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			panic(err.Error())
		}

		_, err = clientset.CoreV1().Namespaces().Get(context.TODO(), "kube-system", metav1.GetOptions{})
		if err != nil {

			if strings.Contains(err.Error(), "connect: no route to host") || strings.Contains(err.Error(), "no such host") {
				GinkgoWriter.Printf("Skipping K8s test because there is no accessible K8s cluster")
				return
			}
		}
		When("New SA", func() {
			It("Should create a new SA", func() {
				err := CreateServiceAccount(clientset, "argocd-manager", "kube-system")
				Expect(err).To(BeNil())
				rsa, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
				Expect(err).To(BeNil())
				Expect(rsa).ToNot(BeNil())
				errDelSA := clientset.CoreV1().ServiceAccounts("kube-system").Delete(context.Background(), "argocd-manager", metav1.DeleteOptions{})
				Expect(errDelSA).To(BeNil())
			})
		})
		When("Invalid name", func() {
			It("Should pass if there is an error in service name.", func() {
				err := CreateServiceAccount(clientset, "", "kube-system")
				// if error in service account name exists, the test should pass
				Expect(err).ToNot(BeNil())
				// to cross verify if "argocd-manager" still exists then GET
				rsa, err := clientset.CoreV1().ServiceAccounts("kube-system").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
				Expect(err).ToNot(BeNil())
				Expect(rsa.Secrets).To(BeNil())
			})
		})
		When("Invalid namespace", func() {
			It("Should pass if there is an error in namespace exists.", func() {
				err := CreateServiceAccount(clientset, "argocd-manager", "invalid")
				// if error in namespace exists, the test should pass
				Expect(err).ToNot(BeNil())
				// to cross verify if "argocd-manager" still exists in an "invalid" namespace then GET, which should fail
				rsa, err := clientset.CoreV1().ServiceAccounts("invalid").Get(context.Background(), "argocd-manager", metav1.GetOptions{})
				Expect(err).ToNot(BeNil())
				Expect(rsa.Secrets).To(BeNil())
			})
		})
		GinkgoWriter.Printf("\nService Account Creation Successful!\n")
		When("Test Bearer Token", func() {
			It("Should pass.", func() {
				namespaces := []string{"argocd"}
				token, err := InstallServiceAccount(clientset, "kube-system", namespaces)
				Expect(err).To(BeNil())
				Expect(token).ToNot(BeNil())
				clientObj, err := generateClientFromClusterServiceAccount(ctrl.GetConfigOrDie(), token)
				Expect(err).To(BeNil())
				Expect(clientObj).ToNot(BeNil())
				errDelSA := clientset.CoreV1().ServiceAccounts("kube-system").Delete(context.Background(), "argocd-manager", metav1.DeleteOptions{})
				Expect(errDelSA).To(BeNil())
			})
		})
	})
})
