package appstudioredhatcom_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appstudiocontrollers "github.com/redhat-appstudio/managed-gitops/appstudio-controller/controllers/appstudio.redhat.com"
	appstudiosharedv1 "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	apibackend "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	apierr "k8s.io/apimachinery/pkg/api/errors"
)

var _ = Describe("Application Snapshot Environment Binding Reconciler Tests", func() {

	Context("Testing ApplicationSnapshotEnvironmentBindingReconciler.", func() {
		var ctx context.Context
		var request reconcile.Request
		var binding *appstudiosharedv1.ApplicationSnapshotEnvironmentBinding
		var bindingReconciler appstudiocontrollers.ApplicationSnapshotEnvironmentBindingReconciler

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			// Create fake client
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, argocdNamespace, kubesystemNamespace).
				Build()

			bindingReconciler = appstudiocontrollers.ApplicationSnapshotEnvironmentBindingReconciler{
				Client: k8sClient, Scheme: scheme}

			// Create ApplicationSnapshotEnvironmentBinding CR.
			binding = &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "appa-staging-binding",
					Namespace: kubesystemNamespace.Name,
					Labels: map[string]string{
						"appstudio.application": "new-demo-app",
						"appstudio.environment": "staging",
					},
				},
				Spec: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingSpec{
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
								Replicas: 3,
							},
						},
					},
				},
				Status: appstudiosharedv1.ApplicationSnapshotEnvironmentBindingStatus{
					Components: []appstudiosharedv1.ComponentStatus{
						{
							Name: "component-a",
							GitOpsRepository: appstudiosharedv1.BindingComponentGitOpsRepository{
								URL:    "https://github.com/redhat-appstudio/gitops-repository-template",
								Branch: "main",
								Path:   "components/componentA/overlays/staging",
							},
						},
					},
				},
			}

			ctx = context.Background()

			// Create request object for Reconciler
			request = newRequest(kubesystemNamespace.Name, binding.Name)
		})

		It("Should set the status field of Binding.", func() {
			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Check status field before calling Reconciler
			bindingFirst := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, bindingFirst)
			Expect(err).To(BeNil())
			Expect(len(bindingFirst.Status.GitOpsDeployments)).To(Equal(0))

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Check status field after calling Reconciler
			bindingSecond := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, bindingSecond)

			Expect(err).To(BeNil())
			Expect(len(bindingSecond.Status.GitOpsDeployments)).NotTo(Equal(0))
			Expect(bindingSecond.Status.GitOpsDeployments[0].ComponentName).To(Equal("component-a"))
			Expect(bindingSecond.Status.GitOpsDeployments[0].GitOpsDeployment).
				To(Equal(binding.Name + "-" +
					binding.Spec.Application + "-" +
					binding.Spec.Environment + "-" +
					binding.Spec.Components[0].Name))
		})

		It("Should not update GitOpsDeployment if same Binding is created again.", func() {

			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Fetch GitOpsDeployment object before calling Reconciler
			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name: binding.Name + "-" +
					binding.Spec.Application + "-" +
					binding.Spec.Environment + "-" +
					binding.Spec.Components[0].Name}

			gitopsDeploymentFirst := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeploymentFirst)
			Expect(err).To(BeNil())

			// Trigger Reconciler again
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Fetch GitOpsDeployment object after calling Reconciler
			gitopsDeploymentSecond := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeploymentSecond)
			Expect(err).To(BeNil())

			// Reconciler should not do any change in GitOpsDeployment object.
			Expect(gitopsDeploymentFirst).To(Equal(gitopsDeploymentSecond))
		})

		It("Should revert GitOpsDeploymentObject if it's spec is different than Binding Component.", func() {
			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			gitopsDeploymentKey := client.ObjectKey{
				Namespace: binding.Namespace,
				Name: binding.Name + "-" +
					binding.Spec.Application + "-" +
					binding.Spec.Environment + "-" +
					binding.Spec.Components[0].Name}

			// Fetch GitOpsDeployment object
			gitopsDeployment := &apibackend.GitOpsDeployment{}
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).To(BeNil())

			// GitOpsDeployment object spec should be same as Binding Component.
			Expect(gitopsDeployment.Spec.Source.Path).To(Equal(binding.Status.Components[0].GitOpsRepository.Path))

			// Update GitOpsDeploymentObject in cluster.
			gitopsDeployment.Spec.Source.Path = "components/componentA/overlays/dev"
			err = bindingReconciler.Update(ctx, gitopsDeployment)
			Expect(err).To(BeNil())

			// Trigger Reconciler again
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Fetch GitOpsDeployment object after calling Reconciler
			err = bindingReconciler.Get(ctx, gitopsDeploymentKey, gitopsDeployment)
			Expect(err).To(BeNil())

			// Reconciler should revert GitOpsDeployment object, so it will be same as old object
			Expect(gitopsDeployment.Spec.Source.Path).To(Equal(binding.Status.Components[0].GitOpsRepository.Path))
		})

		It("Should use short name for GitOpsDeployment object.", func() {
			// Update application name to exceed the limit
			binding.Spec.Application = strings.Repeat("abcde", 45)
			request = newRequest(binding.Namespace, binding.Name)

			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			// Check status field after calling Reconciler
			binding := &appstudiosharedv1.ApplicationSnapshotEnvironmentBinding{}
			err = bindingReconciler.Client.Get(ctx, request.NamespacedName, binding)

			Expect(err).To(BeNil())
			Expect(len(binding.Status.GitOpsDeployments)).NotTo(Equal(0))
			Expect(binding.Status.GitOpsDeployments[0].ComponentName).To(Equal("component-a"))

			// GitOpsDeployment should have short name
			Expect(binding.Status.GitOpsDeployments[0].GitOpsDeployment).
				To(Equal(binding.Name + "-" + binding.Spec.Components[0].Name))
		})

		It("Should return error as Binding object is not available in cluster.", func() {
			// Trigger Reconciler
			_, err := bindingReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(BeNil())
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("Should return error if Status.Components is not available in Binding object.", func() {
			binding.Status.Components = []appstudiosharedv1.ComponentStatus{}
			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(Equal("ApplicationSnapshotEventBinding Component status is required to generate GitOps deployment, waiting for the Application Service controller to finish reconciling binding appa-staging-binding"))
		})

		It("Should return error if Status.GitOpsRepoConditions Status is set to False in Binding object.", func() {
			binding.Status.GitOpsRepoConditions = []metav1.Condition{
				{
					Status: metav1.ConditionFalse,
				},
			}

			// Create ApplicationSnapshotEnvironmentBinding CR in cluster.
			err := bindingReconciler.Create(ctx, binding)
			Expect(err).To(BeNil())

			// Trigger Reconciler
			_, err = bindingReconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())
		})
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
