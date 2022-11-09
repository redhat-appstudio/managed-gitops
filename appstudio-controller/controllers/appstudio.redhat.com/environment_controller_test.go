package appstudioredhatcom

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudioshared "github.com/redhat-appstudio/application-api/api/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Environment controller tests", func() {

	ctx := context.Background()

	var k8sClient client.Client
	var reconciler EnvironmentReconciler
	var apiNamespace corev1.Namespace

	Context("Reconcile function call tests", func() {

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				namespace,
				err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appstudioshared.AddToScheme(scheme)
			Expect(err).To(BeNil())

			apiNamespace = *namespace

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespace, argocdNamespace, kubesystemNamespace).
				Build()

			reconciler = EnvironmentReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}

		})

		It("should create a GitOpsDeploymentManagedEnvironment, if the Environment is created", func() {
			var err error

			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: apiNamespace.Name,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					"kubeconfig": ([]byte)("{}"),
				},
			}
			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudioshared.EnvironmentSpec{
					Type:               appstudioshared.EnvironmentType_POC,
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudioshared.EnvironmentConfiguration{},
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							TargetNamespace:          "my-target-namespace",
							APIURL:                   "https://my-api-url",
							ClusterCredentialsSecret: secret.Name,
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).To(BeNil())

			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-" + env.Name,
					Namespace: req.Namespace,
				},
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).To(BeNil(), "the ManagedEnvironment object should have been created by the reconciler")

			Expect(managedEnvCR.Spec.APIURL).To(Equal(env.Spec.UnstableConfigurationFields.APIURL),
				"ManagedEnvironment should match the Environment")
			Expect(managedEnvCR.Spec.ClusterCredentialsSecret).To(Equal(env.Spec.UnstableConfigurationFields.ClusterCredentialsSecret),
				"ManagedEnvironment should match the Environment")
		})

		It("should update a GitOpsDeploymentManagedEnvironment, if the Environment is updated", func() {

			var err error

			By("creating first managed environment Secret")
			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: apiNamespace.Name,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					"kubeconfig": ([]byte)("{}"),
				},
			}
			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			By("creating second managed environment Secret")
			secret2 := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret-2",
					Namespace: apiNamespace.Name,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					"kubeconfig": ([]byte)("{}"),
				},
			}
			err = k8sClient.Create(ctx, &secret2)
			Expect(err).To(BeNil())

			By("creating an Environment pointing to the first secret")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudioshared.EnvironmentSpec{
					Type:               appstudioshared.EnvironmentType_POC,
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudioshared.EnvironmentConfiguration{},
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							TargetNamespace:          "my-target-namespace",
							APIURL:                   "https://my-api-url",
							ClusterCredentialsSecret: secret2.Name,
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			By("creating a managed environment containing outdated values, versus what's in the environment")

			previouslyReconciledManagedEnv := generateEmptyManagedEnvironment(env.Name, env.Namespace)
			previouslyReconciledManagedEnv.Spec = managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
				APIURL:                   "https://old-api-url",
				ClusterCredentialsSecret: secret.Name,
			}
			err = k8sClient.Create(ctx, &previouslyReconciledManagedEnv)
			Expect(err).To(BeNil())

			By("reconciling the ManagedEnvironment")
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).To(BeNil())

			By("retrieving the update ManagedEnvironment")
			newManagedEnv := generateEmptyManagedEnvironment(env.Name, env.Namespace)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&newManagedEnv), &newManagedEnv)
			Expect(err).To(BeNil())

			Expect(newManagedEnv.Spec.APIURL).To(Equal(env.Spec.UnstableConfigurationFields.APIURL),
				"ManagedEnvironment should match the new Environment spec, not the old value of the managed env")
			Expect(newManagedEnv.Spec.ClusterCredentialsSecret).To(Equal(env.Spec.UnstableConfigurationFields.ClusterCredentialsSecret),
				"ManagedEnvironment should match the Environment, not the old value")

			By("reconciling again, and confirming that nothing changed")
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).To(BeNil())
			Expect(newManagedEnv.Spec.APIURL).To(Equal(env.Spec.UnstableConfigurationFields.APIURL),
				"ManagedEnvironment should continue to match the new Environment spec")
			Expect(newManagedEnv.Spec.ClusterCredentialsSecret).To(Equal(env.Spec.UnstableConfigurationFields.ClusterCredentialsSecret),
				"ManagedEnvironment should continue to match the Environment spec")

		})

		It("should not return an error, if the Environment is deleted", func() {

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "no-longer-exists",
					Namespace: apiNamespace.Name,
				},
			}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).To(BeNil())

		})

		It("should return an error if the Environment references a Secret that doesn't exist", func() {

			By("creating an Environment resource pointing to a Secret that doesn't exist")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudioshared.EnvironmentSpec{
					Type:               appstudioshared.EnvironmentType_POC,
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudioshared.EnvironmentConfiguration{},
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							TargetNamespace:          "my-target-namespace",
							APIURL:                   "https://my-api-url",
							ClusterCredentialsSecret: "secret-that-doesnt-exist",
						},
					},
				},
			}
			err := k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			By("reconciling the ManagedEnvironment")
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(BeNil())
		})

		It("should not return an error if the Environment does not container UnstableConfigurationFields", func() {

			By("creating an Environment resource pointing to a Secret that doesn't exist")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudioshared.EnvironmentSpec{
					Type:               appstudioshared.EnvironmentType_POC,
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudioshared.EnvironmentConfiguration{},
				},
			}
			err := k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			By("reconciling the ManagedEnvironment")
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).To(BeNil())
		})

		It("should return an error if the TargetNamespace field is missing", func() {

			var err error

			By("creating managed environment Secret")
			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-my-managed-env-secret",
					Namespace: apiNamespace.Name,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
				Data: map[string][]byte{
					"kubeconfig": ([]byte)("{}"),
				},
			}
			err = k8sClient.Create(ctx, &secret)
			Expect(err).To(BeNil())

			By("creating an Environment resource pointing with an invalid target namespace field")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudioshared.EnvironmentSpec{
					Type:               appstudioshared.EnvironmentType_POC,
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudioshared.EnvironmentConfiguration{},
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							TargetNamespace:          "",
							APIURL:                   "https://my-api-url",
							ClusterCredentialsSecret: "my-secret",
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			By("reconciling the ManagedEnvironment")
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(BeNil())

		})

	})
})
