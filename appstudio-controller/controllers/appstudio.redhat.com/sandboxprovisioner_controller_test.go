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

var _ = Describe("Test SandboxProvisionerController", func() {
	Context("Testing SandboxProvisionerController", func() {

		var (
			ctx        context.Context
			k8sClient  client.Client
			reconciler SandboxProvisionerReconciler
		)

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
					Name: "test-sandbox-provisioner",
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(&testNS).Build()

			reconciler = SandboxProvisionerReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}
		})

		It("should skip handling a DTC that has a DTCLS that doesn't use a Sandbox provisioner", func() {
			By("create a DTC that targets a DTCLS that exists")
			dtcls := getSandboxDeploymentTargetClass(
				func(dtcls *appstudiosharedv1.DeploymentTargetClass) {
					dtcls.Spec.Provisioner = ""
				})

			err := k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

			dtc := getDeploymentTargetClaim(
				func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnTargetProvisioner: "sandbox-provisioner",
					}
					dtc.Spec.TargetName = "random-dt"
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Pending
					dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName(dtcls.Name)
				})

			err = k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile with a pending DTC and a matching DTCLS")
			request := newRequest(dtc.Namespace, dtc.Name)
			res, err := reconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(ctrl.Result{}))

			By("find a newly created matching SpaceRequest for the DTC")
			spaceRequest, err := findMatchingSpaceRequestForDTC(ctx, k8sClient, &dtc)
			Expect(err).ToNot(HaveOccurred())
			Expect(spaceRequest).To(BeNil())
		})

		It("should return an error when handling a DTC that doesn't have a matching DTCLS", func() {
			By("create a DTC that targets a DTCLS that doesn't exist")
			missingDTCLSName := "noSuchDTCLS"
			dtc := getDeploymentTargetClaim(
				func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnTargetProvisioner: "sandbox-provisioner",
					}
					dtc.Spec.TargetName = "random-dt"
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Pending
					dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName(missingDTCLSName)
				})

			err := k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile with a pending DTC and a missing DTCLS")

			request := newRequest(dtc.Namespace, dtc.Name)
			res, err := reconciler.Reconcile(ctx, request)

			missingDTCLSErr := missingDTCLSErrWrap(dtc.Name, missingDTCLSName)
			expectedErr := missingDTCLSErr("the resource could not be found on the cluster")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(expectedErr.Error()))
			Expect(res).To(Equal(ctrl.Result{}))
		})

		It("should create a new SpaceRequest for a new deploymentTargetClaim that's marked for dynamic provisioning", func() {
			By("create a DTC that targets a DTCLS that exists")
			dtcls := getSandboxDeploymentTargetClass(
				func(dtcls *appstudiosharedv1.DeploymentTargetClass) {
				})

			err := k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

			dtc := getDeploymentTargetClaim(
				func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnTargetProvisioner: "sandbox-provisioner",
					}
					dtc.Spec.TargetName = "random-dt"
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Pending
					dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName(dtcls.Name)
				})

			err = k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile with a pending DTC and a matching DTCLS")
			request := newRequest(dtc.Namespace, dtc.Name)
			res, err := reconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(ctrl.Result{}))

			By("find a newly created matching SpaceRequest for the DTC")
			spaceRequest, err := findMatchingSpaceRequestForDTC(ctx, k8sClient, &dtc)
			Expect(err).ToNot(HaveOccurred())
			Expect(spaceRequest).ToNot(BeNil())
			Expect(spaceRequest.Labels[DeploymentTargetClaimLabel]).To(Equal(dtc.Name))
			Expect(spaceRequest.Spec.TierName).To(Equal(environmentTierName))
		})

		It("should skip creation of an existing SpaceRequest for a new deploymentTargetClaim that's marked for dynamic provisioning", func() {
			By("create a DTC that targets a DTCLS that exists")
			dtcls := getSandboxDeploymentTargetClass(
				func(dtcls *appstudiosharedv1.DeploymentTargetClass) {
				})

			err := k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

			dtc := getDeploymentTargetClaim(
				func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnTargetProvisioner: "sandbox-provisioner",
					}
					dtc.Spec.TargetName = "random-dt"
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Pending
					dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName(dtcls.Name)
				})

			err = k8sClient.Create(ctx, &dtc)
			Expect(err).ToNot(HaveOccurred())

			expected := getSandboxSpaceRequest(func(spaceRequest *codereadytoolchainv1alpha1.SpaceRequest) {
				spaceRequest.Labels = map[string]string{
					DeploymentTargetClaimLabel: dtc.Name,
				}
			})
			err = k8sClient.Create(ctx, &expected)
			Expect(err).ToNot(HaveOccurred())

			By("reconcile with a pending DTC and a matching DTCLS")
			request := newRequest(dtc.Namespace, dtc.Name)
			res, err := reconciler.Reconcile(ctx, request)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).To(Equal(ctrl.Result{}))

			By("find the already existing matching SpaceRequest for the DTC")
			spaceRequest, err := findMatchingSpaceRequestForDTC(ctx, k8sClient, &dtc)
			Expect(err).ToNot(HaveOccurred())
			Expect(spaceRequest).ToNot(BeNil())
			Expect(spaceRequest.Labels[DeploymentTargetClaimLabel]).To(Equal(dtc.Name))
			Expect(client.ObjectKeyFromObject(spaceRequest)).To(Equal(client.ObjectKeyFromObject(&expected)))
		})

		Context("Test findMatchingDTClassForDTC function", func() {
			It("should match a sandbox provisioned matching DTCLS if found", func() {
				expected := getSandboxDeploymentTargetClass(func(dt *appstudiosharedv1.DeploymentTargetClass) {
				})
				err := k8sClient.Create(ctx, &expected)
				Expect(err).ToNot(HaveOccurred())

				dtc := getSandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnTargetProvisioner: "sandbox-provisioner",
					}
					dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName(expected.Name)
				})
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dt, err := findMatchingDTClassForDTC(ctx, k8sClient, dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(client.ObjectKeyFromObject(dt)).To(Equal(client.ObjectKeyFromObject(&expected)))
			})
		})

		Context("Test findMatchingSpaceRequestForDTC function", func() {
			It("should match a sandbox provisioned matching SpaceRequest if found", func() {
				dtc := getSandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnTargetProvisioner: "sandbox-provisioner",
					}
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				expected := getSandboxSpaceRequest(func(spaceRequest *codereadytoolchainv1alpha1.SpaceRequest) {
					spaceRequest.Labels = map[string]string{
						DeploymentTargetClaimLabel: dtc.Name,
					}
				})
				err = k8sClient.Create(ctx, &expected)
				Expect(err).ToNot(HaveOccurred())

				spaceRequest, err := findMatchingSpaceRequestForDTC(ctx, k8sClient, &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(client.ObjectKeyFromObject(spaceRequest)).To(Equal(client.ObjectKeyFromObject(&expected)))
			})
		})
	})
})

func getSandboxDeploymentTargetClaim(ops ...func(dtc *appstudiosharedv1.DeploymentTargetClaim)) appstudiosharedv1.DeploymentTargetClaim {
	dtc := appstudiosharedv1.DeploymentTargetClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-dtc",
			Namespace:   "test-ns",
			Annotations: map[string]string{},
		},
	}

	for _, o := range ops {
		o(&dtc)
	}

	return dtc
}

func getSandboxDeploymentTargetClass(ops ...func(dtc *appstudiosharedv1.DeploymentTargetClass)) appstudiosharedv1.DeploymentTargetClass {
	dtcls := appstudiosharedv1.DeploymentTargetClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-sandbox-class",
			Namespace:   "test-ns",
			Annotations: map[string]string{},
		},
		Spec: appstudiosharedv1.DeploymentTargetClassSpec{
			Provisioner: appstudiosharedv1.Provisioner_Devsandbox,
		},
	}

	for _, o := range ops {
		o(&dtcls)
	}

	return dtcls
}

func getSandboxSpaceRequest(ops ...func(spaceRequest *codereadytoolchainv1alpha1.SpaceRequest)) codereadytoolchainv1alpha1.SpaceRequest {
	spaceRequest := codereadytoolchainv1alpha1.SpaceRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-sandbox-spacerequest",
			Namespace:   "test-ns",
			Annotations: map[string]string{},
		},
		Spec: codereadytoolchainv1alpha1.SpaceRequestSpec{
			TierName:           environmentTierName,
			TargetClusterRoles: []string{},
		},
	}

	for _, o := range ops {
		o(&spaceRequest)
	}

	return spaceRequest
}
