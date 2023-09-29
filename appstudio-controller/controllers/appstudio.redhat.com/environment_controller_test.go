package appstudioredhatcom

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudioshared "github.com/redhat-appstudio/application-api/api/v1alpha1"
	appstudiosharedv1beta1 "github.com/redhat-appstudio/application-api/api/v1beta1"

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
			Expect(err).ToNot(HaveOccurred())

			err = appstudioshared.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			err = appstudiosharedv1beta1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1beta1.EnvironmentConfiguration{},
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
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
			Expect(err).ToNot(HaveOccurred())

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			By("verify that error condition is not set")
			Expect(env.Status.Conditions).To(BeNil())

			managedEnvCR := generateEmptyManagedEnvironment(env.Name, req.Namespace)

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).ToNot(HaveOccurred(), "the ManagedEnvironment object should have been created by the reconciler")

			Expect(managedEnvCR.Spec.APIURL).To(Equal(env.Spec.Target.APIURL),
				"ManagedEnvironment should match the Environment")
			Expect(managedEnvCR.Spec.ClusterCredentialsSecret).To(Equal(env.Spec.Target.ClusterCredentialsSecret),
				"ManagedEnvironment should match the Environment")
			Expect(managedEnvCR.Spec.AllowInsecureSkipTLSVerify).To(Equal(env.Spec.Target.AllowInsecureSkipTLSVerify),
				"ManagedEnvironment should match the Environment")
			Expect(managedEnvCR.Spec.Namespaces).To(Equal(env.Spec.Target.Namespaces),
				"ManagedEnvironment should match the Environment")
			Expect(managedEnvCR.Spec.ClusterResources).To(Equal(env.Spec.Target.ClusterResources),
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
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			By("creating an Environment pointing to the first secret")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1beta1.EnvironmentConfiguration{},
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
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
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			By("reconciling the ManagedEnvironment")
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			By("verify that error condition is not set")
			Expect(env.Status.Conditions).To(BeNil())

			By("retrieving the update ManagedEnvironment")
			newManagedEnv := generateEmptyManagedEnvironment(env.Name, env.Namespace)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&newManagedEnv), &newManagedEnv)
			Expect(err).ToNot(HaveOccurred())

			Expect(newManagedEnv.Spec.APIURL).To(Equal(env.Spec.Target.APIURL),
				"ManagedEnvironment should match the new Environment spec, not the old value of the managed env")
			Expect(newManagedEnv.Spec.ClusterCredentialsSecret).To(Equal(env.Spec.Target.ClusterCredentialsSecret),
				"ManagedEnvironment should match the Environment, not the old value")
			Expect(newManagedEnv.Spec.AllowInsecureSkipTLSVerify).To(Equal(env.Spec.Target.AllowInsecureSkipTLSVerify),
				"ManagedEnvironment should match the Environment, not the old value")
			Expect(newManagedEnv.Spec.Namespaces).To(Equal(env.Spec.Target.Namespaces),
				"ManagedEnvironment should match the Environment, not the old value")
			Expect(newManagedEnv.Spec.ClusterResources).To(Equal(env.Spec.Target.ClusterResources),
				"ManagedEnvironment should match the Environment, not the old value")

			By("reconciling again, and confirming that nothing changed")
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			By("retrieving the update ManagedEnvironment")
			newManagedEnv = generateEmptyManagedEnvironment(env.Name, env.Namespace)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&newManagedEnv), &newManagedEnv)
			Expect(err).ToNot(HaveOccurred())

			By("verify that error condition is not set")
			Expect(env.Status.Conditions).To(BeNil())

			Expect(newManagedEnv.Spec.APIURL).To(Equal(env.Spec.Target.APIURL),
				"ManagedEnvironment should continue to match the new Environment spec")
			Expect(newManagedEnv.Spec.ClusterCredentialsSecret).To(Equal(env.Spec.Target.ClusterCredentialsSecret),
				"ManagedEnvironment should continue to match the Environment spec")
			Expect(newManagedEnv.Spec.AllowInsecureSkipTLSVerify).To(Equal(env.Spec.Target.AllowInsecureSkipTLSVerify),
				"ManagedEnvironment should continue to match the new Environment spec")
			Expect(newManagedEnv.Spec.Namespaces).To(Equal(env.Spec.Target.Namespaces),
				"ManagedEnvironment should continue to match the new Environment spec")
			Expect(newManagedEnv.Spec.ClusterResources).To(Equal(env.Spec.Target.ClusterResources),
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
			Expect(err).ToNot(HaveOccurred())

		})

		It("should update the Environment status condition if the Environment references a Secret that doesn't exist, but should not return an error", func() {

			By("creating an Environment resource pointing to a Secret that doesn't exist")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1beta1.EnvironmentConfiguration{},
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
							TargetNamespace:          "my-target-namespace",
							APIURL:                   "https://my-api-url",
							ClusterCredentialsSecret: "secret-that-doesnt-exist",
						},
					},
				},
			}
			err := k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconciling the Environment")
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&env), &env)
			Expect(err).ToNot(HaveOccurred())
			expectEnvironmentStatusConditionError("the secret secret-that-doesnt-exist referenced by the Environment resource was not found", EnvironmentReasonSecretNotFound, env)

		})

		It("should update the Environment status condition when an error is resolved", func() {

			By("creating an Environment resource pointing to a Secret that doesn't exist")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1beta1.EnvironmentConfiguration{},
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
							TargetNamespace:          "my-target-namespace",
							APIURL:                   "https://my-api-url",
							ClusterCredentialsSecret: "secret-that-doesnt-exist",
						},
					},
				},
			}
			err := k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconciling the Environment")
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&env), &env)
			Expect(err).ToNot(HaveOccurred())
			expectEnvironmentStatusConditionError("the secret secret-that-doesnt-exist referenced by the Environment resource was not found", EnvironmentReasonSecretNotFound, env)

			By("fixing the error")
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
			Expect(err).ToNot(HaveOccurred())

			env.Spec.Target.ClusterCredentialsSecret = secret.Name
			Expect(k8sClient.Update(ctx, &env)).To(Succeed())

			By("reconciling again")
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&env), &env)
			Expect(err).ToNot(HaveOccurred())
			Expect(env.Status.Conditions[0].Type).To(Equal(EnvironmentConditionErrorOccurred))
			Expect(env.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(env.Status.Conditions[0].Reason).To(Equal(EnvironmentReasonErrorOccurred + "Resolved"))
			Expect(env.Status.Conditions[0].Message).To(BeEmpty())
			Expect(env.Status.Conditions[1].Type).To(Equal(EnvironmentConditionReconciled))
			Expect(env.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
			Expect(env.Status.Conditions[1].Reason).To(Equal(EnvironmentReasonSecretNotFound + "Resolved"))
			Expect(env.Status.Conditions[1].Message).To(BeEmpty())

			By("ensuring that another reconcile has no effect on the condition")
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&env), &env)
			Expect(err).ToNot(HaveOccurred())
			Expect(env.Status.Conditions[0].Type).To(Equal(EnvironmentConditionErrorOccurred))
			Expect(env.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(env.Status.Conditions[0].Reason).To(Equal(EnvironmentReasonErrorOccurred + "Resolved"))
			Expect(env.Status.Conditions[0].Message).To(BeEmpty())
			Expect(env.Status.Conditions[1].Type).To(Equal(EnvironmentConditionReconciled))
			Expect(env.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
			Expect(env.Status.Conditions[1].Reason).To(Equal(EnvironmentReasonSecretNotFound + "Resolved"))
			Expect(env.Status.Conditions[1].Message).To(BeEmpty())
		})

		It("should not return an error if the Environment does not contain UnstableConfigurationFields", func() {

			By("creating an Environment resource pointing to a Secret that doesn't exist")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1beta1.EnvironmentConfiguration{},
				},
			}
			err := k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconciling the ManagedEnvironment")
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return an error if both cluster credentials and DeploymentTargetClaim are provided", func() {
			By("create an environment with both cluster credentials and DTC specified")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
							APIURL:                   "abc",
							ClusterCredentialsSecret: "test",
						},

						Claim: appstudiosharedv1beta1.TargetClaim{
							DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
								ClaimName: "test-dtc",
							},
						},
					},
				},
			}

			err := k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("check if an error is returned after reconciling")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(res).To(Equal(reconcile.Result{}))
			Expect(err).ToNot(HaveOccurred())

			By("Checking status field after calling Reconciler")
			env = appstudiosharedv1beta1.Environment{}
			err = reconciler.Get(ctx, req.NamespacedName, &env)
			Expect(err).ToNot(HaveOccurred())
			expectEnvironmentStatusConditionError("Environment is invalid since it cannot have both DeploymentTargetClaim and credentials configuration set", EnvironmentReasonInvalid, env)

		})

		DescribeTable("should reconcile an Environment that references a DeploymentTargetClaim, and verify GitOpsDeploymentManagedEnvironment has been deleted when Environment resource is deleted", func(dtHasNamespaceField bool) {

			By("create a DT and DTC with cluster credentials")
			clusterSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: apiNamespace.Name,
				},
			}

			err := k8sClient.Create(ctx, &clusterSecret)
			Expect(err).ToNot(HaveOccurred())

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

			if dtHasNamespaceField {
				// We set the default namespace to test that this value will be set in the GitOpsDeploymentManagedGnvironment
				dt.Spec.KubernetesClusterCredentials.DefaultNamespace = "my-namespace"
			}

			err = k8sClient.Create(ctx, &dt)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			By("create an Environment that refers to the above DTC")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: dtc.Namespace,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						Claim: appstudiosharedv1beta1.TargetClaim{
							DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile and verify if a ManagedEnvironment is created with the right credentials")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			By("verify if a new managed-environment secret is created")
			managedEnvSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      generateManagedEnvSecretName(env.Name),
					Namespace: env.Namespace,
				},
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvSecret), &managedEnvSecret)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(managedEnvSecret.Type)).To(Equal(sharedutil.ManagedEnvironmentSecretType))
			Expect(reflect.DeepEqual(managedEnvSecret.Data, clusterSecret.Data)).To(BeTrue())
			Expect(managedEnvSecret.OwnerReferences[0].Name).To(Equal(env.Name))
			Expect(managedEnvSecret.OwnerReferences[0].UID).To(Equal(env.UID))
			Expect(managedEnvSecret.GetLabels()[managedEnvironmentSecretLabel]).To(Equal(env.Name))

			managedEnvCR := managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-environment-" + env.Name,
					Namespace: req.Namespace,
				},
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the environment credentials match with the DT")
			Expect(managedEnvCR.Spec.APIURL).To(Equal(dt.Spec.KubernetesClusterCredentials.APIURL))
			Expect(managedEnvCR.Spec.ClusterCredentialsSecret).To(Equal(managedEnvSecret.Name))
			Expect(managedEnvCR.Spec.AllowInsecureSkipTLSVerify).To(Equal(dt.Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify))

			if dtHasNamespaceField {
				Expect(managedEnvCR.Spec.Namespaces).To(Equal([]string{"my-namespace"}), "if the DT contains a Namespace, that same value should also be set in the GitOpsDeploymentManagedEnvironment")
			}

			By("update the credential secret and verify if the managed-environment secret is updated")
			clusterSecret.Data = map[string][]byte{
				"kubeconfig": []byte("updated"),
			}
			err = k8sClient.Update(ctx, &clusterSecret)
			Expect(err).ToNot(HaveOccurred())

			res, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvSecret), &managedEnvSecret)
			Expect(err).ToNot(HaveOccurred())
			Expect(reflect.DeepEqual(managedEnvSecret.Data, clusterSecret.Data)).To(BeTrue())

			By("delete the credential secret and verify if the managed-environment secret is deleted")
			err = k8sClient.Delete(ctx, &clusterSecret)
			Expect(err).ToNot(HaveOccurred())

			res, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&env), &env); err != nil {
				Expect(err).ToNot(HaveOccurred())
			}
			expectEnvironmentStatusConditionError("the secret test-secret referenced by the Environment resource was not found", EnvironmentReasonSecretNotFound, env)
			Expect(res).To(Equal(reconcile.Result{}))
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvSecret), &managedEnvSecret)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			By("delete environment resource")
			err = k8sClient.Delete(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			res, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			By("verify whether the GitOpsDeploymentManagedEnvironment has been deleted when the Environment resource is deleted.")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())

		},
			Entry("Initial DeploymentTarget contains a value in DefaultNamespace field", true),
			Entry("Initial DeploymentTarget does NOT contain a DefaultNamespace field", false))

		It("should manage an Environment with DeploymentTargetClaim specified", func() {
			By("create a DT and DTC with cluster credentials")
			clusterSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: apiNamespace.Name,
				},
			}

			err := k8sClient.Create(ctx, &clusterSecret)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			By("create an Environment that refer the above DTC")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: dtc.Namespace,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						Claim: appstudiosharedv1beta1.TargetClaim{
							DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile and verify if a ManagedEnvironment is created with the right credentials")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			By("verify if a new managed-environment secret is created")
			managedEnvSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      generateManagedEnvSecretName(env.Name),
					Namespace: env.Namespace,
				},
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvSecret), &managedEnvSecret)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(managedEnvSecret.Type)).To(Equal(sharedutil.ManagedEnvironmentSecretType))
			Expect(reflect.DeepEqual(managedEnvSecret.Data, clusterSecret.Data)).To(BeTrue())
			Expect(managedEnvSecret.OwnerReferences[0].Name).To(Equal(env.Name))
			Expect(managedEnvSecret.OwnerReferences[0].UID).To(Equal(env.UID))
			Expect(managedEnvSecret.GetLabels()[managedEnvironmentSecretLabel]).To(Equal(env.Name))

			managedEnvCR := generateEmptyManagedEnvironment(env.Name, req.Namespace)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the environment credentials match with the DT")
			Expect(managedEnvCR.Spec.APIURL).To(Equal(dt.Spec.KubernetesClusterCredentials.APIURL))
			Expect(managedEnvCR.Spec.ClusterCredentialsSecret).To(Equal(managedEnvSecret.Name))
			Expect(managedEnvCR.Spec.AllowInsecureSkipTLSVerify).To(Equal(dt.Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify))

			By("update the credential secret and verify if the managed-environment secret is updated")
			clusterSecret.Data = map[string][]byte{
				"kubeconfig": []byte("updated"),
			}
			err = k8sClient.Update(ctx, &clusterSecret)
			Expect(err).ToNot(HaveOccurred())

			res, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvSecret), &managedEnvSecret)
			Expect(err).ToNot(HaveOccurred())
			Expect(reflect.DeepEqual(managedEnvSecret.Data, clusterSecret.Data)).To(BeTrue())

			By("delete the credential secret and verify if the managed-environment secret is deleted")
			err = k8sClient.Delete(ctx, &clusterSecret)
			Expect(err).ToNot(HaveOccurred())

			res, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&env), &env); err != nil {
				Expect(err).ToNot(HaveOccurred())
			}
			expectEnvironmentStatusConditionError("the secret test-secret referenced by the Environment resource was not found", EnvironmentReasonSecretNotFound, env)
			Expect(res).To(Equal(reconcile.Result{}))

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvSecret), &managedEnvSecret)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			By("delete environment resource")
			err = k8sClient.Delete(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			res, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			By("verify whether the GitOpsDeploymentManagedEnvironment has been deleted when the Environment resource is deleted.")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("should return and wait if the specified DTC is not in Bounded phase", func() {
			dtc := appstudioshared.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dtc",
					Namespace: apiNamespace.Name,
				},
			}

			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("create an Environment that refer the above DTC")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: dtc.Namespace,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						Claim: appstudiosharedv1beta1.TargetClaim{
							DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile and verify that a ManagedEnvironment is not created")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			managedEnvCR := generateEmptyManagedEnvironment(env.Name, req.Namespace)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("should set an error condition, but not return an error, if the Environment references a DTC that doesn't exist", func() {

			By("create an Environment that refers to a non-existent DTC")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						Claim: appstudiosharedv1beta1.TargetClaim{
							DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
								ClaimName: "a-dtc-that-does-not-exist",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile and verify if an error is returned")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			By("Checking status field after calling Reconciler")
			env = appstudiosharedv1beta1.Environment{}
			err = reconciler.Get(ctx, req.NamespacedName, &env)
			Expect(err).ToNot(HaveOccurred())

			expectEnvironmentStatusConditionError("DeploymentTargetClaim not found while generating the desired Environment resource", EnvironmentReasonDeploymentTargetClaimNotFound, env)

		})

		It("should set an error condition, but not return a Reconcile error, if Environment's DTC has no bound DT", func() {

			// Create a DeploymentTarget that isn't referenced
			unusedDT := appstudioshared.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dt",
					Namespace: apiNamespace.Name,
				},
			}

			err := k8sClient.Create(ctx, &unusedDT)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			By("create and Environment that refer the above DTC")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: dtc.Namespace,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						Claim: appstudiosharedv1beta1.TargetClaim{
							DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile and verify if an error is returned")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			By("Checking status field after calling Reconciler")
			env = appstudiosharedv1beta1.Environment{}
			err = reconciler.Get(ctx, req.NamespacedName, &env)
			Expect(err).ToNot(HaveOccurred())

			expectEnvironmentStatusConditionError("DeploymentTarget not found for DeploymentTargetClaim", EnvironmentReasonDeploymentTargetNotFound, env)
		})

		It("should set an error condition, but not return a Reconcile error, if Environment's DTC references a DeploymentTarget that doesn't exist", func() {

			dtc := appstudioshared.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dtc",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudioshared.DeploymentTargetClaimSpec{
					TargetName: "a-non-existent-dt",
				},
				Status: appstudioshared.DeploymentTargetClaimStatus{
					Phase: appstudioshared.DeploymentTargetClaimPhase_Bound,
				},
			}

			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("create and Environment that refer the above DTC")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: dtc.Namespace,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						Claim: appstudiosharedv1beta1.TargetClaim{
							DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile and verify if an error is returned")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			By("Checking status field after calling Reconciler")
			env = appstudiosharedv1beta1.Environment{}
			err = reconciler.Get(ctx, req.NamespacedName, &env)
			Expect(err).ToNot(HaveOccurred())
			expectEnvironmentStatusConditionError("DeploymentTargetClaim references a DeploymentTarget that does not exist", EnvironmentReasonDeploymentTargetNotFound, env)
		})

		It("shouldn't process the Environment if neither credentials nor DTC is provided", func() {
			By("create an Environment without DTC and credentials")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: apiNamespace.Name,
				},
			}
			err := k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile and verify that a ManagedEnvironment is not created")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			managedEnvCR := generateEmptyManagedEnvironment(env.Name, req.Namespace)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("shouldn't create a new secret if the incoming secret is of type managed-environment", func() {
			By("create a DT and DTC with cluster credentials")
			clusterSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: apiNamespace.Name,
				},
				Type: sharedutil.ManagedEnvironmentSecretType,
			}

			err := k8sClient.Create(ctx, &clusterSecret)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

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
			Expect(err).ToNot(HaveOccurred())

			By("create an Environment that refer the above DTC")
			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-1",
					Namespace: dtc.Namespace,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						Claim: appstudiosharedv1beta1.TargetClaim{
							DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
								ClaimName: dtc.Name,
							},
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile and verify if a ManagedEnvironment is created")
			req := newRequest(env.Namespace, env.Name)
			res, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{}))

			By("verify if a new managed-environment secret is not created")
			managedEnvSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      generateManagedEnvSecretName(env.Name),
					Namespace: env.Namespace,
				},
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvSecret), &managedEnvSecret)
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			By("verify if the ManagedEnvironment is using the incoming secret")

			managedEnvCR := generateEmptyManagedEnvironment(env.Name, req.Namespace)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).ToNot(HaveOccurred())

			By("verify if the environment credentials match with the DT")
			Expect(managedEnvCR.Spec.APIURL).To(Equal(dt.Spec.KubernetesClusterCredentials.APIURL))
			Expect(managedEnvCR.Spec.ClusterCredentialsSecret).To(Equal(clusterSecret.Name))
			Expect(managedEnvCR.Spec.AllowInsecureSkipTLSVerify).To(Equal(dt.Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify))
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
			Expect(err).ToNot(HaveOccurred())

			env := appstudiosharedv1beta1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-env",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1beta1.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1beta1.EnvironmentConfiguration{},
					Target: &appstudiosharedv1beta1.TargetConfiguration{
						KubernetesClusterCredentials: appstudiosharedv1beta1.KubernetesClusterCredentials{
							TargetNamespace:            "my-target-namespace",
							APIURL:                     "https://my-api-url",
							ClusterCredentialsSecret:   secret.Name,
							AllowInsecureSkipTLSVerify: false,
						},
					},
				},
			}
			err = k8sClient.Create(ctx, &env)
			Expect(err).ToNot(HaveOccurred())

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      env.Name,
					Namespace: env.Namespace,
				},
			}
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			managedEnvCR := generateEmptyManagedEnvironment(env.Name, req.Namespace)
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&managedEnvCR), &managedEnvCR)
			Expect(err).ToNot(HaveOccurred(), "the ManagedEnvironment object should have been created by the reconciler")

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
				env1 := appstudiosharedv1beta1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env-1",
						Namespace: dtc.Namespace,
					},
					Spec: appstudiosharedv1beta1.EnvironmentSpec{
						Target: &appstudiosharedv1beta1.TargetConfiguration{
							Claim: appstudiosharedv1beta1.TargetClaim{
								DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}
				err := k8sClient.Create(ctx, &env1)
				Expect(err).ToNot(HaveOccurred())

				env2 := appstudiosharedv1beta1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-env-2",
						Namespace:       dtc.Namespace,
						ResourceVersion: "",
					},
					Spec: appstudiosharedv1beta1.EnvironmentSpec{
						Target: &appstudiosharedv1beta1.TargetConfiguration{
							Claim: appstudiosharedv1beta1.TargetClaim{
								DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}

				err = k8sClient.Create(ctx, &env2)
				Expect(err).ToNot(HaveOccurred())

				By("check if the requests are mapped to the correct environments")
				expectedReqs := map[string]int{
					env1.Name: 1,
					env2.Name: 1,
				}
				reqs := reconciler.findObjectsForDeploymentTargetClaim(&dtc)
				Expect(reqs).To(HaveLen(len(expectedReqs)))
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
				Expect(err).ToNot(HaveOccurred())

				By("create Environments that refer the above DTC")
				env1 := appstudiosharedv1beta1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env-1",
						Namespace: dtc.Namespace,
					},
					Spec: appstudiosharedv1beta1.EnvironmentSpec{
						Target: &appstudiosharedv1beta1.TargetConfiguration{
							Claim: appstudiosharedv1beta1.TargetClaim{
								DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}
				err = k8sClient.Create(ctx, &env1)
				Expect(err).ToNot(HaveOccurred())

				env2 := appstudiosharedv1beta1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-env-2",
						Namespace:       dtc.Namespace,
						ResourceVersion: "",
					},
					Spec: appstudiosharedv1beta1.EnvironmentSpec{
						Target: &appstudiosharedv1beta1.TargetConfiguration{
							Claim: appstudiosharedv1beta1.TargetClaim{
								DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}

				err = k8sClient.Create(ctx, &env2)
				Expect(err).ToNot(HaveOccurred())

				By("check if the requests are mapped to the correct environments")
				expectedReqs := map[string]int{
					env1.Name: 1,
					env2.Name: 1,
				}
				reqs := reconciler.findObjectsForDeploymentTarget(&dt)
				Expect(reqs).To(HaveLen(len(expectedReqs)))
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

		Context("Test findObjectsForSecrets function", func() {
			It("should map requests created by the SpaceRequest controller", func() {
				By("create a credential secret")
				secret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: apiNamespace.Namespace,
					},
					Type: corev1.SecretTypeOpaque,
				}

				err := k8sClient.Create(ctx, &secret)
				Expect(err).ToNot(HaveOccurred())

				By("create a DT and DTC that target each other")
				dt := appstudioshared.DeploymentTarget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dt",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudioshared.DeploymentTargetSpec{
						KubernetesClusterCredentials: appstudioshared.DeploymentTargetKubernetesClusterCredentials{
							ClusterCredentialsSecret: secret.Name,
						},
					},
				}

				err = k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				dtc := appstudioshared.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudioshared.DeploymentTargetClaimSpec{
						TargetName: dt.Name,
					},
				}

				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("create Environments that refer the above DTC")
				env1 := appstudiosharedv1beta1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env-1",
						Namespace: dtc.Namespace,
					},
					Spec: appstudiosharedv1beta1.EnvironmentSpec{
						Target: &appstudiosharedv1beta1.TargetConfiguration{
							Claim: appstudiosharedv1beta1.TargetClaim{
								DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}
				err = k8sClient.Create(ctx, &env1)
				Expect(err).ToNot(HaveOccurred())

				env2 := appstudiosharedv1beta1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-env-2",
						Namespace: dtc.Namespace,
					},
					Spec: appstudiosharedv1beta1.EnvironmentSpec{
						Target: &appstudiosharedv1beta1.TargetConfiguration{
							Claim: appstudiosharedv1beta1.TargetClaim{
								DeploymentTargetClaim: appstudiosharedv1beta1.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}

				err = k8sClient.Create(ctx, &env2)
				Expect(err).ToNot(HaveOccurred())

				By("check if the requests are mapped to the correct environments")
				expectedReqs := map[string]int{
					env1.Name: 1,
					env2.Name: 1,
				}
				reqs := reconciler.findObjectsForSecret(&secret)
				Expect(reqs).To(HaveLen(len(expectedReqs)))
				for _, r := range reqs {
					Expect(expectedReqs[r.Name]).To(Equal(1))
					expectedReqs[r.Name]--
				}
			})

			It("should map requests for managed-environment secrets", func() {
				secret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: apiNamespace.Name,
						Labels: map[string]string{
							managedEnvironmentSecretLabel: "test-env",
						},
					},
					Type: sharedutil.ManagedEnvironmentSecretType,
				}

				reqs := reconciler.findObjectsForSecret(&secret)
				Expect(reqs).To(Equal([]reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      "test-env",
							Namespace: secret.Namespace,
						},
					},
				}))
			})

			It("shouldn't map any requests if no matching object is found", func() {
				secret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: apiNamespace.Name,
					},
				}

				reqs := reconciler.findObjectsForSecret(&secret)
				Expect(reqs).To(Equal([]reconcile.Request{}))
			})

			It("shouldn't map any requests if an incompatible object is passed", func() {
				dtc := appstudioshared.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "target-dtc",
						Namespace: apiNamespace.Name,
					},
				}

				reqs := reconciler.findObjectsForSecret(&dtc)

				Expect(reqs).To(Equal([]reconcile.Request{}))
			})

			It("shouldn't map any requests if the secret is not of the expected type", func() {
				By("create secrets of different types that are not supported")
				secret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret-docker",
						Namespace: apiNamespace.Namespace,
					},
					Type: corev1.SecretTypeDockercfg,
				}

				err := k8sClient.Create(ctx, &secret)
				Expect(err).ToNot(HaveOccurred())

				secretAuth := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret-auth",
						Namespace: apiNamespace.Namespace,
					},
					Type: corev1.SecretTypeBasicAuth,
				}

				err = k8sClient.Create(ctx, &secretAuth)
				Expect(err).ToNot(HaveOccurred())

				reqs := reconciler.findObjectsForSecret(&secret)
				Expect(reqs).To(Equal([]reconcile.Request{}))

				reqs = reconciler.findObjectsForSecret(&secretAuth)
				Expect(reqs).To(Equal([]reconcile.Request{}))
			})
		})
	})

	Context("Unit tests of non-reconcile functions", func() {

		log := log.FromContext(ctx)

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				namespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = appstudioshared.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			err = appstudiosharedv1beta1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			apiNamespace = *namespace

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespace, argocdNamespace, kubesystemNamespace).
				Build()

		})

		DescribeTable("verify updateStatusConditionOfEnvironment works as expected",
			func(preCondition []metav1.Condition, newCondition metav1.Condition, expectedResult []metav1.Condition) {

				env := appstudiosharedv1beta1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-env",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1beta1.EnvironmentSpec{
						DisplayName:        "my-environment",
						DeploymentStrategy: appstudiosharedv1beta1.DeploymentStrategy_Manual,
						ParentEnvironment:  "",
						Tags:               []string{},
						Configuration:      appstudiosharedv1beta1.EnvironmentConfiguration{},
					},
				}
				err := k8sClient.Create(ctx, &env)
				Expect(err).ToNot(HaveOccurred())

				env.Status.Conditions = preCondition
				err = k8sClient.Update(ctx, &env)
				Expect(err).ToNot(HaveOccurred())

				err = updateEnvironmentReconciledStatusCondition(ctx, k8sClient, newCondition.Message, &env,
					newCondition.Status, newCondition.Reason, log)
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&env), &env)
				Expect(err).ToNot(HaveOccurred())

				Expect(env.Status.Conditions).To(HaveLen(2))

				expectedCondition := expectedResult[0]
				actualCondition := env.Status.Conditions[0]
				Expect(actualCondition.Message).To(Equal(expectedCondition.Message))
				Expect(actualCondition.Type).To(Equal(expectedCondition.Type))
				Expect(actualCondition.Reason).To(Equal(expectedCondition.Reason))
				Expect(actualCondition.Status).To(Equal(expectedCondition.Status))

				expectedCondition = expectedResult[1]
				actualCondition = env.Status.Conditions[1]
				Expect(actualCondition.Message).To(Equal(expectedCondition.Message))
				Expect(actualCondition.Type).To(Equal(expectedCondition.Type))
				Expect(actualCondition.Reason).To(Equal(expectedCondition.Reason))
				Expect(actualCondition.Status).To(Equal(expectedCondition.Status))
			},
			Entry("add a new condition",
				[]metav1.Condition{},
				metav1.Condition{
					Status:  metav1.ConditionTrue,
					Reason:  "my-reason",
					Message: "my-message",
				},
				[]metav1.Condition{
					{
						Type:    EnvironmentConditionErrorOccurred,
						Status:  metav1.ConditionFalse,
						Reason:  EnvironmentReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    EnvironmentConditionReconciled,
						Status:  metav1.ConditionTrue,
						Reason:  "my-reason",
						Message: "my-message",
					},
				},
			),
			Entry("replace an existing condition with mismatched reason",
				[]metav1.Condition{
					{
						Type:    EnvironmentConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  "my-reason",
						Message: "my-message",
					},
					{
						Type:    EnvironmentConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason",
						Message: "my-message",
					},
				},
				metav1.Condition{
					Status:  metav1.ConditionFalse,
					Reason:  "my-reason2",
					Message: "my-message",
				},
				[]metav1.Condition{
					{
						Type:    EnvironmentConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  EnvironmentReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    EnvironmentConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason2",
						Message: "my-message",
					},
				},
			),
			Entry("replace an existing condition with mismatched message",
				[]metav1.Condition{
					{
						Type:    EnvironmentConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  EnvironmentReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    EnvironmentConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason",
						Message: "my-message",
					},
				},
				metav1.Condition{
					Status:  metav1.ConditionFalse,
					Reason:  "my-reason",
					Message: "my-message2",
				},
				[]metav1.Condition{
					{
						Type:    EnvironmentConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  EnvironmentReasonErrorOccurred,
						Message: "my-message2",
					},
					{
						Type:    EnvironmentConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason",
						Message: "my-message2",
					},
				},
			),
			Entry("replace an existing condition with mismatched status",
				[]metav1.Condition{
					{
						Type:    EnvironmentConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  EnvironmentReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    EnvironmentConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason",
						Message: "my-message",
					},
				},
				metav1.Condition{
					Status:  metav1.ConditionTrue,
					Reason:  "my-reason",
					Message: "my-message",
				},
				[]metav1.Condition{
					{
						Type:    EnvironmentConditionErrorOccurred,
						Status:  metav1.ConditionFalse,
						Reason:  EnvironmentReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    EnvironmentConditionReconciled,
						Status:  metav1.ConditionTrue,
						Reason:  "my-reason",
						Message: "my-message",
					},
				},
			),
		)

	})

	Context("test findObjectsForGitOpsDeploymentManagedEnvironment", func() {

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				namespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = appstudioshared.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			err = appstudiosharedv1beta1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

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

		DescribeTable("Various tests of inputs to function",
			func(managedEnv managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment, expected []reconcile.Request) {

				// The elements in the output of the function should match the elements in 'expected', regardless of order
				res := reconciler.findObjectsForGitOpsDeploymentManagedEnvironment(&managedEnv)
				Expect(res).To(ConsistOf(expected))

			}, Entry("managedenvironment with no owner refs should return no results", managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "managed-env", Namespace: apiNamespace.Name,
					OwnerReferences: []metav1.OwnerReference{},
				},
			}, []reconcile.Request{}),
			Entry("managedenvironment with owner ref to another kind should return no results", managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "managed-env", Namespace: apiNamespace.Name,
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "Environment",
							Name:       "name",
							APIVersion: "SomeOtherAPIVersion",
						},
					},
				},
			}, []reconcile.Request{}),
			Entry("managedenvironment with ownerref to Environment should return a result", managedgitopsv1alpha1.GitOpsDeploymentManagedEnvironment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "managed-env", Namespace: apiNamespace.Name,
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "Environment",
							Name:       "name",
							APIVersion: managedgitopsv1alpha1.GroupVersion.Group + "/" + managedgitopsv1alpha1.GroupVersion.Version,
						},
					},
				},
			}, []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: apiNamespace.Name, Name: "name"}}}),
		)

	})
})

// expectEnvironmentStatusConditionError verifies that both a Reconciled and an EnvironmentConditionErrorOccurred
// status conditions are set, with the appropriate message, and in the case of the Reconciled condition, with the
// appropriate reason.
func expectEnvironmentStatusConditionError(envMessage, envReason string, env appstudiosharedv1beta1.Environment) {

	// WithOffset tells Gingko to ignore this function when reporting the failing line in the test

	ExpectWithOffset(1, env.Status.Conditions).To(HaveLen(2), "two conditions should exist")

	ExpectWithOffset(1, env.Status.Conditions[0].Type).To(Equal(EnvironmentConditionErrorOccurred), "type should be EnvironmentConditionErrorOccurred")
	ExpectWithOffset(1, env.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue), "should be true")
	ExpectWithOffset(1, env.Status.Conditions[0].Reason).To(Equal(EnvironmentReasonErrorOccurred), "reason should be EnvironmentConditionErrorOccurred")

	fmt.Println("The Environment's condition message is", env.Status.Conditions[0].Message)
	ExpectWithOffset(1, env.Status.Conditions[0].Message).To(Equal(envMessage), "message should be as provided")

	ExpectWithOffset(1, env.Status.Conditions[1].Type).To(Equal(EnvironmentConditionReconciled), "type should be EnvironmentConditionErrorOccurred")
	ExpectWithOffset(1, env.Status.Conditions[1].Status).To(Equal(metav1.ConditionFalse), "should be false")
	ExpectWithOffset(1, env.Status.Conditions[1].Reason).To(Equal(envReason), "reason should be "+envReason)

	fmt.Println("The Environment's condition message is", env.Status.Conditions[0].Message)
	ExpectWithOffset(1, env.Status.Conditions[1].Message).To(Equal(envMessage), "message should be as provided")

}
