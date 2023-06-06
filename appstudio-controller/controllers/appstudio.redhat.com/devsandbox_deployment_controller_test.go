package appstudioredhatcom

import (
	"context"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Test DevsandboxDeploymentController", func() {
	Context("Testing DevsandboxDeploymentController", func() {

		var (
			ctx        context.Context
			k8sClient  client.Client
			reconciler DevsandboxDeploymentReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()

			scheme,
				_,
				_,
				_,
				err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).To(BeNil())
			err = codereadytoolchainv1alpha1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			testNS := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-devsandboxdeployment",
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(&testNS).Build()

			reconciler = DevsandboxDeploymentReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}
		})

		It("should handle the creation of DeploymentTarget for a SpaceRequest", func() {
			dtc := getDevsandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
				dtc.Annotations = map[string]string{
					appstudiosharedv1.AnnTargetProvisioner: "sandbox",
				}
				dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class")
			})
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			spacerequest := getDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Labels = map[string]string{
					deploymentTargetClaimLabel: dtc.Name,
				}
				spacerequest.Status.Conditions[0].Status = corev1.ConditionTrue

			})

			err = k8sClient.Create(ctx, &spacerequest)
			Expect(err).To(BeNil())

			By("reconcile with a SpaceRequest with Ready condition set to True")
			request := newRequest(spacerequest.Namespace, spacerequest.Name)
			res, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())
			Expect(res).To(Equal(ctrl.Result{}))

			By("find a newly created matching DT for the SpaceRequest")
			dt, err := findMatchingDTForSpaceRequest(ctx, k8sClient, &spacerequest)
			Expect(err).To(BeNil())
			Expect(dt).NotTo(BeNil())
			Expect(dt.Annotations).ToNot(BeNil())
			Expect(dt.Annotations[annDynamicallyProvisioned]).To(Equal(string(appstudiosharedv1.Provisioner_Devsandbox)))

		})

		It("should return an error when handling a SpaceRequest that doesn't have a matching DTC", func() {
			By("create a SpaceRequest with invalid data for appstudio.openshift.io/dtc label")
			spacerequest := getDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Labels = map[string]string{
					deploymentTargetClaimLabel: "no-dtc-Name",
				}
				spacerequest.Status.Conditions[0].Status = corev1.ConditionTrue
			})

			err := k8sClient.Create(ctx, &spacerequest)
			Expect(err).To(BeNil())

			By("reconcile with a SpaceRequest with invalid data for dtc label")
			request := newRequest(spacerequest.Namespace, spacerequest.Name)
			res, err := reconciler.Reconcile(ctx, request)
			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(ctrl.Result{}))
		})

		It("should return an error when handling a SpaceRequest with no dtc label set", func() {

			By("create a SpaceRequest with no appstudio.openshift.io/dtc label")
			spacerequestnolabel := getDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Name = "test-spacerequest-nolabel"
				spacerequest.Status.Conditions[0].Status = corev1.ConditionTrue
			})

			err := k8sClient.Create(ctx, &spacerequestnolabel)
			Expect(err).To(BeNil())
			Expect(HasLabel(&spacerequestnolabel, deploymentTargetClaimLabel)).NotTo(BeTrue())

			By("reconcile with a SpaceRequest with invalid data for dtc label")
			requestnolabel := newRequest(spacerequestnolabel.Namespace, spacerequestnolabel.Name)
			res, err := reconciler.Reconcile(ctx, requestnolabel)

			Expect(err).NotTo(BeNil())
			Expect(res).To(Equal(ctrl.Result{}))
		})

		It("Test findMatchingDTCForSpaceRequest function", func() {
			dtc := getDevsandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
				dtc.Annotations = map[string]string{
					appstudiosharedv1.AnnTargetProvisioner: "sandbox",
				}
				dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class")
			})
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			spacerequest := getDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Labels = map[string]string{
					deploymentTargetClaimLabel: dtc.Name,
				}
			})
			err = k8sClient.Create(ctx, &spacerequest)
			Expect(err).To(BeNil())

			found, err := findMatchingDTCForSpaceRequest(ctx, k8sClient, &spacerequest)
			Expect(err).To(BeNil())
			Expect(found.Name == dtc.Name).To(BeTrue())
		})

		It("should not create an extra DT if there is an existiong one for a SpaceRequest", func() {
			dtc := getDevsandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
				dtc.Annotations = map[string]string{
					appstudiosharedv1.AnnTargetProvisioner: "sandbox",
				}
				dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class")
			})
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).To(BeNil())

			spacerequest := getDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Labels = map[string]string{
					deploymentTargetClaimLabel: "test-dtc",
				}
			})
			err = k8sClient.Create(ctx, &spacerequest)
			Expect(err).To(BeNil())

			By("create a DeploymentTarget matching the SpaceRequest")
			expected := getDevsandboxDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
				dt.Spec.ClaimRef = "test-dtc"
				dt.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class")
				dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
			})
			err = k8sClient.Create(ctx, &expected)
			Expect(err).To(BeNil())

			By("reconcile with a SpaceRequest with matching deployment target")
			request := newRequest(spacerequest.Namespace, spacerequest.Name)
			res, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(BeNil())

			Expect(res).To(Equal(ctrl.Result{}))
			dt, err := findMatchingDTForSpaceRequest(ctx, k8sClient, &spacerequest)
			Expect(err).To(BeNil())
			Expect(client.ObjectKeyFromObject(dt)).To(Equal(client.ObjectKeyFromObject(&expected)))
		})

	})
})

func getDevsandboxSpaceRequest(ops ...func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest)) codereadytoolchainv1alpha1.SpaceRequest {
	spacerequest := codereadytoolchainv1alpha1.SpaceRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-spacerequest",
			Namespace:   "test-deployment",
			Annotations: map[string]string{},
		},
		Spec: codereadytoolchainv1alpha1.SpaceRequestSpec{
			TierName:           "test-tiername",
			TargetClusterRoles: []string{"test-role"},
		},
		Status: codereadytoolchainv1alpha1.SpaceRequestStatus{
			TargetClusterURL: "https://api-url/test-api",
			NamespaceAccess: []codereadytoolchainv1alpha1.NamespaceAccess{
				{
					Name:      "test-ns",
					SecretRef: "test-secret",
				},
			},
			Conditions: []codereadytoolchainv1alpha1.Condition{
				{
					Type:   "Ready",
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	for _, o := range ops {
		o(&spacerequest)
	}

	return spacerequest
}

func getDevsandboxDeploymentTargetClaim(ops ...func(dtc *appstudiosharedv1.DeploymentTargetClaim)) appstudiosharedv1.DeploymentTargetClaim {
	dtc := appstudiosharedv1.DeploymentTargetClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-dtc",
			Namespace:   "test-deployment",
			Annotations: map[string]string{},
		},
		Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
			DeploymentTargetClassName: appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class"),
		},
	}

	for _, o := range ops {
		o(&dtc)
	}

	return dtc
}

func getDevsandboxDeploymentTarget(ops ...func(dt *appstudiosharedv1.DeploymentTarget)) appstudiosharedv1.DeploymentTarget {
	dt := appstudiosharedv1.DeploymentTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-dt",
			Namespace:   "test-deployment",
			Annotations: map[string]string{},
		},
		Spec: appstudiosharedv1.DeploymentTargetSpec{
			DeploymentTargetClassName: appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class"),
			KubernetesClusterCredentials: appstudiosharedv1.DeploymentTargetKubernetesClusterCredentials{
				DefaultNamespace:         "test-ns",
				APIURL:                   "https://api-url/api",
				ClusterCredentialsSecret: "test-secret",
			},
		},
	}

	for _, o := range ops {
		o(&dt)
	}

	return dt
}
