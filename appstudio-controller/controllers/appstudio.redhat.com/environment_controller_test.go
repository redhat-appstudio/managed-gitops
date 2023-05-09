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
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Environment controller tests", func() {

	Context("Reconcile function call tests", func() {

		ctx := context.Background()

		var k8sClient client.Client
		var reconciler EnvironmentReconciler
		var apiNamespace corev1.Namespace

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

		createEnvironmentTest := func(allowInsecureSkipTLSVerifyParam, clusterResources bool, namespaces []string) {
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
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudioshared.EnvironmentConfiguration{},
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							TargetNamespace:            "my-target-namespace",
							APIURL:                     "https://my-api-url",
							ClusterCredentialsSecret:   secret.Name,
							AllowInsecureSkipTLSVerify: allowInsecureSkipTLSVerifyParam,
							ClusterResources:           clusterResources,
							Namespaces:                 namespaces,
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

			By("verify that error condition is not set")
			Expect(env.Status.Conditions).To(BeNil())

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
			Expect(managedEnvCR.Spec.AllowInsecureSkipTLSVerify).To(Equal(env.Spec.UnstableConfigurationFields.AllowInsecureSkipTLSVerify),
				"ManagedEnvironment should match the Environment")
			Expect(managedEnvCR.Spec.Namespaces).To(Equal(env.Spec.UnstableConfigurationFields.Namespaces),
				"ManagedEnvironment should match the Environment")
			Expect(managedEnvCR.Spec.ClusterResources).To(Equal(env.Spec.UnstableConfigurationFields.ClusterResources),
				"ManagedEnvironment should match the Environment")
		}

		It("should create a GitOpsDeploymentManagedEnvironment, if the Environment is created where AllowInsecureSkipTLSVerify field is true", func() {
			createEnvironmentTest(true, false, nil)
		})

		It("should create a GitOpsDeploymentManagedEnvironment, if the Environment is created where AllowInsecureSkipTLSVerify field is false", func() {
			createEnvironmentTest(false, false, nil)
		})

		It("should create a GitOpsDeploymentManagedEnvironment, if the Environment is created where ClusterResources is true and namespaces are specified", func() {
			createEnvironmentTest(false, true, []string{
				"namespace-1",
				"namespace-2",
			})
		})

		updateEnvTest := func(allowInsecureSkipTLSVerifyParam, initialClusterResources, updatedClusterResources bool, initialNamespaces, updatedNamespaces []string) {
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
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudioshared.EnvironmentConfiguration{},
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							TargetNamespace:            "my-target-namespace",
							APIURL:                     "https://my-api-url",
							ClusterCredentialsSecret:   secret2.Name,
							AllowInsecureSkipTLSVerify: allowInsecureSkipTLSVerifyParam,
							ClusterResources:           initialClusterResources,
							Namespaces:                 initialNamespaces,
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			By("creating a managed environment containing outdated values, versus what's in the environment")

			previouslyReconciledManagedEnv := generateEmptyManagedEnvironment(env.Name, env.Namespace)
			previouslyReconciledManagedEnv.Spec = managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironmentSpec{
				APIURL:                     "https://old-api-url",
				ClusterCredentialsSecret:   secret.Name,
				AllowInsecureSkipTLSVerify: !allowInsecureSkipTLSVerifyParam,
				ClusterResources:           updatedClusterResources,
				Namespaces:                 updatedNamespaces,
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

			By("verify that error condition is not set")
			Expect(env.Status.Conditions).To(BeNil())

			By("retrieving the update ManagedEnvironment")
			newManagedEnv := generateEmptyManagedEnvironment(env.Name, env.Namespace)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&newManagedEnv), &newManagedEnv)
			Expect(err).To(BeNil())

			Expect(newManagedEnv.Spec.APIURL).To(Equal(env.Spec.UnstableConfigurationFields.APIURL),
				"ManagedEnvironment should match the new Environment spec, not the old value of the managed env")
			Expect(newManagedEnv.Spec.ClusterCredentialsSecret).To(Equal(env.Spec.UnstableConfigurationFields.ClusterCredentialsSecret),
				"ManagedEnvironment should match the Environment, not the old value")
			Expect(newManagedEnv.Spec.AllowInsecureSkipTLSVerify).To(Equal(env.Spec.UnstableConfigurationFields.AllowInsecureSkipTLSVerify),
				"ManagedEnvironment should match the Environment, not the old value")
			Expect(newManagedEnv.Spec.Namespaces).To(Equal(env.Spec.UnstableConfigurationFields.Namespaces),
				"ManagedEnvironment should match the Environment, not the old value")
			Expect(newManagedEnv.Spec.ClusterResources).To(Equal(env.Spec.UnstableConfigurationFields.ClusterResources),
				"ManagedEnvironment should match the Environment, not the old value")

			By("reconciling again, and confirming that nothing changed")
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).To(BeNil())

			By("retrieving the update ManagedEnvironment")
			newManagedEnv = generateEmptyManagedEnvironment(env.Name, env.Namespace)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&newManagedEnv), &newManagedEnv)
			Expect(err).To(BeNil())

			By("verify that error condition is not set")
			Expect(env.Status.Conditions).To(BeNil())

			Expect(newManagedEnv.Spec.APIURL).To(Equal(env.Spec.UnstableConfigurationFields.APIURL),
				"ManagedEnvironment should continue to match the new Environment spec")
			Expect(newManagedEnv.Spec.ClusterCredentialsSecret).To(Equal(env.Spec.UnstableConfigurationFields.ClusterCredentialsSecret),
				"ManagedEnvironment should continue to match the Environment spec")
			Expect(newManagedEnv.Spec.AllowInsecureSkipTLSVerify).To(Equal(env.Spec.UnstableConfigurationFields.AllowInsecureSkipTLSVerify),
				"ManagedEnvironment should continue to match the new Environment spec")
			Expect(newManagedEnv.Spec.Namespaces).To(Equal(env.Spec.UnstableConfigurationFields.Namespaces),
				"ManagedEnvironment should continue to match the new Environment spec")
			Expect(newManagedEnv.Spec.ClusterResources).To(Equal(env.Spec.UnstableConfigurationFields.ClusterResources),
				"ManagedEnvironment should continue to match the new Environment spec")
		}

		It("should update a GitOpsDeploymentManagedEnvironment,  if the Environment is updated where AllowInsecureSkipTLSVerify field is true", func() {
			updateEnvTest(true, false, false, nil, nil)
		})

		It("should update a GitOpsDeploymentManagedEnvironment, if the Environment is updated where AllowInsecureSkipTLSVerify field is false", func() {
			updateEnvTest(false, false, false, nil, nil)
		})

		It("should update a GitOpsDeploymentManagedEnvironment, if the Environment is updated where ClusterResources and Namespaces fields are updated", func() {
			updateEnvTest(false, true, false,
				[]string{
					"namespace-1",
					"namespace-2",
				}, []string{
					"namespace-1",
					"namespace-2",
					"namespace-3",
				})
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

		It("should return an error if both cluster credentials and DeploymentTargetClaim are provided", func() {
			By("create an environment with both cluster credentials and DTC specified")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudioshared.EnvironmentSpec{
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							APIURL:                   "abc",
							ClusterCredentialsSecret: "test",
						},
					},
					Configuration: appstudioshared.EnvironmentConfiguration{
						Target: appstudioshared.EnvironmentTarget{
							DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
								ClaimName: "test-dtc",
							},
						},
					},
				},
			}

			err := k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			By("check if an error is returned after reconciling")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(res).To(Equal(reconcile.Result{}))
			Expect(err).To(BeNil())

			By("Checking status field after calling Reconciler")
			env = appstudioshared.Environment{}
			err = reconciler.Get(ctx, req.NamespacedName, &env)
			Expect(err).To(BeNil())
			Expect(len(env.Status.Conditions)).To(Equal(1))
			Expect(env.Status.Conditions[0].Type).To(Equal(EnvironmentConditionErrorOccurred))
			Expect(env.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(env.Status.Conditions[0].Reason).To(Equal(EnvironmentReasonErrorOccurred))
			Expect(env.Status.Conditions[0].Message).To(Equal("Environment is invalid since it cannot have both DeploymentTargetClaim and credentials configuration set"))
		})

		It("should manage an Environment with DeploymentTargetClaim specified", func() {
			By("create a DT and DTC with cluster credentials")
			clusterSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: apiNamespace.Name,
				},
			}

			err := k8sClient.Create(ctx, &clusterSecret)
			Expect(err).To(BeNil())

			dt := appstudioshared.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dt",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudioshared.DeploymentTargetSpec{
					KubernetesClusterCredentials: appstudioshared.DeploymentTargetKubernetesClusterCredentials{
						APIURL:                     "https://test-url",
						ClusterCredentialsSecret:   "test-secret",
						AllowInsecureSkipTLSVerify: true,
					},
				},
				Status: appstudioshared.DeploymentTargetStatus{
					Phase: appstudioshared.DeploymentTargetPhase_Bound,
				},
			}

			err = k8sClient.Create(ctx, &dt)
			Expect(err).To(BeNil())

			dtc := appstudioshared.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dtc",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudioshared.DeploymentTargetClaimSpec{
					TargetName: dt.Name,
				},
				Status: appstudioshared.DeploymentTargetClaimStatus{
					Phase: appstudioshared.DeploymentTargetClaimPhase_Bound,
				},
			}

			err = k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			By("create an Environment that refer the above DTC")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: dtc.Namespace,
				},
				Spec: appstudioshared.EnvironmentSpec{
					Configuration: appstudioshared.EnvironmentConfiguration{
						Target: appstudioshared.EnvironmentTarget{
							DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			By("reconcile and verify if a ManagedEnvironment is created with the right credentials")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).To(BeNil())
			Expect(res).To(Equal(reconcile.Result{}))

			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-" + env.Name,
					Namespace: req.Namespace,
				},
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).To(BeNil())

			By("verify if the environment credentials match with the DT")
			Expect(managedEnvCR.Spec.APIURL).To(Equal(dt.Spec.KubernetesClusterCredentials.APIURL))
			Expect(managedEnvCR.Spec.ClusterCredentialsSecret).To(Equal(dt.Spec.KubernetesClusterCredentials.ClusterCredentialsSecret))
			Expect(managedEnvCR.Spec.AllowInsecureSkipTLSVerify).To(Equal(dt.Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify))
		})

		It("should return and wait if the specified DTC is not in Bounded phase", func() {
			dtc := appstudioshared.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dtc",
					Namespace: apiNamespace.Name,
				},
			}

			err := k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			By("create an Environment that refer the above DTC")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: dtc.Namespace,
				},
				Spec: appstudioshared.EnvironmentSpec{
					Configuration: appstudioshared.EnvironmentConfiguration{
						Target: appstudioshared.EnvironmentTarget{
							DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			By("reconcile and verify that a ManagedEnvironment is not created")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).To(BeNil())
			Expect(res).To(Equal(reconcile.Result{}))

			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-" + env.Name,
					Namespace: req.Namespace,
				},
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).ToNot(BeNil())
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("should return an error if the DeploymentTarget is not found", func() {
			dt := appstudioshared.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dt",
					Namespace: apiNamespace.Name,
				},
			}

			err := k8sClient.Create(ctx, &dt)
			Expect(err).To(BeNil())

			dtc := appstudioshared.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dtc",
					Namespace: apiNamespace.Name,
				},
				Status: appstudioshared.DeploymentTargetClaimStatus{
					Phase: appstudioshared.DeploymentTargetClaimPhase_Bound,
				},
			}

			err = k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			By("create and Environment that refer the above DTC")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: dtc.Namespace,
				},
				Spec: appstudioshared.EnvironmentSpec{
					Configuration: appstudioshared.EnvironmentConfiguration{
						Target: appstudioshared.EnvironmentTarget{
							DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			By("reconcile and verify if an error is returned")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).To(BeNil())
			Expect(res).To(Equal(reconcile.Result{}))

			By("Checking status field after calling Reconciler")
			env = appstudioshared.Environment{}
			err = reconciler.Get(ctx, req.NamespacedName, &env)
			Expect(err).To(BeNil())
			Expect(len(env.Status.Conditions)).To(Equal(1))
			Expect(env.Status.Conditions[0].Type).To(Equal(EnvironmentConditionErrorOccurred))
			Expect(env.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(env.Status.Conditions[0].Reason).To(Equal(EnvironmentReasonErrorOccurred))
			Expect(env.Status.Conditions[0].Message).To(Equal("DeploymentTarget not found for DeploymentTargetClaim"))
		})

		It("shouldn't process the Environment if neither credentials nor DTC is provided", func() {
			By("create an Environment without DTC and credentials")
			env := appstudioshared.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: apiNamespace.Name,
				},
			}
			err := k8sClient.Create(ctx, &env)
			Expect(err).To(BeNil())

			By("reconcile and verify that a ManagedEnvironment is not created")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).To(BeNil())
			Expect(res).To(Equal(reconcile.Result{}))

			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-" + env.Name,
					Namespace: req.Namespace,
				},
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).ToNot(BeNil())
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("Should not error out if the namespaces and clusterResources fields are not set in the Environment", func() {
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
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudioshared.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudioshared.EnvironmentConfiguration{},
					UnstableConfigurationFields: &appstudioshared.UnstableEnvironmentConfiguration{
						KubernetesClusterCredentials: appstudioshared.KubernetesClusterCredentials{
							TargetNamespace:            "my-target-namespace",
							APIURL:                     "https://my-api-url",
							ClusterCredentialsSecret:   secret.Name,
							AllowInsecureSkipTLSVerify: false,
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

			Expect(managedEnvCR.Spec.Namespaces).To(BeEmpty())
			Expect(managedEnvCR.Spec.ClusterResources).To(BeFalse())
		})

		Context("Test findObjectsForDeploymentTargetClaim function", func() {
			It("should map requests if matching Environments are found", func() {
				dtc := appstudioshared.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: apiNamespace.Name,
					},
				}

				By("create Environments that refer the above DTC")
				env1 := appstudioshared.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env-1",
						Namespace: dtc.Namespace,
					},
					Spec: appstudioshared.EnvironmentSpec{
						Configuration: appstudioshared.EnvironmentConfiguration{
							Target: appstudioshared.EnvironmentTarget{
								DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}
				err := k8sClient.Create(ctx, &env1)
				Expect(err).To(BeNil())

				env2 := appstudioshared.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env-2",
						Namespace: dtc.Namespace,
					},
				}
				env2.Spec.Configuration.Target.DeploymentTargetClaim.ClaimName = dtc.Name
				env2.ResourceVersion = ""
				err = k8sClient.Create(ctx, &env2)
				Expect(err).To(BeNil())

				By("check if the requests are mapped to the correct environments")
				expectedReqs := map[string]int{
					env1.Name: 1,
					env2.Name: 1,
				}
				reqs := reconciler.findObjectsForDeploymentTargetClaim(&dtc)
				Expect(len(reqs)).To(Equal(len(expectedReqs)))
				for _, r := range reqs {
					Expect(expectedReqs[r.Name]).To(Equal(1))
					expectedReqs[r.Name]--
				}
			})

			It("shouldn't map any requests if no matching Environment is found", func() {
				dtc := appstudioshared.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-dtc",
						Namespace: apiNamespace.Name,
					},
				}

				reqs := reconciler.findObjectsForDeploymentTargetClaim(&dtc)

				Expect(reqs).To(Equal([]reconcile.Request{}))
			})

			It("shouldn't map any requests if an incompatible object is passed", func() {
				dt := appstudioshared.DeploymentTarget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-dt",
						Namespace: apiNamespace.Name,
					},
				}

				reqs := reconciler.findObjectsForDeploymentTargetClaim(&dt)

				Expect(reqs).To(Equal([]reconcile.Request{}))
			})
		})

		Context("Test findObjectsForDeploymentTarget function", func() {
			It("should map requests if matching environments are found", func() {
				By("create a DT and DTC that target each other")
				dt := appstudioshared.DeploymentTarget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dt",
						Namespace: apiNamespace.Name,
					},
				}

				dtc := appstudioshared.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudioshared.DeploymentTargetClaimSpec{
						TargetName: dt.Name,
					},
				}

				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("create Environments that refer the above DTC")
				env1 := appstudioshared.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env-1",
						Namespace: dtc.Namespace,
					},
					Spec: appstudioshared.EnvironmentSpec{
						Configuration: appstudioshared.EnvironmentConfiguration{
							Target: appstudioshared.EnvironmentTarget{
								DeploymentTargetClaim: appstudioshared.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}
				err = k8sClient.Create(ctx, &env1)
				Expect(err).To(BeNil())

				env2 := appstudioshared.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env-2",
						Namespace: dtc.Namespace,
					},
				}
				env2.Spec.Configuration.Target.DeploymentTargetClaim.ClaimName = dtc.Name
				env2.ResourceVersion = ""
				err = k8sClient.Create(ctx, &env2)
				Expect(err).To(BeNil())

				By("check if the requests are mapped to the correct environments")
				expectedReqs := map[string]int{
					env1.Name: 1,
					env2.Name: 1,
				}
				reqs := reconciler.findObjectsForDeploymentTarget(&dt)
				Expect(len(reqs)).To(Equal(len(expectedReqs)))
				for _, r := range reqs {
					Expect(expectedReqs[r.Name]).To(Equal(1))
					expectedReqs[r.Name]--
				}
			})

			It("shouldn't map any requests if no matching environments are found", func() {
				dt := appstudioshared.DeploymentTarget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-dt",
						Namespace: apiNamespace.Name,
					},
				}

				reqs := reconciler.findObjectsForDeploymentTarget(&dt)

				Expect(reqs).To(Equal([]reconcile.Request{}))
			})

			It("shouldn't map any requests if an incompatible object is passed", func() {
				dtc := appstudioshared.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-dtc",
						Namespace: apiNamespace.Name,
					},
				}

				reqs := reconciler.findObjectsForDeploymentTarget(&dtc)

				Expect(reqs).To(Equal([]reconcile.Request{}))
			})
		})
	})

	Context("Unit tests of non-reconcile functions", func() {

		ctx := context.Background()

		var k8sClient client.Client
		var apiNamespace corev1.Namespace

		log := log.FromContext(ctx)

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

		})

		DescribeTable("verify updateStatusConditionOfEnvironment works as expected",
			func(preCondition []metav1.Condition, newCondition metav1.Condition, expectedResult []metav1.Condition) {

				env := appstudioshared.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-env",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudioshared.EnvironmentSpec{
						DisplayName:        "my-environment",
						DeploymentStrategy: appstudioshared.DeploymentStrategy_Manual,
						ParentEnvironment:  "",
						Tags:               []string{},
						Configuration:      appstudioshared.EnvironmentConfiguration{},
					},
				}
				err := k8sClient.Create(ctx, &env)
				Expect(err).To(BeNil())

				env.Status.Conditions = preCondition
				err = k8sClient.Update(ctx, &env)
				Expect(err).To(BeNil())

				err = updateStatusConditionOfEnvironment(ctx, k8sClient, newCondition.Message, &env, EnvironmentConditionErrorOccurred,
					newCondition.Status, newCondition.Reason, log)
				Expect(err).To(BeNil())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&env), &env)
				Expect(err).To(BeNil())

				Expect(len(env.Status.Conditions)).To(BeNumerically("==", 1))

				expectedCondition := expectedResult[0]

				actualCondition := env.Status.Conditions[0]

				Expect(actualCondition.Message).To(Equal(expectedCondition.Message))
				Expect(actualCondition.Type).To(Equal(expectedCondition.Type))
				Expect(actualCondition.Reason).To(Equal(expectedCondition.Reason))
				Expect(actualCondition.Status).To(Equal(expectedCondition.Status))

			},
			Entry("add a new condition", []metav1.Condition{}, metav1.Condition{
				Type:    EnvironmentConditionErrorOccurred,
				Status:  metav1.ConditionTrue,
				Reason:  "my-reason",
				Message: "my-message",
			}, []metav1.Condition{
				{
					Type:    EnvironmentConditionErrorOccurred,
					Status:  metav1.ConditionTrue,
					Reason:  "my-reason",
					Message: "my-message",
				},
			}),
			Entry("replace an existing condition with mismatched reason", []metav1.Condition{{
				Type:    EnvironmentConditionErrorOccurred,
				Status:  metav1.ConditionTrue,
				Reason:  "my-reason",
				Message: "my-message",
			}}, metav1.Condition{
				Type:    EnvironmentConditionErrorOccurred,
				Status:  metav1.ConditionTrue,
				Reason:  "my-reason2",
				Message: "my-message",
			}, []metav1.Condition{{
				Type:    EnvironmentConditionErrorOccurred,
				Status:  metav1.ConditionTrue,
				Reason:  "my-reason2",
				Message: "my-message",
			}}),

			Entry("replace an existing condition with mismatched message", []metav1.Condition{{
				Type:    EnvironmentConditionErrorOccurred,
				Status:  metav1.ConditionTrue,
				Reason:  "my-reason",
				Message: "my-message",
			}}, metav1.Condition{
				Type:    EnvironmentConditionErrorOccurred,
				Status:  metav1.ConditionTrue,
				Reason:  "my-reason",
				Message: "my-message2",
			}, []metav1.Condition{{
				Type:    EnvironmentConditionErrorOccurred,
				Status:  metav1.ConditionTrue,
				Reason:  "my-reason",
				Message: "my-message2",
			}}),

			Entry("replace an existing condition with mismatched status", []metav1.Condition{{
				Type:    EnvironmentConditionErrorOccurred,
				Status:  metav1.ConditionTrue,
				Reason:  "my-reason",
				Message: "my-message",
			}}, metav1.Condition{
				Type:    EnvironmentConditionErrorOccurred,
				Status:  metav1.ConditionFalse,
				Reason:  "my-reason",
				Message: "my-message",
			}, []metav1.Condition{{
				Type:    EnvironmentConditionErrorOccurred,
				Status:  metav1.ConditionFalse,
				Reason:  "my-reason",
				Message: "my-message",
			}}),
		)

	})
})
