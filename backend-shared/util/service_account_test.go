package util

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Service Account Tests", func() {

	Context("Setup service account test", func() {

		ctx := context.Background()
		log := log.FromContext(ctx)

		kubeSystemNamespace := "kube-system"
		var k8sClient client.Client

		BeforeEach(func() {
			// TEST: Create Service Account
			GinkgoWriter.Println("creating a service account on remote cluster!")

			config, err := ctrl.GetConfig()
			if err != nil {
				Skip("Skipping service account creation tests, because a K8s config could not be found.")
				return
			}
			Expect(err).To(BeNil())

			k8sClient, err = client.New(config, client.Options{Scheme: scheme.Scheme})
			if err != nil {
				if strings.Contains(err.Error(), "no such host") {
					Skip("Skipping K8s test because there is no accessible K8s cluster")
					return
				}
			}
			Expect(err).To(BeNil())

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: kubeSystemNamespace,
				},
			}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(namespace), namespace); err != nil {
				if strings.Contains(err.Error(), "connect: no route to host") || strings.Contains(err.Error(), "no such host") {
					Skip("Skipping K8s test because there is no accessible K8s cluster")
					return
				}
				fmt.Println("error occurred on client construction", err)
				Expect(err).To(BeNil())
			}

			list := corev1.ServiceAccountList{}
			err = k8sClient.List(ctx, &list, &client.ListOptions{Namespace: "kube-system"})
			Expect(err).To(BeNil())

			for idx := range list.Items {
				sa := list.Items[idx]
				// Skip any service accounts that DON'T contain argocd
				if !strings.Contains(sa.Name, "argocd") {
					continue
				}

				err = k8sClient.Delete(ctx, &sa)
				Expect(err).To(BeNil())
			}

		})

		When("New SA", func() {

			It("Should create a new Service", func() {

				serviceAccountName := "argocd-manager"
				serviceAccountNamespace := "kube-system"

				sa, err := getOrCreateServiceAccount(ctx, k8sClient, serviceAccountName, serviceAccountNamespace, log)
				Expect(err).To(BeNil())
				Expect(sa).ToNot(BeNil())
				Expect(sa.Name).ToNot(BeEmpty())

				serviceAccount := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceAccountName,
						Namespace: serviceAccountNamespace,
					},
				}
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount)
				Expect(err).To(BeNil())
				Expect(serviceAccount).ToNot(BeNil())

				err = k8sClient.Delete(ctx, serviceAccount)
				Expect(err).To(BeNil())
			})
		})

		When("Invalid name", func() {
			It("Should return an error if the service account name is empty", func() {
				_, err := getOrCreateServiceAccount(ctx, k8sClient, "", "kube-system", log)
				Expect(err).ToNot(BeNil())
			})
		})

		When("Invalid namespace", func() {
			It("Should return an error if the service account namespace is invalid", func() {
				_, err := getOrCreateServiceAccount(ctx, k8sClient, "argocd-manager", "invalid", log)
				Expect(err).ToNot(BeNil())
			})
		})

		When("Test Bearer Token", func() {

			It("Should pass.", func() {
				// namespaces := []string{}
				uuid := "my-uuid"
				token, sa, err := InstallServiceAccount(ctx, k8sClient, uuid, "kube-system" /*namespaces,*/, log)
				Expect(err).To(BeNil())
				Expect(token).ToNot(BeEmpty())
				Expect(sa).ToNot(BeNil())

				clientObj, err := generateClientFromClusterServiceAccount(ctrl.GetConfigOrDie(), token)
				Expect(err).To(BeNil())
				Expect(clientObj).ToNot(BeNil())

				err = clientObj.Delete(ctx, sa)
				Expect(err).To(BeNil())
			})
		})

		When("Token secret is not found", func() {
			It("Should create a new token secret and attach to service account", func() {

				const (
					uuid               = "my-uuid"
					serviceAccountName = ArgoCDManagerServiceAccountPrefix + "my-uuid"
					serviceAccountNS   = "kube-system"
				)
				sa, err := getOrCreateServiceAccount(ctx, k8sClient, serviceAccountName, serviceAccountNS, log)
				Expect(err).To(BeNil())

				secret, err := getServiceAccountTokenSecret(ctx, k8sClient, sa)
				Expect(err).To(BeNil())

				By("check if a new token secret is created")
				if secret == nil {
					token, sa, err := InstallServiceAccount(ctx, k8sClient, uuid, serviceAccountNS, log)
					Expect(err).To(BeNil())
					Expect(token).ToNot(BeEmpty())
					Expect(sa).ToNot(BeNil())

					clientObj, err := generateClientFromClusterServiceAccount(ctrl.GetConfigOrDie(), token)
					Expect(err).To(BeNil())
					Expect(clientObj).ToNot(BeNil())

					sa, err = getOrCreateServiceAccount(ctx, k8sClient, sa.Name, sa.Namespace, log)
					Expect(err).To(BeNil())

					secret, err := getServiceAccountTokenSecret(ctx, k8sClient, sa)
					Expect(err).To(BeNil())
					Expect(secret).ToNot(BeNil())
					Expect(secret.Type).To(Equal(corev1.SecretTypeServiceAccountToken))

					err = clientObj.Delete(ctx, secret)
					Expect(err).To(BeNil())
				}

				err = k8sClient.Delete(ctx, sa)
				Expect(err).To(BeNil())
			})
		})
	})
})
