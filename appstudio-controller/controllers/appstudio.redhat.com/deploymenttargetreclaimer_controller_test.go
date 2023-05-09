package appstudioredhatcom

import (
	"context"
	"time"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Test DeploymentTargetReclaimController", func() {
	Context("Testing DeploymentTargetReclaimController", func() {

		var (
			ctx        context.Context
			k8sClient  client.Client
			reconciler DeploymentTargetReconciler
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
					Name: "test-namespace",
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(&testNS).Build()

			reconciler = DeploymentTargetReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}
		})

		Context("Test the reclaim of a DeploymentTarget", func() {

			It("Failed to delete SpaceRequest and DeploymentTarget", func() {

				By("creating a default DeploymentTargetClass")
				dtcls := generateDeploymentTargetClass(func(dtc *appstudiosharedv1.DeploymentTargetClass) {
					dtc.Spec.ReclaimPolicy = appstudiosharedv1.ReclaimPolicy_Delete
				})
				err := k8sClient.Create(ctx, &dtcls)
				Expect(err).To(BeNil())

				By("creating a default DeploymentTarget")
				dt := generateReclaimDeploymentTarget()
				err = k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				By("creating a SpaceRequest")
				sr := generateReclaimSpaceRequest(func(sr *codereadytoolchainv1alpha1.SpaceRequest) {
					sr.Spec.TierName = "appstudio-env"
					sr.Status.Conditions[0].Status = corev1.ConditionFalse
				})
				err = k8sClient.Create(ctx, &sr)
				Expect(err).To(BeNil())

				By("reconcile with a DeploymentTarget")
				request := newRequest(dt.Namespace, dt.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the DT has set the finalizer")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).To(BeNil())
				finalizerFound := false
				for _, f := range dt.GetFinalizers() {
					if f == FinalizerDT {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())

				By("deleting the DT")
				err = k8sClient.Delete(ctx, &dt)
				Expect(err).To(BeNil())

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{Requeue: true}), "should requeue after deleting the SpaceRequest")

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{RequeueAfter: 30 * time.Second}),
					"should requeue because the SpaceRequest hasn't been cleaned up yet")

				err = k8sClient.Delete(ctx, &sr)
				Expect(err).To(BeNil())

				// TODO: GITOPSRVCE-576 - Replace this with mocked time
				time.Sleep(2 * time.Minute)

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).To(BeNil())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Failed))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&sr), &sr)
				Expect(apierr.IsNotFound(err)).To(BeFalse())
			})
		})
	})
})

func generateReclaimSpaceRequest(ops ...func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest)) codereadytoolchainv1alpha1.SpaceRequest {
	spacerequest := codereadytoolchainv1alpha1.SpaceRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-spacerequest",
			Namespace: "test-deployment",
			Labels: map[string]string{
				"appstudio.openshift.io/dtc": "test-dtc",
			},
		},
		Spec: codereadytoolchainv1alpha1.SpaceRequestSpec{
			TierName:           "appstudio-env",
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
					Status: corev1.ConditionTrue,
					Reason: "UnableToTerminate",
				},
			},
		},
	}
	for _, o := range ops {
		o(&spacerequest)
	}

	return spacerequest
}

func generateReclaimDeploymentTarget(ops ...func(dt *appstudiosharedv1.DeploymentTarget)) appstudiosharedv1.DeploymentTarget {
	dt := appstudiosharedv1.DeploymentTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-dt",
			Namespace:   "test-deployment",
			Annotations: map[string]string{},
		},
		Spec: appstudiosharedv1.DeploymentTargetSpec{
			ClaimRef:                  "test-dtc",
			DeploymentTargetClassName: appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class"),
			KubernetesClusterCredentials: appstudiosharedv1.DeploymentTargetKubernetesClusterCredentials{
				DefaultNamespace:         "test-ns",
				APIURL:                   "https://api-url/api",
				ClusterCredentialsSecret: "test-secret",
			},
		},
		Status: appstudiosharedv1.DeploymentTargetStatus{
			Phase: appstudiosharedv1.DeploymentTargetPhase_Bound,
		},
	}

	for _, o := range ops {
		o(&dt)
	}

	return dt
}
