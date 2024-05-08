package appstudioredhatcom

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"

	appstudiosharedv1 "github.com/github.com/konflux-ci/application-api/api/v1alpha1"
	apibackend "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
)

var _ = Describe("SnapshotEnvironmentBinding Reconciler Tests", func() {

	threeReplicas := 3

	Context("Testing SnapshotEnvironmentBindingReconciler.", func() {
		var ctx context.Context
		var request reconcile.Request
		var binding *appstudiosharedv1.SnapshotEnvironmentBinding
		var bindingReconciler SnapshotEnvironmentBindingReconciler

		var environment appstudiosharedv1.Environment
		var application appstudiosharedv1.Application

		BeforeEach(func() {
			ctx = context.Background()

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			// Create fake client
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			// Create placeholder environment
			environment = appstudiosharedv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "staging",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1.EnvironmentConfiguration{},
				},
			}
			err = k8sClient.Create(ctx, &environment)
			Expect(err).ToNot(HaveOccurred())

			// Create placeholder application
			application = appstudiosharedv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-demo-app",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1.ApplicationSpec{
					DisplayName: "my-application",
				},
			}
			err = k8sClient.Create(ctx, &application)
			Expect(err).ToNot(HaveOccurred())

			bindingReconciler = SnapshotEnvironmentBindingReconciler{Client: k8sClient, Scheme: scheme}

			// Create SnapshotEnvironmentBinding CR.
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: apiNamespace.Name,
					Labels: map[string]string{
						"appstudio.application": "new-demo-app",
						"appstudio.environment": "staging",
					},
				},
				Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
					Application: "new-demo-app",
					Environment: "staging",
					Snapshot:    "my-snapshot",
					Components: []appstudiosharedv1.BindingComponent{
						{
							Name: "component-a",
							Configuration: appstudiosharedv1.BindingComponentConfiguration{
								Env: []appstudiosharedv1.EnvVarPair{
									{Name: "My_STG_ENV", Value: "1000"},
								},
								Replicas: &threeReplicas,
							},
						},
					},
				},
				Status: appstudiosharedv1.SnapshotEnvironmentBindingStatus{
					Components: []appstudiosharedv1.BindingComponentStatus{
						{
							Name: "component-a",
							GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
								URL:    "https://github.com/redhat-appstudio/managed-gitops",
								Branch: "main",
								Path:   "resources/test-data/sample-gitops-repository/components/componentA/overlays/staging",
							},
						},
					},
				},
			}

			// Create request object for Reconciler
			request = newRequest(apiNamespace.Name, binding.Name)
		})

		It("should delete the SnapshotEnvironmentBinding if the Application doesn't exist and X minutes have passed", func() {
			// Create the Binding
			binding.Spec.Application = "no-such-app"
			binding.CreationTimestamp = metav1.NewTime(time.Now().Add(-1 * allowDeletionOfOrphanedSnapshotEnvironmentBindingAfterXMinutes))
			Expect(bindingReconciler.Create(ctx, binding)).To(Succeed())

			// Call the Reconciler
			_, err := bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			// Check that the binding has been deleted
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, binding)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("should not delete the SnapshotEnvironmentBinding if the Application doesn't exist and fewer than X minutes have passed", func() {
			// Create the Binding
			binding.Spec.Application = "no-such-app"
			binding.CreationTimestamp = metav1.NewTime(time.Now())
			Expect(bindingReconciler.Create(ctx, binding)).To(Succeed())

			// Call the Reconciler
			requeue, err := bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			Expect(requeue.RequeueAfter).To(Equal(allowDeletionOfOrphanedSnapshotEnvironmentBindingAfterXMinutes))

			// Check that the binding has not been deleted
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, binding)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should set the .status.GitOpsDeployments field of Binding.", func() {
			// Create SnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Check status field before calling Reconciler
			bindingFirst := &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, bindingFirst)
			Expect(err).ToNot(HaveOccurred())
			Expect(bindingFirst.Status.GitOpsDeployments).To(BeEmpty())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			// Check status field after calling Reconciler
			bindingSecond := &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, bindingSecond)

			Expect(err).ToNot(HaveOccurred())
			Expect(bindingSecond.Status.GitOpsDeployments).NotTo(BeEmpty())
			Expect(bindingSecond.Status.GitOpsDeployments[0].ComponentName).To(Equal("component-a"))
			Expect(bindingSecond.Status.GitOpsDeployments[0].GitOpsDeployment).
				To(Equal(binding.Name + "-" +
					binding.Spec.Application + "-" +
					binding.Spec.Environment + "-" +
					binding.Spec.Components[0].Name))
		})

		It("Should not update GitOpsDeployment if same Binding is created again.", func() {

			// Create SnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			// Fetch GitOpsDeployment object before calling Reconciler
			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}

			gitopsDeploymentFirst := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeploymentFirst)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler again
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			// Fetch GitOpsDeployment object after calling Reconciler
			gitopsDeploymentSecond := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeploymentSecond)
			Expect(err).ToNot(HaveOccurred())

			// Reconciler should not do any change in GitOpsDeployment object.
			Expect(gitopsDeploymentFirst).To(Equal(gitopsDeploymentSecond))
		})

		It("Should revert GitOpsDeploymentObject if it's spec is different than Binding Component.", func() {
			// Create SnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}

			// Fetch GitOpsDeployment object
			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			// GitOpsDeployment object spec should be same as Binding Component.
			Expect(gitopsDeployment.Spec.Source.Path).To(Equal(binding.Status.Components[0].GitOpsRepository.Path))

			// Update GitOpsDeploymentObject in cluster.
			gitopsDeployment.Spec.Source.Path = "components/componentA/overlays/dev"
			err = bindingReconciler.Update(ctx, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler again
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			// Fetch GitOpsDeployment object after calling Reconciler
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			// Reconciler should revert GitOpsDeployment object, so it will be same as old object
			Expect(gitopsDeployment.Spec.Source.Path).To(Equal(binding.Status.Components[0].GitOpsRepository.Path))
		})

		It("Should remove a GitOpsDeployment when the corresponding component is removed from the binding.", func() {

			By("Creating a Binding with two components")
			binding.Spec.Components = []appstudiosharedv1.BindingComponent{
				{
					Name: "component-a",
					Configuration: appstudiosharedv1.BindingComponentConfiguration{
						Env: []appstudiosharedv1.EnvVarPair{
							{Name: "My_STG_ENV", Value: "1000"},
						},
						Replicas: &threeReplicas,
					},
				},
				{
					Name: "component-b",
					Configuration: appstudiosharedv1.BindingComponentConfiguration{
						Env: []appstudiosharedv1.EnvVarPair{
							{Name: "My_STG_ENV", Value: "1000"},
						},
						Replicas: &threeReplicas,
					},
				},
			}
			binding.Status.Components = []appstudiosharedv1.BindingComponentStatus{
				{
					Name: "component-a",
					GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
						URL:    "https://github.com/redhat-appstudio/managed-gitops",
						Branch: "main",
						Path:   "resources/test-data/sample-gitops-repository/components/componentA/overlays/staging",
					},
				},
				{
					Name: "component-b",
					GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
						URL:    "https://github.com/redhat-appstudio/managed-gitops",
						Branch: "main",
						Path:   "resources/test-data/sample-gitops-repository/components/componentB/overlays/staging",
					},
				},
			}
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			err = bindingReconciler.Get(context.Background(), client.ObjectKeyFromObject(binding), binding)
			Expect(err).ToNot(HaveOccurred())

			Expect(binding.Status.Components).ToNot(BeEmpty())

			By("Also creating an unrelated GitOpsDeployment which should not be removed")
			unrelatedDeployment := apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unrelated-deployment",
					Namespace: binding.Namespace,
					Labels: map[string]string{
						applicationLabelKey: "unrelated-application",
						environmentLabelKey: "unrelated-environment",
						componentLabelKey:   "unrelated-component",
					},
				},
			}
			err = bindingReconciler.Create(ctx, &unrelatedDeployment)
			Expect(err).ToNot(HaveOccurred())

			By("Triggering the Reconciler to create the GitOpsDeployment instance")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("Ensuring the two corresponding GitOpsDeployment instances have been created")
			gitopsDeploymentKey0 := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}
			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey0, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			gitopsDeploymentKey1 := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[1].Name),
			}
			gitopsDeployment = &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey1, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			By("Ensuring the unrelated deployment still exists")
			unrelatedDeploymentKey := client.ObjectKey{
				Namespace: unrelatedDeployment.Namespace,
				Name:      unrelatedDeployment.Name,
			}
			gitopsDeployment = &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, unrelatedDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			By("Removing the first component and reconciling")
			err = bindingReconciler.Get(ctx, client.ObjectKeyFromObject(binding), binding)
			Expect(err).ToNot(HaveOccurred())
			binding.Spec.Components = binding.Spec.Components[1:]
			err = bindingReconciler.Update(ctx, binding)
			Expect(err).ToNot(HaveOccurred())
			binding.Status.Components = binding.Status.Components[1:]
			err = bindingReconciler.Status().Update(ctx, binding)
			Expect(err).ToNot(HaveOccurred())
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("Ensuring the deployment associated with the first component has been deleted")
			gitopsDeployment = &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey0, gitopsDeployment)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			By("Ensuring the deployment associated with the second component still exists")
			gitopsDeployment = &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey1, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			By("Ensuring the unrelated deployment still exists")
			gitopsDeployment = &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, unrelatedDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			By("Deleting the other component and reconciling, so there are no components associated with the binding")
			err = bindingReconciler.Get(ctx, client.ObjectKeyFromObject(binding), binding)
			Expect(err).ToNot(HaveOccurred())
			binding.Spec.Components = []appstudiosharedv1.BindingComponent{}
			err = bindingReconciler.Update(ctx, binding)
			Expect(err).ToNot(HaveOccurred())
			binding.Status.Components = []appstudiosharedv1.BindingComponentStatus{}
			err = bindingReconciler.Status().Update(ctx, binding)
			Expect(err).ToNot(HaveOccurred())
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("Ensuring the deployment associated with the first component is still deleted")
			gitopsDeployment = &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey0, gitopsDeployment)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			By("Ensuring the deployment associated with the second component has been deleted")
			gitopsDeployment = &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey1, gitopsDeployment)
			Expect(err).To(HaveOccurred())
			Expect(apierr.IsNotFound(err)).To(BeTrue())

			By("Ensuring the unrelated deployment still exists")
			gitopsDeployment = &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, unrelatedDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should use short name for GitOpsDeployment object.", func() {
			// Update the names to exceed the limit
			// The application name, environment name and component name are each limited to be at most 63 characters.
			// The GitOpsDeployment name is formed from
			// binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + componentName
			environment = appstudiosharedv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.Repeat("e", 63),
					Namespace: binding.Namespace,
				},
				Spec: appstudiosharedv1.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1.EnvironmentConfiguration{},
				},
			}
			err := bindingReconciler.Create(ctx, &environment)
			Expect(err).ToNot(HaveOccurred())
			binding.Spec.Environment = environment.Name
			binding.ObjectMeta.Labels["appstudio.environment"] = environment.Name
			application = appstudiosharedv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.Repeat("a", 63),
					Namespace: binding.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSpec{
					DisplayName: "my-application",
				},
			}
			err = bindingReconciler.Create(ctx, &application)
			Expect(err).ToNot(HaveOccurred())
			binding.Spec.Application = application.Name
			binding.Name = strings.Repeat("b", 111)
			request = newRequest(binding.Namespace, binding.Name)

			// Create SnapshotEnvironmentBinding CR in cluster.
			err = bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			// Check status field after calling Reconciler
			binding := &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, binding)

			Expect(err).ToNot(HaveOccurred())
			Expect(binding.Status.GitOpsDeployments).NotTo(BeEmpty())
			Expect(binding.Status.GitOpsDeployments[0].ComponentName).To(Equal("component-a"))

			// GitOpsDeployment should have short name
			Expect(binding.Status.GitOpsDeployments[0].GitOpsDeployment).
				To(Equal(binding.Name + "-" + binding.Spec.Components[0].Name))
		})

		It("Should use short name with hash value for GitOpsDeployment, if combination of Binding name and Component name is still longer than 250 characters.", func() {

			// Update the names to exceed the limit
			// The application name, environment name and component name are each limited to be at most 63 characters.
			// The GitOpsDeployment name is formed from
			// binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + componentName
			binding.Name = strings.Repeat("abcd", 45) + "1234567" // length = 187
			application = appstudiosharedv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.Repeat("a", 63),
					Namespace: binding.Namespace,
				},
				Spec: appstudiosharedv1.ApplicationSpec{
					DisplayName: "my-application",
				},
			}
			err := bindingReconciler.Create(ctx, &application)
			Expect(err).ToNot(HaveOccurred())
			binding.Spec.Application = application.Name
			compName := strings.Repeat("c", 63)
			binding.Status.Components[0].Name = compName
			request = newRequest(binding.Namespace, binding.Name)

			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err = bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			// Check status field after calling Reconciler
			binding := &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, binding)

			Expect(err).ToNot(HaveOccurred())
			Expect(binding.Status.GitOpsDeployments).NotTo(BeEmpty())

			// 180 (First 180 characters of combination of Binding name and Component name) + 1 ("-")+64 (length of hast value) = 245 (Total length)
			Expect(binding.Status.GitOpsDeployments[0].GitOpsDeployment).To(HaveLen(245))

			// Get the short name with hash value.
			hashValue := sha256.Sum256([]byte(binding.Name + "-" + binding.Spec.Application + "-" + binding.Spec.Environment + "-" + compName))
			hashString := fmt.Sprintf("%x", hashValue)
			expectedName := (binding.Name + "-" + compName)[0:180] + "-" + hashString

			Expect(binding.Status.GitOpsDeployments[0].GitOpsDeployment).
				To(Equal(expectedName))
		})

		It("Should not return error if Status.Components is not available in Binding object.", func() {
			binding.Status.Components = []appstudiosharedv1.BindingComponentStatus{}
			// Create SnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			checkStatusConditionOfEnvironmentBinding(ctx, bindingReconciler.Client, binding, "SnapshotEventBinding Component status is required to generate GitOps deployment, waiting for the Application Service controller to finish reconciling binding 'appa-staging-binding'", metav1.ConditionFalse, SnapshotEnvironmentBindingReasonWaitingForComponentStatus)

		})

		It("Should return error if Status.GitOpsRepoConditions Status is set to False in Binding object.", func() {
			binding.Status.GitOpsRepoConditions = []metav1.Condition{
				{
					Status: metav1.ConditionFalse,
				},
			}

			// Create SnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			checkStatusConditionOfEnvironmentBinding(ctx, bindingReconciler.Client, binding, "Cannot Reconcile Binding 'appa-staging-binding', since GitOps Repo Conditions status is false.", metav1.ConditionFalse, SnapshotEnvironmentBindingReasonGitOpsRepoNotReady)
		})

		It("should update the previous binding condition status if there is no longer a repo condition error", func() {
			By("set a repo condition to false and verify if the binding condition is set")
			binding.Status.GitOpsRepoConditions = []metav1.Condition{
				{
					Status: metav1.ConditionFalse,
				},
			}

			// Create SnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			checkStatusConditionOfEnvironmentBinding(ctx, bindingReconciler.Client, binding, "Cannot Reconcile Binding 'appa-staging-binding', since GitOps Repo Conditions status is false.", metav1.ConditionFalse, SnapshotEnvironmentBindingReasonGitOpsRepoNotReady)

			By("update the repo condition and verify if the binding condition is updated")
			binding.Status.GitOpsRepoConditions = []metav1.Condition{
				{
					Status: metav1.ConditionTrue,
				},
			}
			err = bindingReconciler.Update(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			err = bindingReconciler.Get(ctx, client.ObjectKeyFromObject(binding), binding)
			Expect(err).ToNot(HaveOccurred())
			checkStatusConditionOfEnvironmentBinding(ctx, bindingReconciler.Client, binding, "", metav1.ConditionTrue, SnapshotEnvironmentBindingReasonReconciled)
		})

		It("should not return an error if there are duplicate components in binding.Status.Components", func() {

			By("creating an SnapshotEnvironmentBinding with duplicate component names")

			binding.Status.Components = []appstudiosharedv1.BindingComponentStatus{
				{
					Name: "componentA",
					GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
						URL:                "https://url",
						Branch:             "branch",
						Path:               "path",
						GeneratedResources: []string{},
					},
				},
				{
					Name: "componentA",
					GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
						URL:                "https://url2",
						Branch:             "branch2",
						Path:               "path2",
						GeneratedResources: []string{},
					},
				},
			}

			// Create SnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			checkStatusConditionOfEnvironmentBinding(ctx, bindingReconciler.Client, binding, "duplicate component keys found in status field in componentA", metav1.ConditionFalse, SnapshotEnvironmentBindingReasonDuplicateComponents)

		})

		It("should verify that if the Environment references a DeploymentTargetClaim, and the DT references a Namespace, that the Namespace is included in the generated GitOpsDeployment", func() {

			dtc := appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-claim",
					Namespace: environment.Namespace,
					Annotations: map[string]string{
						appstudiosharedv1.AnnTargetProvisioner: "provisioner-name",
					},
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: "some-class",
				},
				Status: appstudiosharedv1.DeploymentTargetClaimStatus{
					Phase: appstudiosharedv1.DeploymentTargetClaimPhase(appstudiosharedv1.DeploymentTargetPhase_Available),
				},
			}
			err := bindingReconciler.Client.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			dt := appstudiosharedv1.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-target",
					Namespace: environment.Namespace,
				},
				Spec: appstudiosharedv1.DeploymentTargetSpec{
					DeploymentTargetClassName: dtc.Spec.DeploymentTargetClassName,
					ClaimRef:                  dtc.Name,
					KubernetesClusterCredentials: appstudiosharedv1.DeploymentTargetKubernetesClusterCredentials{
						DefaultNamespace:         "default-namespace-from-dt",
						APIURL:                   "https://fake-url",
						ClusterCredentialsSecret: "fake-secret-name",
					},
				},
			}
			err = bindingReconciler.Client.Create(ctx, &dt)
			Expect(err).ToNot(HaveOccurred())

			By("creating an Environment that references the DTC")
			environment.Spec.UnstableConfigurationFields = nil
			environment.Spec.Configuration.Target = appstudiosharedv1.EnvironmentTarget{
				DeploymentTargetClaim: appstudiosharedv1.DeploymentTargetClaimConfig{
					ClaimName: dtc.Name,
				},
			}
			err = bindingReconciler.Client.Update(ctx, &environment)
			Expect(err).ToNot(HaveOccurred())

			By("creating default Binding")
			err = bindingReconciler.Client.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("calling Reconcile on the SnapshotEnvironmentBinding")
			request = newRequest(binding.Namespace, binding.Name)
			_, _ = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("ensuring that the GitOpsDeployment was created using values from DeploymentTarget")

			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}
			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironmentName := generateEmptyManagedEnvironment(environment.Name, environment.Namespace).Name

			Expect(gitopsDeployment.Spec.Destination.Namespace).To(Equal(dt.Spec.KubernetesClusterCredentials.DefaultNamespace))
			Expect(gitopsDeployment.Spec.Destination.Environment).To(Equal(managedEnvironmentName))

		})

		It("should verify that if DeploymentTargetClaim is not found referencing to an the Environment", func() {
			By("creating an Environment that references the DTC")
			environment.Spec.UnstableConfigurationFields = nil
			environment.Spec.Configuration.Target = appstudiosharedv1.EnvironmentTarget{
				DeploymentTargetClaim: appstudiosharedv1.DeploymentTargetClaimConfig{
					ClaimName: "dtc-that-doesnt-exist",
				},
			}
			err := bindingReconciler.Client.Update(ctx, &environment)
			Expect(err).ToNot(HaveOccurred())

			By("creating default Binding")
			err = bindingReconciler.Client.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("calling Reconcile on the SnapshotEnvironmentBinding")
			request = newRequest(binding.Namespace, binding.Name)
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error on retrieving DeploymentTargetClaim"))

		})

		It("should verify that if the Environment contains configuration information, that it is included in the generated GitOpsDeployment", func() {

			By("creating an Environment with valid configuration fields")
			environment.Spec.UnstableConfigurationFields = &appstudiosharedv1.UnstableEnvironmentConfiguration{
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					TargetNamespace:          "my-target-namespace",
					APIURL:                   "my-api-url",
					ClusterCredentialsSecret: "secret",
				},
			}
			err := bindingReconciler.Client.Update(ctx, &environment)
			Expect(err).ToNot(HaveOccurred())

			By("creating default Binding")
			err = bindingReconciler.Client.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("calling Reconcile on the SnapshotEnvironmentBinding")
			request = newRequest(binding.Namespace, binding.Name)
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("ensuring that the GitOpsDeployment was created using values from ConfigurationFields")

			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}
			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			managedEnvironmentName := generateEmptyManagedEnvironment(environment.Name, environment.Namespace).Name

			Expect(gitopsDeployment.Spec.Destination.Namespace).To(Equal(environment.Spec.UnstableConfigurationFields.TargetNamespace))
			Expect(gitopsDeployment.Spec.Destination.Environment).To(Equal(managedEnvironmentName))

			By("removing the field from Environment, and ensuring the GitOpsDeployment is updated")
			environment.Spec.UnstableConfigurationFields = nil
			err = bindingReconciler.Client.Update(ctx, &environment)
			Expect(err).ToNot(HaveOccurred())

			By("reconciling again")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.Spec.Destination.Namespace).To(Equal(""))
			Expect(gitopsDeployment.Spec.Destination.Environment).To(Equal(""))

			By("testing with a missing TargetNamespace, which should return an error")
			environment.Spec.UnstableConfigurationFields = &appstudiosharedv1.UnstableEnvironmentConfiguration{
				KubernetesClusterCredentials: appstudiosharedv1.KubernetesClusterCredentials{
					APIURL:                   "my-api-url",
					ClusterCredentialsSecret: "secret",
				},
			}
			err = bindingReconciler.Client.Update(ctx, &environment)
			Expect(err).ToNot(HaveOccurred())

			By("reconciling again, and expecting an TargetNamespace missing error")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(HaveOccurred())
			Expect(strings.Contains(err.Error(), errMissingTargetNamespace)).To(BeTrue())

		})

		It("should append ASEB label with key `appstudio.openshift.io` into the GitopsDeployment Label", func() {
			By("updating binding.ObjectMeta.Labels with appstudio.openshift.io label")
			binding.ObjectMeta.Labels[appstudioLabelKey] = "testing"

			By("creating SnapshotEnvironmentBinding CR in cluster.")
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("triggering Reconciler")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("Fetching GitOpsDeployment object to check whether GitOpsDeployment label field has been updated")
			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}

			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels).ToNot(BeNil())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(Equal(map[string]string{
				appstudioLabelKey:   "testing",
				applicationLabelKey: binding.Spec.Application,
				componentLabelKey:   binding.Spec.Components[0].Name,
				environmentLabelKey: binding.Spec.Environment,
			}))
		})

		It("should not append ASEB label without key appstudio.openshift.io into the GitopsDeployment Label, but it should add labels identifying the application, environment and component", func() {
			By("creating SnapshotEnvironmentBinding CR in cluster.")
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("triggering Reconciler")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("fetching GitOpsDeployment object to check whether GitOpsDeployment label field is not updated")
			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}

			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			Expect(gitopsDeployment.ObjectMeta.Labels).To(Equal(map[string]string{
				applicationLabelKey: binding.Spec.Application,
				componentLabelKey:   binding.Spec.Components[0].Name,
				environmentLabelKey: binding.Spec.Environment,
			}))
		})

		It("should update gitopsDeployment label if ASEB label gets updated", func() {
			By("updating binding.ObjectMeta.Labels with appstudio.openshift.io label")
			binding.ObjectMeta.Labels[appstudioLabelKey] = "testing"

			By("creating SnapshotEnvironmentBinding CR in cluster.")
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("triggering Reconciler")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("fetching GitOpsDeployment object to check whether GitOpsDeployment label field has been updated")
			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}

			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels).ToNot(BeNil())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(Equal(map[string]string{
				appstudioLabelKey:   "testing",
				applicationLabelKey: binding.Spec.Application,
				componentLabelKey:   binding.Spec.Components[0].Name,
				environmentLabelKey: binding.Spec.Environment,
			}))

			err = bindingReconciler.Get(ctx, types.NamespacedName{Namespace: binding.Namespace, Name: binding.Name}, binding)
			Expect(err).To(Succeed())

			By("updating appstudio.openshift.io label")
			binding.ObjectMeta.Labels[appstudioLabelKey] = "testing-update"

			err = bindingReconciler.Update(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("triggering Reconciler")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("verifying GitopsDeployment is updated as binding CR is updated")
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels).ToNot(BeNil())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(Equal(map[string]string{
				appstudioLabelKey:   "testing-update",
				applicationLabelKey: binding.Spec.Application,
				componentLabelKey:   binding.Spec.Components[0].Name,
				environmentLabelKey: binding.Spec.Environment,
			}))

			err = bindingReconciler.Get(ctx, types.NamespacedName{Namespace: binding.Namespace, Name: binding.Name}, binding)
			Expect(err).To(Succeed())

			By("removing ASEB label `appstudio.openshift.io` label and verify whether it is removed from gitopsDeployment label")
			delete(binding.ObjectMeta.Labels, appstudioLabelKey)

			err = bindingReconciler.Update(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("triggering Reconciler")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("verifying whether gitopsDeployment.ObjectMeta.Label `appstudio.openshift.io` is removed from gitopsDeployment")
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels).ToNot(BeEmpty())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(Equal(map[string]string{
				applicationLabelKey: binding.Spec.Application,
				componentLabelKey:   binding.Spec.Components[0].Name,
				environmentLabelKey: binding.Spec.Environment,
			}))

		})

		It("should update gitopsDeployment label if ASEB label gets updated, but not affect non-appstudio labels", func() {
			By("updating binding.ObjectMeta.Labels with appstudio.openshift.io label, plus a non-appstudio-label")
			binding.ObjectMeta.Labels[appstudioLabelKey] = "testing"
			binding.ObjectMeta.Labels["non-appstudio-label"] = "should-not-be-copied"

			By("creating SnapshotEnvironmentBinding")
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("triggering Reconciler")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("fetching GitOpsDeployment object to check whether GitOpsDeployment label field has been updated")
			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}

			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels).ToNot(BeNil())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(Equal(map[string]string{
				appstudioLabelKey:   "testing",
				applicationLabelKey: binding.Spec.Application,
				componentLabelKey:   binding.Spec.Components[0].Name,
				environmentLabelKey: binding.Spec.Environment,
			}), "reconciler should only copy appstudio labels to the gitops deployment")

			err = bindingReconciler.Get(ctx, types.NamespacedName{Namespace: binding.Namespace, Name: binding.Name}, binding)
			Expect(err).To(Succeed())

			By("updating appstudio.openshift.io label")
			binding.ObjectMeta.Labels[appstudioLabelKey] = "testing-update"

			err = bindingReconciler.Update(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("triggering Reconciler")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("verifying GitopsDeployment is updated as binding CR is updated, and the non-appstudio value is unchanged")
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels).ToNot(BeNil())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(Equal(map[string]string{
				appstudioLabelKey:   "testing-update",
				applicationLabelKey: binding.Spec.Application,
				componentLabelKey:   binding.Spec.Components[0].Name,
				environmentLabelKey: binding.Spec.Environment,
			}))

			err = bindingReconciler.Get(ctx, types.NamespacedName{Namespace: binding.Namespace, Name: binding.Name}, binding)
			Expect(err).To(Succeed())

			By("removing ASEB label `appstudio.openshift.io` label and verifying whether it is removed from gitopsDeployment label")
			delete(binding.ObjectMeta.Labels, appstudioLabelKey)

			err = bindingReconciler.Update(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("triggering Reconciler")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("verifying whether gitopsDeployment.ObjectMeta.Label `appstudio.openshift.io` is removed from " +
				"gitopsDeployment, but the non-appstudio label is still present")
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(Equal(map[string]string{
				applicationLabelKey: binding.Spec.Application,
				componentLabelKey:   binding.Spec.Components[0].Name,
				environmentLabelKey: binding.Spec.Environment,
			}))

		})

		It("should add labels identifying the application, environment and component to an existing GitOpsDeployment's labels", func() {
			By("creating the GitOpsDeployment by creating a SnapshotEnvironmentBinding CR and reconciling")
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name:      GenerateBindingGitOpsDeploymentName(*binding, binding.Spec.Components[0].Name),
			}
			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels).ToNot(BeEmpty())

			By("removing the labels from the GitOpsDeployment resource")
			delete(gitopsDeployment.ObjectMeta.Labels, applicationLabelKey)
			delete(gitopsDeployment.ObjectMeta.Labels, componentLabelKey)
			delete(gitopsDeployment.ObjectMeta.Labels, environmentLabelKey)
			err = bindingReconciler.Update(ctx, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())

			By("check that the labels have really been removed")
			gitopsDeployment = &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(BeEmpty())

			By("triggering Reconciler")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("fetching the GitOpsDeployment object to check if the GitOpsDeployment label field has been updated correctly")
			gitopsDeployment = &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(gitopsDeployment.ObjectMeta.Labels).ToNot(BeEmpty())
			Expect(gitopsDeployment.ObjectMeta.Labels).To(Equal(map[string]string{
				applicationLabelKey: binding.Spec.Application,
				componentLabelKey:   binding.Spec.Components[0].Name,
				environmentLabelKey: binding.Spec.Environment,
			}))
		})

		When("the SnapshotEnvironmentBinding contains components out of lexical order", func() {
			It("should sort the .status.gitOpsDeployments field into proper lexical order", func() {

				binding.Spec.Components = []appstudiosharedv1.BindingComponent{
					{
						Name: "component-b",
						Configuration: appstudiosharedv1.BindingComponentConfiguration{
							Env: []appstudiosharedv1.EnvVarPair{
								{Name: "My_STG_ENV", Value: "1000"},
							},
							Replicas: &threeReplicas,
						},
					},
					{
						Name: "component-a",
						Configuration: appstudiosharedv1.BindingComponentConfiguration{
							Env: []appstudiosharedv1.EnvVarPair{
								{Name: "My_STG_ENV", Value: "1000"},
							},
							Replicas: &threeReplicas,
						},
					},
				}

				binding.Status = appstudiosharedv1.SnapshotEnvironmentBindingStatus{
					Components: []appstudiosharedv1.BindingComponentStatus{
						{
							Name: "component-b",
							GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
								URL:    "https://github.com/redhat-appstudio/managed-gitops",
								Branch: "main",
								Path:   "resources/test-data/sample-gitops-repository/components/componentB/overlays/staging",
							},
						},
						{
							Name: "component-a",
							GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
								URL:    "https://github.com/redhat-appstudio/managed-gitops",
								Branch: "main",
								Path:   "resources/test-data/sample-gitops-repository/components/componentA/overlays/staging",
							},
						},
					},
				}

				By("creating a SnapshotEnvironmentBinding with components out of lexical order")
				Expect(bindingReconciler.Create(ctx, binding)).To(Succeed())

				By("triggering Reconciler on that SEB")
				Expect(bindingReconciler.Reconcile(ctx, request)).Error().To(Succeed())

				By("verifying the .status.GitopsDeployments is in lexical order")
				Expect(bindingReconciler.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

				Expect(binding.Status.GitOpsDeployments).To(HaveLen(2))
				Expect(binding.Status.GitOpsDeployments[0].ComponentName).To(Equal("component-a"))
				Expect(binding.Status.GitOpsDeployments[1].ComponentName).To(Equal("component-b"))

			})
		})

	})

	Context("Test SnapshotEnvironmentBinding's findObjectsForDeploymentTarget function", func() {

		var apiNamespace *corev1.Namespace
		var k8sClient client.WithWatch
		var bindingReconciler SnapshotEnvironmentBindingReconciler
		ctx := context.Background()

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				testApiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			apiNamespace = testApiNamespace

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testApiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			bindingReconciler = SnapshotEnvironmentBindingReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

		})

		When("the function is passed an object that is not DeploymentTarget", func() {

			It("should return an empty set", func() {
				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "my-app",
						Environment: "some-other-env",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}

				Expect(bindingReconciler.findObjectsForDeploymentTarget(&seb)).To(BeEmpty())
			})

		})

		When("the function is passed a valid chain of DT to DTC to Environment to SEB", func() {

			It("should successfully return the namespaced name of the SEB ", func() {

				By("create a new DeploymentTarget")
				dt := appstudiosharedv1.DeploymentTarget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dt",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.DeploymentTargetSpec{
						DeploymentTargetClassName: "test-class",
						// Likewise, for this test we don't need real values
						KubernetesClusterCredentials: appstudiosharedv1.DeploymentTargetKubernetesClusterCredentials{
							APIURL:                     "https://fake-url.redhat.com",
							ClusterCredentialsSecret:   "not-a-real-secret",
							DefaultNamespace:           "some-other-dtc-namespace",
							AllowInsecureSkipTLSVerify: true,
						},
					},
				}
				err := k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				By("create a DeploymentTargetClaim that can bind to the above DeploymentTarget")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: dt.Namespace,
					},
					Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
						TargetName:                dt.Name,
						DeploymentTargetClassName: dt.Spec.DeploymentTargetClassName,
					},
				}
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("creating a new Environment that references the DeploymentTarget")
				env := appstudiosharedv1.Environment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Environment",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-env",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.EnvironmentSpec{
						DisplayName: "my-env",
						Configuration: appstudiosharedv1.EnvironmentConfiguration{
							Target: appstudiosharedv1.EnvironmentTarget{
								DeploymentTargetClaim: appstudiosharedv1.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}
				err = k8sClient.Create(ctx, &env)
				Expect(err).ToNot(HaveOccurred())

				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "my-app",
						Environment: env.Name,
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}
				err = k8sClient.Create(ctx, &seb)
				Expect(err).ToNot(HaveOccurred())

				Expect(bindingReconciler.findObjectsForDeploymentTarget(&dt)).To(ConsistOf([]reconcile.Request{
					{NamespacedName: client.ObjectKeyFromObject(&seb)},
				}))

			})
		})
	})

	Context("Test SnapshotEnvironmentBinding's findObjectsForDeploymentTargetClaim function", func() {

		var apiNamespace *corev1.Namespace
		var k8sClient client.WithWatch
		var bindingReconciler SnapshotEnvironmentBindingReconciler
		ctx := context.Background()

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				testApiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			apiNamespace = testApiNamespace

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testApiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			bindingReconciler = SnapshotEnvironmentBindingReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

		})

		When("the function is passed an object that is not DeploymentTargetClaim", func() {

			It("should return an empty set", func() {
				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "my-app",
						Environment: "some-other-env",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}

				Expect(bindingReconciler.findObjectsForDeploymentTargetClaim(&seb)).To(BeEmpty())
			})

		})

		When("the function is passed a valid chain of DTC to Environment to SEB", func() {

			It("should successfully return the namespaced name of the SEB ", func() {

				By("create a DeploymentTargetClaim that can bind to the above DeploymentTarget")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: apiNamespace.Namespace,
					},
					Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
						TargetName:                "some-dt",
						DeploymentTargetClassName: "some-test-class",
					},
				}
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("creating a new Environment that references the DeploymentTarget")
				env := appstudiosharedv1.Environment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Environment",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-env",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.EnvironmentSpec{
						DisplayName: "my-env",
						Configuration: appstudiosharedv1.EnvironmentConfiguration{
							Target: appstudiosharedv1.EnvironmentTarget{
								DeploymentTargetClaim: appstudiosharedv1.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}
				err = k8sClient.Create(ctx, &env)
				Expect(err).ToNot(HaveOccurred())

				By("creating a SnapshotEnvironmentBinding that references the Environment")
				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "my-app",
						Environment: env.Name,
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}
				err = k8sClient.Create(ctx, &seb)
				Expect(err).ToNot(HaveOccurred())

				Expect(bindingReconciler.findObjectsForDeploymentTargetClaim(&dtc)).To(ConsistOf([]reconcile.Request{
					{NamespacedName: client.ObjectKeyFromObject(&seb)},
				}))

			})
		})

		When("the function is passed a DTC that points to Environment, but the Environment does not link to the SEB", func() {

			It("should successfully return empty set", func() {

				By("create a DeploymentTargetClaim that can bind to the above DeploymentTarget")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: apiNamespace.Namespace,
					},
					Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
						TargetName:                "some-dt",
						DeploymentTargetClassName: "some-test-class",
					},
				}
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("creating a new Environment that references the DeploymentTarget")
				env := appstudiosharedv1.Environment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Environment",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-env",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.EnvironmentSpec{
						DisplayName: "my-env",
						Configuration: appstudiosharedv1.EnvironmentConfiguration{
							Target: appstudiosharedv1.EnvironmentTarget{
								DeploymentTargetClaim: appstudiosharedv1.DeploymentTargetClaimConfig{
									ClaimName: dtc.Name,
								},
							},
						},
					},
				}
				err = k8sClient.Create(ctx, &env)
				Expect(err).ToNot(HaveOccurred())

				By("creating a SnapshotEnvironmentBinding that DOESN'T reference the Environment")
				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "my-app",
						Environment: "some-other-environment",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}
				err = k8sClient.Create(ctx, &seb)
				Expect(err).ToNot(HaveOccurred())

				Expect(bindingReconciler.findObjectsForDeploymentTargetClaim(&dtc)).To(BeEmpty())

			})
		})

	})

	Context("Test SnapshotEnvironmentBinding's findObjectsForEnvironment function", func() {
		ctx := context.Background()
		var apiNamespace *corev1.Namespace

		var k8sClient client.WithWatch

		var bindingReconciler SnapshotEnvironmentBindingReconciler
		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				testApiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			apiNamespace = testApiNamespace

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testApiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			bindingReconciler = SnapshotEnvironmentBindingReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

		})

		When("SnapshotEnvironmentBinding points to a non-existing env", func() {

			It("verifies that findObjectForEnvironment should return an empty set", func() {

				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "my-app",
						Environment: "does-not-exist",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}
				err := k8sClient.Create(ctx, &seb)
				Expect(err).ToNot(HaveOccurred())

				env := appstudiosharedv1.Environment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Environment",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-other-env",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.EnvironmentSpec{
						DisplayName: "some-other-env",
					},
				}

				Expect(bindingReconciler.findObjectsForEnvironment(&env)).To(BeEmpty())
			})

		})

		When("SnapshotEnvironmentBinding points to a different env", func() {

			It("verifies that findObjectForEnvironment should return an empty set", func() {

				env := appstudiosharedv1.Environment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Environment",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-other-env",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.EnvironmentSpec{
						DisplayName: "some-other-env",
					},
				}
				err := k8sClient.Create(ctx, &env)
				Expect(err).ToNot(HaveOccurred())

				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "my-app",
						Environment: "some-other-env",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}
				err = k8sClient.Create(ctx, &seb)
				Expect(err).ToNot(HaveOccurred())

				thirdEnv := appstudiosharedv1.Environment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Environment",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "a-third-env",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.EnvironmentSpec{
						DisplayName: "a-third-env",
					},
				}

				Expect(bindingReconciler.findObjectsForEnvironment(&thirdEnv)).To(BeEmpty())
			})

		})

		When("SnapshotEnvironmentBinding points to the same Environment that is passed into findObjectsForEnvironment", func() {

			It("verifies that findObjectForEnvironment should return the SEB pointing to that env", func() {

				env := appstudiosharedv1.Environment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Environment",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-other-env",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.EnvironmentSpec{
						DisplayName: "some-other-env",
					},
				}
				err := k8sClient.Create(ctx, &env)
				Expect(err).ToNot(HaveOccurred())

				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "my-app",
						Environment: "some-other-env",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}
				err = k8sClient.Create(ctx, &seb)
				Expect(err).ToNot(HaveOccurred())

				Expect(bindingReconciler.findObjectsForEnvironment(&env)).
					To(Equal([]reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(&seb)}}))
			})

		})

		When("bindingReconciler.findObjectsForEnvironment is passed another object", func() {
			It("should return an empty set", func() {

				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "my-app",
						Environment: "some-other-env",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}

				Expect(bindingReconciler.findObjectsForEnvironment(&seb)).To(BeEmpty())
			})
		})

	})

	Context("Test SnapshotEnvironmentBinding's findObjectsForApplication function", func() {
		ctx := context.Background()
		var apiNamespace *corev1.Namespace

		var k8sClient client.WithWatch

		var bindingReconciler SnapshotEnvironmentBindingReconciler
		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				testApiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			apiNamespace = testApiNamespace

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testApiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			bindingReconciler = SnapshotEnvironmentBindingReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

		})

		When("SnapshotEnvironmentBinding points to a non-existing app", func() {
			It("verifies that findObjectsForApplication should return an empty slice", func() {

				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "does-not-exist",
						Environment: "my-env",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}
				err := k8sClient.Create(ctx, &seb)
				Expect(err).ToNot(HaveOccurred())

				app := appstudiosharedv1.Application{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Application",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-other-app",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.ApplicationSpec{
						DisplayName: "some-other-app",
					},
				}

				Expect(bindingReconciler.findObjectsForApplication(&app)).To(BeEmpty())
			})
		})

		When("SnapshotEnvironmentBinding points to a different app", func() {
			It("verifies that findObjectsForApplication should return an empty slice", func() {

				app := appstudiosharedv1.Application{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Application",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-other-app",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.ApplicationSpec{
						DisplayName: "some-other-app",
					},
				}
				err := k8sClient.Create(ctx, &app)
				Expect(err).ToNot(HaveOccurred())

				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "some-other-app",
						Environment: "my-env",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}
				err = k8sClient.Create(ctx, &seb)
				Expect(err).ToNot(HaveOccurred())

				thirdApp := appstudiosharedv1.Application{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Application",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "a-third-app",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.ApplicationSpec{
						DisplayName: "a-third-app",
					},
				}

				Expect(bindingReconciler.findObjectsForApplication(&thirdApp)).To(BeEmpty())
			})
		})

		When("SnapshotEnvironmentBinding points to the same Application that is passed into findObjectsForApplication", func() {
			It("verifies that findObjectsForApplication should return the SEB pointing to that app", func() {

				env := appstudiosharedv1.Application{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Application",
						APIVersion: appstudiosharedv1.GroupVersion.Identifier(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-other-app",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.ApplicationSpec{
						DisplayName: "some-other-app",
					},
				}
				err := k8sClient.Create(ctx, &env)
				Expect(err).ToNot(HaveOccurred())

				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "some-other-app",
						Environment: "my-env",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}
				err = k8sClient.Create(ctx, &seb)
				Expect(err).ToNot(HaveOccurred())

				Expect(bindingReconciler.findObjectsForApplication(&env)).
					To(Equal([]reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(&seb)}}))
			})
		})

		When("bindingReconciler.findObjectsForApplication is passed another object", func() {
			It("should return an empty slice", func() {

				seb := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
					Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
						Application: "some-other-app",
						Environment: "my-env",
						Snapshot:    "my-snapshot",
						Components:  []appstudiosharedv1.BindingComponent{},
					},
				}

				Expect(bindingReconciler.findObjectsForApplication(&seb)).To(BeEmpty())
			})
		})

	})

	Context("SnapshotEnvironmentBindingReconciler ComponentDeploymentConditions", func() {
		var ctx context.Context
		var request reconcile.Request
		var binding *appstudiosharedv1.SnapshotEnvironmentBinding
		var bindingReconciler SnapshotEnvironmentBindingReconciler

		var environment appstudiosharedv1.Environment
		var application appstudiosharedv1.Application

		BeforeEach(func() {
			ctx = context.Background()

			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			// Create fake client
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			// Create placeholder environment
			environment = appstudiosharedv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "staging",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1.EnvironmentSpec{
					DisplayName:        "my-environment",
					DeploymentStrategy: appstudiosharedv1.DeploymentStrategy_AppStudioAutomated,
					ParentEnvironment:  "",
					Tags:               []string{},
					Configuration:      appstudiosharedv1.EnvironmentConfiguration{},
				},
			}
			err = k8sClient.Create(ctx, &environment)
			Expect(err).ToNot(HaveOccurred())

			// Create placeholder application
			application = appstudiosharedv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-demo-app",
					Namespace: apiNamespace.Name,
				},
				Spec: appstudiosharedv1.ApplicationSpec{
					DisplayName: "my-application",
				},
			}
			err = k8sClient.Create(ctx, &application)
			Expect(err).ToNot(HaveOccurred())

			bindingReconciler = SnapshotEnvironmentBindingReconciler{Client: k8sClient, Scheme: scheme}

			// Create SnapshotEnvironmentBinding CR.
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: apiNamespace.Name,
					Labels: map[string]string{
						"appstudio.application": "new-demo-app",
						"appstudio.environment": "staging",
					},
				},
				Spec: appstudiosharedv1.SnapshotEnvironmentBindingSpec{
					Application: "new-demo-app",
					Environment: "staging",
					Snapshot:    "my-snapshot",
					Components: []appstudiosharedv1.BindingComponent{
						{
							Name: "component-a",
							Configuration: appstudiosharedv1.BindingComponentConfiguration{
								Env: []appstudiosharedv1.EnvVarPair{
									{Name: "My_STG_ENV", Value: "1000"},
								},
								Replicas: &threeReplicas,
							},
						},
						{
							Name: "component-b",
							Configuration: appstudiosharedv1.BindingComponentConfiguration{
								Env: []appstudiosharedv1.EnvVarPair{
									{Name: "My_STG_ENV", Value: "1000"},
								},
								Replicas: &threeReplicas,
							},
						},
					},
				},
				Status: appstudiosharedv1.SnapshotEnvironmentBindingStatus{
					Components: []appstudiosharedv1.BindingComponentStatus{
						{
							Name: "component-a",
							GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
								URL:    "https://github.com/redhat-appstudio/managed-gitops",
								Branch: "main",
								Path:   "resources/test-data/sample-gitops-repository/components/componentA/overlays/staging",
							},
						},
						{
							Name: "component-b",
							GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
								URL:    "https://github.com/redhat-appstudio/managed-gitops",
								Branch: "main",
								Path:   "resources/test-data/sample-gitops-repository/components/componentB/overlays/staging",
							},
						},
					},
				},
			}

			// Create request object for Reconciler
			request = newRequest(apiNamespace.Name, binding.Name)
		})

		It("sets the binding's ComponentDeploymentConditions status field when some components are out of sync.", func() {
			By("Creating SnapshotEnvironmentBinding CR in cluster.")
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("Checking status field before calling Reconciler")
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, binding)
			Expect(err).ToNot(HaveOccurred())
			Expect(binding.Status.ComponentDeploymentConditions).To(BeEmpty())

			By("Triggering Reconciler to create the GitOpsDeployments")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Get(ctx, request.NamespacedName, binding)
			Expect(err).ToNot(HaveOccurred())

			By("Updating the first GitOpsDeployment to simulate a successful sync")
			deployment := &apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      binding.Status.GitOpsDeployments[0].GitOpsDeployment,
					Namespace: binding.Namespace,
				},
			}
			err = bindingReconciler.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
			Expect(err).ToNot(HaveOccurred())
			deployment.Status.Sync.Status = apibackend.SyncStatusCodeSynced
			err = bindingReconciler.Status().Update(ctx, deployment)
			Expect(err).ToNot(HaveOccurred())

			By("Updating the second GitOpsDeployment to simulate a failed sync")
			deployment = &apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      binding.Status.GitOpsDeployments[1].GitOpsDeployment,
					Namespace: binding.Namespace,
				},
			}
			err = bindingReconciler.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
			Expect(err).ToNot(HaveOccurred())
			deployment.Status.Sync.Status = apibackend.SyncStatusCodeOutOfSync
			err = bindingReconciler.Status().Update(ctx, deployment)
			Expect(err).ToNot(HaveOccurred())

			By("Triggering Reconciler to update the status ComponentDeploymentConditions field")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("Checking status field after calling Reconciler")
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Get(ctx, request.NamespacedName, binding)
			Expect(err).ToNot(HaveOccurred())
			Expect(binding.Status.ComponentDeploymentConditions).To(HaveLen(1))
			Expect(binding.Status.ComponentDeploymentConditions[0].Type).To(Equal(appstudiosharedv1.ComponentDeploymentConditionAllComponentsDeployed))
			Expect(binding.Status.ComponentDeploymentConditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(binding.Status.ComponentDeploymentConditions[0].Reason).To(Equal(appstudiosharedv1.ComponentDeploymentConditionCommitsUnsynced))
			Expect(binding.Status.ComponentDeploymentConditions[0].Message).To(Equal("1 of 2 components deployed"))
		})

		// Similar to previous test except to test the SnapshotEnvironmentBinding's statuses of its GitOpsDeployments
		It("sets the status fields of the GitOpsDeployments of a SnapshotEnvironmentBinding", func() {
			By("Creating SnapshotEnvironmentBinding CR in cluster.")
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			// Initially there are no deployments
			Expect(binding.Status.GitOpsDeployments).To(BeEmpty())

			By("Checking status field before calling Reconciler")
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Get(ctx, request.NamespacedName, binding)
			Expect(err).ToNot(HaveOccurred())
			Expect(binding.Status.ComponentDeploymentConditions).To(BeEmpty())

			By("Triggering Reconciler to create the GitOpsDeployments")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Get(ctx, request.NamespacedName, binding)
			Expect(err).ToNot(HaveOccurred())

			// After reconciling, there should be two deployments
			Expect(binding.Status.GitOpsDeployments).To(HaveLen(2))
			// We are not sure of the order
			var indexOfComponentA, indexOfComponentB int
			if binding.Status.GitOpsDeployments[0].ComponentName == "component-a" {
				indexOfComponentA = 0
				indexOfComponentB = 1
			} else { // There are only two components, so the indices are switched
				indexOfComponentA = 1
				indexOfComponentB = 0
			}
			// Check the deployment name too
			// Eg. appa-staging-binding-new-demo-app-staging-component-a
			componentADeploymentName := GenerateBindingGitOpsDeploymentName(*binding, "component-a")

			// Eg. appa-staging-binding-new-demo-app-staging-component-b
			componentBDeploymentName := GenerateBindingGitOpsDeploymentName(*binding, "component-b")

			Expect(binding.Status.GitOpsDeployments[indexOfComponentA].GitOpsDeployment).To(Equal(componentADeploymentName))
			Expect(binding.Status.GitOpsDeployments[indexOfComponentB].GitOpsDeployment).To(Equal(componentBDeploymentName))
			// Check that the Sync and Health statuses, and the commit ID are not set yet
			Expect(binding.Status.GitOpsDeployments[indexOfComponentA].GitOpsDeploymentSyncStatus).To(Equal(""))
			Expect(binding.Status.GitOpsDeployments[indexOfComponentA].GitOpsDeploymentHealthStatus).To(Equal(""))
			Expect(binding.Status.GitOpsDeployments[indexOfComponentA].GitOpsDeploymentCommitID).To(Equal(""))
			Expect(binding.Status.GitOpsDeployments[indexOfComponentB].GitOpsDeploymentSyncStatus).To(Equal(""))
			Expect(binding.Status.GitOpsDeployments[indexOfComponentB].GitOpsDeploymentHealthStatus).To(Equal(""))
			Expect(binding.Status.GitOpsDeployments[indexOfComponentB].GitOpsDeploymentCommitID).To(Equal(""))

			By("Updating the first GitOpsDeployment to simulate a successful sync")
			deployment := &apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      binding.Status.GitOpsDeployments[indexOfComponentA].GitOpsDeployment,
					Namespace: binding.Namespace,
				},
			}
			err = bindingReconciler.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
			Expect(err).ToNot(HaveOccurred())
			deployment1Revision := "1.0.0"
			deployment.Status.Sync.Status = apibackend.SyncStatusCodeSynced
			deployment.Status.Health.Status = apibackend.HeathStatusCodeHealthy
			deployment.Status.Sync.Revision = deployment1Revision
			err = bindingReconciler.Status().Update(ctx, deployment)
			Expect(err).ToNot(HaveOccurred())

			By("Updating the second GitOpsDeployment to simulate a failed sync")
			deployment = &apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      binding.Status.GitOpsDeployments[indexOfComponentB].GitOpsDeployment,
					Namespace: binding.Namespace,
				},
			}
			err = bindingReconciler.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
			Expect(err).ToNot(HaveOccurred())
			deployment2Revision := "2.0.0"
			deployment.Status.Sync.Status = apibackend.SyncStatusCodeOutOfSync
			deployment.Status.Health.Status = apibackend.HeathStatusCodeDegraded
			deployment.Status.Sync.Revision = deployment2Revision
			err = bindingReconciler.Status().Update(ctx, deployment)
			Expect(err).ToNot(HaveOccurred())

			By("Triggering Reconciler to update the status field")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("Checking status field after calling Reconciler")
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Get(ctx, request.NamespacedName, binding)
			Expect(err).ToNot(HaveOccurred())
			// Order of components not predictable, so just check the status for the specific name
			for _, deployment := range binding.Status.GitOpsDeployments {
				// For these unit test purposes, we can compare exactly the Commit IDs, although in the e2e tests,
				// it is not deterministic.
				if deployment.ComponentName == "component-a" {
					Expect(deployment.GitOpsDeploymentSyncStatus).To(Equal(string(apibackend.SyncStatusCodeSynced)))
					Expect(deployment.GitOpsDeploymentHealthStatus).To(Equal(string(apibackend.HeathStatusCodeHealthy)))
					Expect(deployment.GitOpsDeploymentCommitID).To(Equal(deployment1Revision))
				} else { // For component-b
					Expect(deployment.GitOpsDeploymentSyncStatus).To(Equal(string(apibackend.SyncStatusCodeOutOfSync)))
					Expect(deployment.GitOpsDeploymentHealthStatus).To(Equal(string(apibackend.HeathStatusCodeDegraded)))
					Expect(deployment.GitOpsDeploymentCommitID).To(Equal(deployment2Revision))
				}
			}
		})

		It("sets the binding's ComponentDeploymentConditions status field when all components deployed successfully.", func() {
			By("Creating SnapshotEnvironmentBinding CR in cluster.")
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).ToNot(HaveOccurred())

			By("Checking status field before calling Reconciler")
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, binding)
			Expect(err).ToNot(HaveOccurred())
			Expect(binding.Status.ComponentDeploymentConditions).To(BeEmpty())

			By("Triggering Reconciler to create the GitOpsDeployments")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Get(ctx, request.NamespacedName, binding)
			Expect(err).ToNot(HaveOccurred())

			By("Updating the first GitOpsDeployment to simulate a successful sync")
			deployment := &apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      binding.Status.GitOpsDeployments[0].GitOpsDeployment,
					Namespace: binding.Namespace,
				},
			}
			err = bindingReconciler.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
			Expect(err).ToNot(HaveOccurred())
			deployment.Status.Sync.Status = apibackend.SyncStatusCodeSynced
			err = bindingReconciler.Status().Update(ctx, deployment)
			Expect(err).ToNot(HaveOccurred())

			By("Updating the first GitOpsDeployment to simulate a successful sync")
			deployment = &apibackend.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      binding.Status.GitOpsDeployments[1].GitOpsDeployment,
					Namespace: binding.Namespace,
				},
			}
			err = bindingReconciler.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
			Expect(err).ToNot(HaveOccurred())
			deployment.Status.Sync.Status = apibackend.SyncStatusCodeSynced
			err = bindingReconciler.Status().Update(ctx, deployment)
			Expect(err).ToNot(HaveOccurred())

			By("Triggering Reconciler to update the status ComponentDeploymentConditions field")
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			By("Checking status field after calling Reconciler")
			binding = &appstudiosharedv1.SnapshotEnvironmentBinding{}
			err = bindingReconciler.Get(ctx, request.NamespacedName, binding)
			Expect(err).ToNot(HaveOccurred())
			Expect(binding.Status.ComponentDeploymentConditions).To(HaveLen(1))
			Expect(binding.Status.ComponentDeploymentConditions[0].Type).To(Equal(appstudiosharedv1.ComponentDeploymentConditionAllComponentsDeployed))
			Expect(binding.Status.ComponentDeploymentConditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(binding.Status.ComponentDeploymentConditions[0].Reason).To(Equal(appstudiosharedv1.ComponentDeploymentConditionCommitsSynced))
			Expect(binding.Status.ComponentDeploymentConditions[0].Message).To(Equal("2 of 2 components deployed"))
		})
	})

	Context("verify functions that are used to ensure that GitOpsDeployments generated by the Binding controller contain "+
		"appstudio labels from the parent", func() {

		DescribeTable("test scenarios of updateMapWithExpectedAppStudioLabels()", func(actualLabels, desiredLabels, expectedResult map[string]string) {

			res := updateMapWithExpectedAppStudioLabels(actualLabels, desiredLabels)
			Expect(res).To(Equal(expectedResult))
		},
			Entry("nil check", nil, nil, nil),
			Entry("empty actual converted to nil", map[string]string{}, nil, nil),
			Entry("empty actual/desired converted to nil", map[string]string{}, map[string]string{}, nil),
			Entry("actual map without appstudio labels shouldn't be modified", map[string]string{
				"a": "b",
			}, map[string]string{}, map[string]string{"a": "b"}),
			Entry("desired appstudio labels should be added, while preserving old labels", map[string]string{
				"a": "b",
			}, map[string]string{appstudioLabelKey + "/label": "appstudio-value"},
				map[string]string{"a": "b", appstudioLabelKey + "/label": "appstudio-value"}),
			Entry("old appstudio labels should be removed, if they are not desired", map[string]string{
				"a": "b", appstudioLabelKey + "/label": "appstudio-value",
			}, map[string]string{}, map[string]string{"a": "b"}),
			Entry("existing appstudio labels should not be touched", map[string]string{
				"a": "b", appstudioLabelKey + "/label": "appstudio-value",
			}, map[string]string{appstudioLabelKey + "/label": "appstudio-value"}, map[string]string{"a": "b", appstudioLabelKey + "/label": "appstudio-value"}),
		)

		DescribeTable("test scenarios of areAppStudioLabelsEqualBetweenMaps()", func(map1, map2 map[string]string, expectedResult bool) {

			res := areAppStudioLabelsEqualBetweenMaps(map1, map2)
			Expect(res).To(Equal(expectedResult))
		},
			Entry("nil check both", nil, nil, true),
			Entry("nil check first", nil, map[string]string{}, true),
			Entry("nil check second", map[string]string{}, nil, true),
			Entry("empty maps", map[string]string{}, map[string]string{}, true),
			Entry("appstudio label in first, but not second",
				map[string]string{appstudioLabelKey + "/label": "value"}, map[string]string{}, false),
			Entry("appstudio label in second, but not first",
				map[string]string{}, map[string]string{appstudioLabelKey + "/label": "value"}, false),
			Entry("same appstudio label in both",
				map[string]string{appstudioLabelKey + "/label": "value"}, map[string]string{appstudioLabelKey + "/label": "value"}, true),
			Entry("same appstudio label in both, but mismatching non-appstudio",
				map[string]string{appstudioLabelKey + "/label": "value", "a": "b"},
				map[string]string{appstudioLabelKey + "/label": "value", "c": "d", "e": "f"}, true),

			Entry("different appstudio label in both",
				map[string]string{appstudioLabelKey + "/label1": "value"}, map[string]string{appstudioLabelKey + "/label2": "value"}, false),
		)

	})

	Context("Test isRequestNamespaceBeingDeleted", func() {

		var k8sClient client.Client
		ctx := context.Background()
		log := log.FromContext(ctx)

		BeforeEach(func() {
			// Create fake client
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

		})

		It("should return false if the Namespace is not being deleted", func() {

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns",
				},
			}
			err := k8sClient.Create(ctx, namespace)
			Expect(err).ToNot(HaveOccurred())

			res, err := isRequestNamespaceBeingDeleted(ctx, namespace.Name, k8sClient, log)
			Expect(res).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())

		})

		It("should return true if the Namespace has a deletionTimestamp", func() {

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ns",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
			}
			err := k8sClient.Create(ctx, namespace)
			Expect(err).ToNot(HaveOccurred())

			res, err := isRequestNamespaceBeingDeleted(ctx, namespace.Name, k8sClient, log)
			Expect(res).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return false, with no error, if the Namespace doesn't exist", func() {
			res, err := isRequestNamespaceBeingDeleted(ctx, "does-not-exist", k8sClient, log)
			Expect(res).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())
		})

	})

	Context("Unit tests of non-reconcile functions", func() {

		ctx := context.Background()

		var k8sClient sharedutil.ProxyClient
		var apiNamespace corev1.Namespace
		log := log.FromContext(ctx)

		var eventReceiver sharedutil.ListEventReceiver

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				namespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

			apiNamespace = *namespace

			// Create fake client
			fakeK8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(namespace, argocdNamespace, kubesystemNamespace).
				Build()

			k8sClient = sharedutil.ProxyClient{
				InnerClient: fakeK8sClient,
				Informer:    &eventReceiver,
			}

		})

		DescribeTable("verify updateBindingConditionOfSEB works as expected",
			func(preCondition []metav1.Condition, newCondition metav1.Condition, expectedResult []metav1.Condition, expectUpdateStatusCalled bool) {

				By("creating an initial SEB with the initial state of status.bindingConditions")
				env := appstudiosharedv1.SnapshotEnvironmentBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-seb",
						Namespace: apiNamespace.Name,
					},
				}

				err := k8sClient.Create(ctx, &env)
				Expect(err).ToNot(HaveOccurred())
				env.Status.BindingConditions = preCondition
				err = k8sClient.Update(ctx, &env)
				Expect(err).ToNot(HaveOccurred())

				By("resetting the list of events after we updated the binding conditions")
				eventReceiver.Events = []sharedutil.ProxyClientEvent{}

				By("calling the binding condition update function")
				err = updateSEBReconciledStatusCondition(ctx, &k8sClient, newCondition.Message, &env,
					newCondition.Status, newCondition.Reason, log)
				Expect(err).ToNot(HaveOccurred())

				By("Verifying the status bindingConditions field was updated as expected")

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&env), &env)
				Expect(err).ToNot(HaveOccurred())

				Expect(env.Status.BindingConditions).To(HaveLen(2))

				expectedCondition := expectedResult[0]
				actualCondition := env.Status.BindingConditions[0]
				Expect(actualCondition.Message).To(Equal(expectedCondition.Message))
				Expect(actualCondition.Type).To(Equal(expectedCondition.Type))
				Expect(actualCondition.Reason).To(Equal(expectedCondition.Reason))
				Expect(actualCondition.Status).To(Equal(expectedCondition.Status))

				expectedCondition = expectedResult[1]
				actualCondition = env.Status.BindingConditions[1]
				Expect(actualCondition.Message).To(Equal(expectedCondition.Message))
				Expect(actualCondition.Type).To(Equal(expectedCondition.Type))
				Expect(actualCondition.Reason).To(Equal(expectedCondition.Reason))
				Expect(actualCondition.Status).To(Equal(expectedCondition.Status))

				statusUpdated := false
				for _, event := range eventReceiver.Events {
					if event.Action == sharedutil.StatusUpdate {
						statusUpdated = true
					}
				}
				Expect(statusUpdated).To(Equal(expectUpdateStatusCalled),
					"Verify that the status of the SEB is only updated if we expected it to be: it should only be updated if one of the conditions actually changed")

				if expectUpdateStatusCalled {
					Expect(time.Now().Add(-1*time.Minute).Before(actualCondition.LastTransitionTime.Time)).To(BeTrue(),
						"if the status was updated, then the last transition time should be within the last minute or so")
				} else {
					Expect(time.Now().After(actualCondition.LastTransitionTime.Time)).To(BeTrue(),
						"if the status was not updated, then the last transition time should be empty")
				}

			},
			Entry("add a new condition",
				[]metav1.Condition{},
				metav1.Condition{
					Status:  metav1.ConditionFalse,
					Reason:  "my-reason",
					Message: "my-message",
				},
				[]metav1.Condition{
					{
						Type:    SnapshotEnvironmentBindingConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  SnapshotEnvironmentBindingReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    SnapshotEnvironmentBindingConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason",
						Message: "my-message",
					},
				},
				true,
			),
			Entry("replace an existing condition with mismatched reason",
				[]metav1.Condition{
					{
						Type:    SnapshotEnvironmentBindingConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  SnapshotEnvironmentBindingReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    SnapshotEnvironmentBindingConditionReconciled,
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
						Type:    SnapshotEnvironmentBindingConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  SnapshotEnvironmentBindingReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    SnapshotEnvironmentBindingConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason2",
						Message: "my-message",
					},
				},
				true,
			),
			Entry("replace an existing condition with mismatched message",
				[]metav1.Condition{
					{
						Type:    SnapshotEnvironmentBindingConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  SnapshotEnvironmentBindingReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    SnapshotEnvironmentBindingConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason",
						Message: "my-message",
					},
				}, metav1.Condition{
					Status:  metav1.ConditionFalse,
					Reason:  "my-reason",
					Message: "my-message2",
				},
				[]metav1.Condition{
					{
						Type:    SnapshotEnvironmentBindingConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  SnapshotEnvironmentBindingReasonErrorOccurred,
						Message: "my-message2",
					},
					{
						Type:    SnapshotEnvironmentBindingConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason",
						Message: "my-message2",
					},
				},
				true,
			),
			Entry("replace an existing condition with mismatched status",
				[]metav1.Condition{
					{
						Type:    SnapshotEnvironmentBindingConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  SnapshotEnvironmentBindingReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    SnapshotEnvironmentBindingConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason",
						Message: "my-message",
					},
				}, metav1.Condition{
					Status:  metav1.ConditionTrue,
					Reason:  "my-reason",
					Message: "my-message",
				},
				[]metav1.Condition{
					{
						Type:    SnapshotEnvironmentBindingConditionErrorOccurred,
						Status:  metav1.ConditionFalse,
						Reason:  SnapshotEnvironmentBindingReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    SnapshotEnvironmentBindingConditionReconciled,
						Status:  metav1.ConditionTrue,
						Reason:  "my-reason",
						Message: "my-message",
					},
				},
				true,
			),
			Entry("if nothing has changed, update should not be called",
				[]metav1.Condition{
					{
						Type:    SnapshotEnvironmentBindingConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  SnapshotEnvironmentBindingReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    SnapshotEnvironmentBindingConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason",
						Message: "my-message",
					},
				}, metav1.Condition{
					Status:  metav1.ConditionFalse,
					Reason:  "my-reason",
					Message: "my-message",
				},
				[]metav1.Condition{
					{
						Type:    SnapshotEnvironmentBindingConditionErrorOccurred,
						Status:  metav1.ConditionTrue,
						Reason:  SnapshotEnvironmentBindingReasonErrorOccurred,
						Message: "my-message",
					},
					{
						Type:    SnapshotEnvironmentBindingConditionReconciled,
						Status:  metav1.ConditionFalse,
						Reason:  "my-reason",
						Message: "my-message",
					},
				},
				false,
			),
		)
	})
})

// newRequest contains the information necessary to reconcile a Kubernetes object.
func newRequest(namespace, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func checkStatusConditionOfEnvironmentBinding(ctx context.Context, rClient client.Client, binding *appstudiosharedv1.SnapshotEnvironmentBinding, message string, status metav1.ConditionStatus, reason string) {
	err := rClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(binding.Status.BindingConditions)).To(BeNumerically(">=", 2))

	found1 := false
	found2 := false

	for _, condition := range binding.Status.BindingConditions {
		if condition.Type == SnapshotEnvironmentBindingConditionErrorOccurred {
			ExpectWithOffset(1, found1).To(BeFalse(), "found more than one SnapshotEnvironmentBindingConditionErrorOccurred condition")
			ExpectWithOffset(1, condition.Message).To(Equal(message), "mismatched SnapshotEnvironmentBindingConditionErrorOccurred message")
			if status == metav1.ConditionTrue {
				ExpectWithOffset(1, condition.Status).To(Equal(metav1.ConditionFalse), "mismatched SnapshotEnvironmentBindingConditionErrorOccurred status")
			} else if status == metav1.ConditionFalse {
				ExpectWithOffset(1, condition.Status).To(Equal(metav1.ConditionTrue), "mismatched SnapshotEnvironmentBindingConditionErrorOccurred status")
			} else {
				ExpectWithOffset(1, condition.Status).To(Equal(metav1.ConditionUnknown), "mismatched SnapshotEnvironmentBindingConditionErrorOccurred status")
			}
			ExpectWithOffset(1, condition.Reason).To(Equal(SnapshotEnvironmentBindingReasonErrorOccurred), "mismatched SnapshotEnvironmentBindingConditionErrorOccurred reason")
			found1 = true
		} else if condition.Type == SnapshotEnvironmentBindingConditionReconciled {
			ExpectWithOffset(1, found2).To(BeFalse(), "found more than one SnapshotEnvironmentBindingConditionReconciled condition")
			ExpectWithOffset(1, condition.Message).To(Equal(message), "mismatched SnapshotEnvironmentBindingConditionReconciled message")
			ExpectWithOffset(1, condition.Status).To(Equal(status), "mismatched SnapshotEnvironmentBindingConditionReconciled status")
			ExpectWithOffset(1, condition.Reason).To(Equal(reason), "mismatched SnapshotEnvironmentBindingConditionReconciled reason")
			found2 = true
		}
	}
	ExpectWithOffset(1, found1).To(BeTrue(), "missing SnapshotEnvironmentBindingConditionErrorOccurred condition")
	ExpectWithOffset(1, found2).To(BeTrue(), "missing SnapshotEnvironmentBindingConditionReconciled condition")
}
