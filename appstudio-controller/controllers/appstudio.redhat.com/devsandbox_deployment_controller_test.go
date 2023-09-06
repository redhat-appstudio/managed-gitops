package appstudioredhatcom

import (
	"context"
	"os"

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

var _ = Describe("Test DevsandboxDeploymentController", func() {
	Context("Testing DevsandboxDeploymentController", func() {

		var (
			ctx        context.Context
			k8sClient  client.Client
			reconciler DevsandboxDeploymentReconciler
		)

		spaceRequestExists := func(sr codereadytoolchainv1alpha1.SpaceRequest) bool {

			srCopy := sr.DeepCopy()

			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(srCopy), srCopy)

			if err == nil {
				return true
			}

			if apierr.IsNotFound(err) {
				return false
			}

			Expect(err).ToNot(HaveOccurred())

			return false
		}

		BeforeEach(func() {
			ctx = context.Background()

			scheme,
				_,
				_,
				_,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())
			err = codereadytoolchainv1alpha1.AddToScheme(scheme)
			Expect(err).ToNot(HaveOccurred())

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
			dtc := generateDevsandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
				dtc.Annotations = map[string]string{
					appstudiosharedv1.AnnTargetProvisioner: "sandbox",
				}
				dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class")
			})
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			spacerequest := generateDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Labels = map[string]string{
					DeploymentTargetClaimLabel: dtc.Name,
				}
				spacerequest.Status.Conditions[0].Status = corev1.ConditionTrue

			})

			err = k8sClient.Create(ctx, &spacerequest)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile with a SpaceRequest with Ready condition set to True")
			request := newRequest(spacerequest.Namespace, spacerequest.Name)
			res, err := reconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(ctrl.Result{}))

			By("find a newly created matching DT for the SpaceRequest")
			dt, err := findMatchingDTForSpaceRequest(ctx, k8sClient, spacerequest)
			Expect(err).ToNot(HaveOccurred())
			Expect(dt).NotTo(BeNil())
			Expect(dt.Annotations).ToNot(BeNil())
			Expect(dt.Annotations[appstudiosharedv1.AnnDynamicallyProvisioned]).To(Equal(string(appstudiosharedv1.Provisioner_Devsandbox)))
			Expect(dt.Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify).To(BeFalse())
		})

		It("should return an error when handling a SpaceRequest that doesn't have a matching DTC", func() {
			By("create a SpaceRequest with invalid data for appstudio.openshift.io/dtc label")
			spacerequest := generateDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Labels = map[string]string{
					DeploymentTargetClaimLabel: "no-dtc-Name",
				}
				spacerequest.Status.Conditions[0].Status = corev1.ConditionTrue
			})

			err := k8sClient.Create(ctx, &spacerequest)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile with a SpaceRequest with invalid data for dtc label")
			request := newRequest(spacerequest.Namespace, spacerequest.Name)
			res, err := reconciler.Reconcile(ctx, request)
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(ctrl.Result{}))
			Expect(spaceRequestExists(spacerequest)).To(BeTrue())

		})

		It("should not return an error when handling a SpaceRequest with no dtc label set", func() {

			By("create a SpaceRequest with no appstudio.openshift.io/dtc label")
			spacerequestnolabel := generateDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Name = "test-spacerequest-nolabel"
				spacerequest.Status.Conditions[0].Status = corev1.ConditionTrue
			})

			err := k8sClient.Create(ctx, &spacerequestnolabel)
			Expect(err).ToNot(HaveOccurred())
			Expect(HasLabel(&spacerequestnolabel, DeploymentTargetClaimLabel)).NotTo(BeTrue())

			By("reconcile with a SpaceRequest with invalid data for dtc label")
			requestnolabel := newRequest(spacerequestnolabel.Namespace, spacerequestnolabel.Name)
			res, err := reconciler.Reconcile(ctx, requestnolabel)

			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(ctrl.Result{}))
			Expect(spaceRequestExists(spacerequestnolabel)).To(BeTrue())
		})

		It("Test findMatchingDTCForSpaceRequest function", func() {
			dtc := generateDevsandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
				dtc.Annotations = map[string]string{
					appstudiosharedv1.AnnTargetProvisioner: "sandbox",
				}
				dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class")
			})
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			spacerequest := generateDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Labels = map[string]string{
					DeploymentTargetClaimLabel: dtc.Name,
				}
			})
			err = k8sClient.Create(ctx, &spacerequest)
			Expect(err).ToNot(HaveOccurred())

			found, err := findMatchingDTCForSpaceRequest(ctx, k8sClient, spacerequest)
			Expect(err).ToNot(HaveOccurred())
			Expect(found.Name).To(Equal(dtc.Name))
		})

		It("should not create an extra DT if there is an existiong one for a SpaceRequest", func() {
			dtc := generateDevsandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
				dtc.Annotations = map[string]string{
					appstudiosharedv1.AnnTargetProvisioner: "sandbox",
				}
				dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class")
			})
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			spacerequest := generateDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Labels = map[string]string{
					DeploymentTargetClaimLabel: "test-dtc",
				}
			})
			err = k8sClient.Create(ctx, &spacerequest)
			Expect(err).ToNot(HaveOccurred())

			By("create a DeploymentTarget matching the SpaceRequest")
			expected := generateDevsandboxDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
				dt.Spec.ClaimRef = "test-dtc"
				dt.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class")
				dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
			})
			err = k8sClient.Create(ctx, &expected)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile with a SpaceRequest with matching deployment target")
			request := newRequest(spacerequest.Namespace, spacerequest.Name)
			res, err := reconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())

			Expect(res).To(Equal(ctrl.Result{}))
			dt, err := findMatchingDTForSpaceRequest(ctx, k8sClient, spacerequest)
			Expect(err).ToNot(HaveOccurred())
			Expect(client.ObjectKeyFromObject(dt)).To(Equal(client.ObjectKeyFromObject(&expected)))
		})

		It("should set Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify field of DT to True, if it is a Dev environment.", func() {
			dtc := generateDevsandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
				dtc.Annotations = map[string]string{
					appstudiosharedv1.AnnTargetProvisioner: "sandbox",
				}
				dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("test-sandbox-class")
			})
			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			spacerequest := generateDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Labels = map[string]string{
					DeploymentTargetClaimLabel: dtc.Name,
				}
				spacerequest.Status.Conditions[0].Status = corev1.ConditionTrue

			})

			err = k8sClient.Create(ctx, &spacerequest)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile with a SpaceRequest with Ready condition set to True")
			request := newRequest(spacerequest.Namespace, spacerequest.Name)

			By("set env variable to indicate it is a dev cluster.")

			os.Setenv("DEV_ONLY_IGNORE_SELFSIGNED_CERT_IN_DEPLOYMENT_TARGET", "true")

			// Reset env variable after test is finished
			defer os.Setenv("DEV_ONLY_IGNORE_SELFSIGNED_CERT_IN_DEPLOYMENT_TARGET", "false")

			res, err := reconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(ctrl.Result{}))

			By("find a newly created matching DT for the SpaceRequest")
			dt, err := findMatchingDTForSpaceRequest(ctx, k8sClient, spacerequest)

			Expect(err).ToNot(HaveOccurred())
			Expect(dt).NotTo(BeNil())
			Expect(dt.Spec.KubernetesClusterCredentials.AllowInsecureSkipTLSVerify).To(BeTrue())
		})

		DescribeTable("SpaceRequest is deleted when the DTC does not exist and when space request provisioner is delete type", func(classSpec appstudiosharedv1.DeploymentTargetClassSpec, shouldExist bool, expectErrorOccurred bool) {
			dtClass := appstudiosharedv1.DeploymentTargetClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "isolation-namespace",
				},
				Spec: classSpec,
			}
			Expect(k8sClient.Create(ctx, &dtClass)).To(Succeed())

			spacerequest := generateDevsandboxSpaceRequest(func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spacerequest.Labels = map[string]string{
					DeploymentTargetClaimLabel: "does-not-exist",
				}
				spacerequest.Status.Conditions[0].Status = corev1.ConditionTrue

			})

			Expect(k8sClient.Create(ctx, &spacerequest)).To(Succeed())

			res, err := reconciler.Reconcile(ctx, newRequest(spacerequest.Namespace, spacerequest.Name))

			if expectErrorOccurred {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(res).To(Equal(ctrl.Result{}))

			Expect(spaceRequestExists(spacerequest)).To(Equal(shouldExist))
		},
			Entry("valid dev sandbox provisioner with delete policy", appstudiosharedv1.DeploymentTargetClassSpec{
				Provisioner:   appstudiosharedv1.Provisioner_Devsandbox,
				Parameters:    appstudiosharedv1.DeploymentTargetParameters{},
				ReclaimPolicy: appstudiosharedv1.ReclaimPolicy_Delete,
			}, false, false),
			Entry("valid dev sandbox provisioner with retain policy", appstudiosharedv1.DeploymentTargetClassSpec{
				Provisioner:   appstudiosharedv1.Provisioner_Devsandbox,
				Parameters:    appstudiosharedv1.DeploymentTargetParameters{},
				ReclaimPolicy: appstudiosharedv1.ReclaimPolicy_Retain,
			}, true, true),
			Entry("some other provisioner with delete policy", appstudiosharedv1.DeploymentTargetClassSpec{
				Provisioner:   "some-other-provisioner",
				Parameters:    appstudiosharedv1.DeploymentTargetParameters{},
				ReclaimPolicy: appstudiosharedv1.ReclaimPolicy_Delete,
			}, true, true))

	})
})

func generateDevsandboxSpaceRequest(ops ...func(spacerequest *codereadytoolchainv1alpha1.SpaceRequest)) codereadytoolchainv1alpha1.SpaceRequest {
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

func generateDevsandboxDeploymentTargetClaim(ops ...func(dtc *appstudiosharedv1.DeploymentTargetClaim)) appstudiosharedv1.DeploymentTargetClaim {
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

func generateDevsandboxDeploymentTarget(ops ...func(dt *appstudiosharedv1.DeploymentTarget)) appstudiosharedv1.DeploymentTarget {
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
