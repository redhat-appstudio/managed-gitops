package utils

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	argosharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
)

const (
	apiHost = "example.org"
	token   = "QWJDZEVmMTIzNDU2Cg=="
)

var _ = Describe("Test SetupArgoCD()", func() {

	Context("SetupArgoCD() Unit Tests", func() {
		var ctx context.Context
		var k8sClient client.WithWatch
		var logger logr.Logger
		var namespace *corev1.Namespace

		BeforeEach(func() {
			ctx = context.Background()
			logger = log.FromContext(ctx)

			var err error
			var scheme *runtime.Scheme
			scheme, _, _, namespace, err = tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())
			Expect(namespace).ToNot(BeNil())

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace).Build()

		})

		When("SetupArgoCD() is called", func() {
			It("creates the required service account, secret, cluster role, rolebinding and cluster secret", func() {
				By("starting a goroutine to add a token to the secret created by SetupArgoCD()")
				startSecretUpdater(ctx, k8sClient)

				By("calling SetupArgoCD()")
				err := SetupArgoCD(ctx, apiHost, namespace.Name, k8sClient, logger)
				Expect(err).ToNot(HaveOccurred())

				By("ensuring the service account was created")
				serviceAccount := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ArgoCDManagerServiceAccountName,
						Namespace: KubeSystemNamespace,
					},
				}
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount)
				Expect(err).ToNot(HaveOccurred())

				By("ensuring the secret was created")
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ArgoCDManagerSecretName,
						Namespace: KubeSystemNamespace,
					},
				}
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
				Expect(err).ToNot(HaveOccurred())
				Expect(secret.Annotations["kubernetes.io/service-account.name"]).To(Equal(serviceAccount.Name))
				Expect(secret.Type).To(Equal(corev1.SecretType("kubernetes.io/service-account-token")))

				By("ensuring the cluster role was created")
				clusterRole := &rbac.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ArgoCDManagerClusterRoleName,
						Namespace: KubeSystemNamespace,
					},
				}
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterRole), clusterRole)
				Expect(err).ToNot(HaveOccurred())
				Expect(clusterRole.Rules).To(Equal([]rbac.PolicyRule{
					{
						APIGroups:       []string{"*"},
						Resources:       []string{"*"},
						Verbs:           []string{"*"},
						NonResourceURLs: nil,
					}}))

				By("ensuring the cluster role binding was created")
				clusterRoleBinding := &rbac.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ArgoCDManagerClusterRoleBindingName,
						Namespace: KubeSystemNamespace,
					},
				}
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterRoleBinding), clusterRoleBinding)
				Expect(err).ToNot(HaveOccurred())
				Expect(clusterRoleBinding.RoleRef).To(Equal(rbac.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     ArgoCDManagerClusterRoleName,
				}))
				Expect(clusterRoleBinding.Subjects).To(Equal([]rbac.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      ArgoCDManagerServiceAccountName,
						Namespace: KubeSystemNamespace,
					}}))

				By("ensuring the cluster secret was created")
				clusterSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-cluster-secret",
						Namespace: namespace.Name,
					},
				}
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterSecret), clusterSecret)
				Expect(err).ToNot(HaveOccurred())
				Expect(clusterSecret.Type).To(Equal(corev1.SecretType("Opaque")))
				Expect(clusterSecret.Labels).To(Equal(map[string]string{sharedutil.ArgoCDSecretTypeIdentifierKey: sharedutil.ArgoCDSecretClusterTypeValue}))
				clusterSecretConfigJSON := argosharedutil.ClusterSecretConfigJSON{
					BearerToken: string(token),
					TLSClientConfig: argosharedutil.ClusterSecretTLSClientConfigJSON{
						Insecure: true,
					},
				}
				jsonString, err := json.Marshal(clusterSecretConfigJSON)
				Expect(err).ToNot(HaveOccurred())
				Expect(clusterSecret.StringData).To(Equal(map[string]string{
					"name":   ClusterSecretName,
					"server": apiHost,
					"config": string(jsonString),
				}))
			})
		})

		When("SetupArgoCD() is called and the resources already exist", func() {
			It("creates the required service account, secret, cluster role, rolebinding and cluster secret", func() {
				By("starting a goroutine to add a token to the secret created by SetupArgoCD()")
				startSecretUpdater(ctx, k8sClient)

				By("creating the service account")
				serviceAccount := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ArgoCDManagerServiceAccountName,
						Namespace: KubeSystemNamespace,
					},
					Secrets: []corev1.ObjectReference{
						{
							Name:      "secret",
							Namespace: "namespace",
						},
					},
					ImagePullSecrets: []corev1.LocalObjectReference{
						{
							Name: "image-pull-secret",
						},
					},
					AutomountServiceAccountToken: ptr.To(false),
				}
				err := k8sClient.Create(ctx, serviceAccount)
				Expect(err).ToNot(HaveOccurred())

				By("creating the secret")
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ArgoCDManagerSecretName,
						Namespace: KubeSystemNamespace,
					},
					Immutable: ptr.To(false),
					Data:      map[string][]byte{},
					Type:      corev1.SecretType("example"),
				}
				err = k8sClient.Create(ctx, secret)
				Expect(err).ToNot(HaveOccurred())

				By("creating the cluster role")
				clusterRole := &rbac.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ArgoCDManagerClusterRoleName,
						Namespace: KubeSystemNamespace,
					},
					Rules: []rbac.PolicyRule{
						{
							APIGroups:       []string{"a"},
							Resources:       []string{"a"},
							Verbs:           []string{"a"},
							NonResourceURLs: []string{"a"},
						},
					},
					AggregationRule: &rbac.AggregationRule{
						ClusterRoleSelectors: []metav1.LabelSelector{
							{
								MatchLabels: map[string]string{},
							},
						},
					},
				}
				err = k8sClient.Create(ctx, clusterRole)
				Expect(err).ToNot(HaveOccurred())

				By("creating the cluster role binding")
				clusterRoleBinding := &rbac.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ArgoCDManagerClusterRoleBindingName,
						Namespace: KubeSystemNamespace,
					},
					Subjects: []rbac.Subject{
						{
							Kind:     "a",
							APIGroup: "a",
						},
					},
					RoleRef: rbac.RoleRef{
						Kind:     "a",
						APIGroup: "a",
					},
				}
				err = k8sClient.Create(ctx, clusterRoleBinding)
				Expect(err).ToNot(HaveOccurred())

				By("creating the cluster secret")
				clusterSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-cluster-secret",
						Namespace: namespace.Name,
						Labels:    map[string]string{"some-key": "some value"},
					},
					Immutable: ptr.To(false),
					Data:      map[string][]byte{},
					Type:      corev1.SecretType("example"),
				}
				err = k8sClient.Create(ctx, clusterSecret)
				Expect(err).ToNot(HaveOccurred())

				By("calling SetupArgoCD()")
				err = SetupArgoCD(ctx, apiHost, namespace.Name, k8sClient, logger)
				Expect(err).ToNot(HaveOccurred())

				By("ensuring the service account was updated")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(serviceAccount), serviceAccount)
				Expect(err).ToNot(HaveOccurred())
				Expect(serviceAccount.Secrets).To(BeEmpty())
				Expect(serviceAccount.ImagePullSecrets).To(BeEmpty())
				Expect(serviceAccount.AutomountServiceAccountToken).To(BeNil())

				By("ensuring the secret was updated")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
				Expect(err).ToNot(HaveOccurred())
				Expect(secret.Annotations["kubernetes.io/service-account.name"]).To(Equal(serviceAccount.Name))
				Expect(secret.Immutable).To(BeNil())
				Expect(secret.Data).ToNot(BeEmpty())
				Expect(secret.Type).To(Equal(corev1.SecretType("kubernetes.io/service-account-token")))

				By("ensuring the cluster role was updated")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterRole), clusterRole)
				Expect(err).ToNot(HaveOccurred())
				Expect(clusterRole.Rules).To(Equal([]rbac.PolicyRule{
					{
						APIGroups:       []string{"*"},
						Resources:       []string{"*"},
						Verbs:           []string{"*"},
						NonResourceURLs: nil,
					}}))
				Expect(clusterRole.AggregationRule).To(BeNil())

				By("ensuring the cluster role binding was updated")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterRoleBinding), clusterRoleBinding)
				Expect(err).ToNot(HaveOccurred())
				Expect(clusterRoleBinding.RoleRef).To(Equal(rbac.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     ArgoCDManagerClusterRoleName,
				}))
				Expect(clusterRoleBinding.Subjects).To(Equal([]rbac.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      ArgoCDManagerServiceAccountName,
						Namespace: KubeSystemNamespace,
					}}))

				By("ensuring the cluster secret was updated")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterSecret), clusterSecret)
				Expect(err).ToNot(HaveOccurred())
				Expect(clusterSecret.Immutable).To(BeNil())
				Expect(clusterSecret.Type).To(Equal(corev1.SecretType("Opaque")))
				Expect(clusterSecret.Labels).To(Equal(map[string]string{sharedutil.ArgoCDSecretTypeIdentifierKey: sharedutil.ArgoCDSecretClusterTypeValue}))
				clusterSecretConfigJSON := argosharedutil.ClusterSecretConfigJSON{
					BearerToken: string(token),
					TLSClientConfig: argosharedutil.ClusterSecretTLSClientConfigJSON{
						Insecure: true,
					},
				}
				jsonString, err := json.Marshal(clusterSecretConfigJSON)
				Expect(err).ToNot(HaveOccurred())
				Expect(clusterSecret.StringData).To(Equal(map[string]string{
					"name":   ClusterSecretName,
					"server": apiHost,
					"config": string(jsonString),
				}))
			})
		})
	})
})

func startSecretUpdater(ctx context.Context, k8sClient client.Client) {
	go func() {
		err := wait.PollUntilContextTimeout(ctx, time.Second*1, time.Second*120, true, func(ctx context.Context) (bool, error) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ArgoCDManagerSecretName,
					Namespace: KubeSystemNamespace,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			if err != nil && apierr.IsNotFound(err) {
				return false, nil
			}
			Expect(err).ToNot(HaveOccurred())

			if _, exists := secret.Annotations["kubernetes.io/service-account.name"]; !exists {
				return false, nil
			}

			secret.Data = map[string][]byte{
				"token": []byte(token),
			}
			err = k8sClient.Update(ctx, secret)
			Expect(err).ToNot(HaveOccurred())

			return true, nil
		})
		Expect(err).ToNot(HaveOccurred())
	}()
}
