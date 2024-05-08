package appstudioredhatcom

import (
	"context"
	"fmt"

	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	appstudiosharedv1 "github.com/github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Test DeploymentTargetClaimController", func() {

	Context("Testing DeploymentTargetClaimController", func() {

		var (
			ctx        context.Context
			k8sClient  client.Client
			reconciler DeploymentTargetClaimReconciler
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

			testNS := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-dtc",
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(&testNS).Build()

			reconciler = DeploymentTargetClaimReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}
		})

		Context("Test the lifecycle of a DeploymentTargetClaim", func() {

			It("should handle the deletion of a bounded DeploymentTargetClaim with class of Retain", func() {
				dt := generateDeploymentTarget()
				err := k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.ObjectMeta.Annotations = map[string]string{
						appstudiosharedv1.AnnBindCompleted: appstudiosharedv1.AnnBinderValueTrue,
					}
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Bound
					dtc.Spec.TargetName = dt.Name
				},
				)

				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dtcls := generateDeploymentTargetClass(appstudiosharedv1.ReclaimPolicy_Retain)
				err = k8sClient.Create(ctx, &dtcls)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile with a DT and DTC that refer each other")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the status of DT and DTC is Bound")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))

				By("check if the binding controller has set the finalizer")
				finalizerFound := false
				for _, f := range dtc.GetFinalizers() {
					if f == appstudiosharedv1.FinalizerBinder {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())

				By("delete the DTC and verify if the DT is moved to Released phase")
				err = k8sClient.Delete(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeFalse())

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeTrue())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Released))

				By("verify if the claimRef of the DT is unset")
				Expect(dt.Spec.ClaimRef).Should(BeEmpty())
			})

			It("should handle the deletion of a bounded DeploymentTargetClaim with class of Delete", func() {
				dt := generateDeploymentTarget()
				err := k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.ObjectMeta.Annotations = map[string]string{
						appstudiosharedv1.AnnBindCompleted: appstudiosharedv1.AnnBinderValueTrue,
					}
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Bound
					dtc.Spec.TargetName = dt.Name
				},
				)

				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dtcls := generateDeploymentTargetClass(appstudiosharedv1.ReclaimPolicy_Delete)
				err = k8sClient.Create(ctx, &dtcls)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile with a DT and DTC that refer each other")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the status of DT and DTC is Bound")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))

				By("check if the binding controller has set the finalizer")
				finalizerFound := false
				for _, f := range dtc.GetFinalizers() {
					if f == appstudiosharedv1.FinalizerBinder {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())

				By("delete the DTC and verify if the DT is removed")
				err = k8sClient.Delete(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeFalse())

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeTrue())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(apierr.IsNotFound(err)).To(BeFalse())

				By("check if the binding controller has set the finalizer")
				finalizerFound = false
				for _, f := range dt.GetFinalizers() {
					if f == FinalizerDT {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())
			})

			It("should handle the deletion of a bounded DeploymentTargetClaim with Retain ReclaimPolicy", func() {
				dt := generateDeploymentTarget()
				err := k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.ObjectMeta.Annotations = map[string]string{
						appstudiosharedv1.AnnBindCompleted: appstudiosharedv1.AnnBinderValueTrue,
					}
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Bound
					dtc.Spec.TargetName = dt.Name
				},
				)

				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dtcls := generateDeploymentTargetClass(appstudiosharedv1.ReclaimPolicy_Retain)
				err = k8sClient.Create(ctx, &dtcls)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile with a DT and DTC that refer each other")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the status of DT and DTC is Bound")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))

				By("check if the binding controller has set the finalizer")
				finalizerFound := false
				for _, f := range dtc.GetFinalizers() {
					if f == appstudiosharedv1.FinalizerBinder {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())

				By("delete the DTC and verify if the DT is removed")
				err = k8sClient.Delete(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeFalse())

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Released))

				By("check if the binding controller has set the finalizer")
				finalizerFound = false
				for _, f := range dt.GetFinalizers() {
					if f == FinalizerDT {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeTrue())
			})

			It("should handle the deletion of a bounded DeploymentTargetClaim with deleted DeploymentTarget", func() {
				dt := generateDeploymentTarget()
				err := k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.ObjectMeta.Annotations = map[string]string{
						appstudiosharedv1.AnnBindCompleted: appstudiosharedv1.AnnBinderValueTrue,
					}
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Bound
					dtc.Spec.TargetName = dt.Name
				},
				)
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile with a DT and DTC that refer each other")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the status of DT and DTC is Bound")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))

				By("check if the binding controller has set the finalizer")
				finalizerFound := false
				for _, f := range dtc.GetFinalizers() {
					if f == appstudiosharedv1.FinalizerBinder {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())

				By("delete both DTC and DT and verify if the DTC is removed")
				err = k8sClient.Delete(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				err = k8sClient.Delete(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeTrue())
			})

			It("should handle the deletion of an unbounded DeploymentTargetClaim", func() {
				dtc := generateDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile with an unbounded DTC")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the status of DTC is Pending")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))

				By("check if the binding controller has set the finalizer")
				finalizerFound := false
				for _, f := range dtc.GetFinalizers() {
					if f == appstudiosharedv1.FinalizerBinder {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())

				By("delete the DTC and verify if it is removed")
				err = k8sClient.Delete(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeTrue())
			})
		})

		Context("Test with DeploymentTargetClaim that was previously binded by the binding controller", func() {
			It("should handle already binded DTC and DT that doesn't have the bound phase set", func() {
				By("create a DTC with binding completed annotation but in Pending phase")
				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnBindCompleted:     appstudiosharedv1.AnnBinderValueTrue,
						appstudiosharedv1.AnnBoundByController: appstudiosharedv1.AnnBinderValueTrue,
					}
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Pending
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("create a DT that claims the above DTC but not in bound phase")
				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err = k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile DT and DTC with bind-complete annotation")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the status of DT and DTC is Bound")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))
			})

			It("should return an error and set the DTC status to Lost if DT is not found", func() {
				By("create a DTC that targets a DT that doesn't exist")
				dtc := generateDeploymentTargetClaim(
					func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
						dtc.Annotations = map[string]string{
							appstudiosharedv1.AnnBindCompleted: appstudiosharedv1.AnnBinderValueTrue,
						}
						dtc.Spec.TargetName = "random-dt"
						dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Bound
					})

				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile with a bounded DT and DTC")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(HaveOccurred())
				expectedErr := fmt.Errorf("DeploymentTarget not found for a bounded DeploymentTargetClaim %s in namespace %s", dtc.Name, dtc.Namespace)
				Expect(err.Error()).Should(Equal(expectedErr.Error()))
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the DTC is in Lost phase")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Lost))
			})

			It("should return an error and set the DTC status to Lost if no matching DT is found", func() {
				By("creates DTC with bounded annotation")
				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnBindCompleted: appstudiosharedv1.AnnBinderValueTrue,
					}
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Bound
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("create multiple DTs that doesn't match the given DTC")
				for i := 0; i < 3; i++ {
					dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
						dt.Name = fmt.Sprintf("test-dt-%d", i)
						dt.Spec.ClaimRef = fmt.Sprintf("test-dtc-%d", i)
						dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
					})
					err = k8sClient.Create(ctx, &dt)
					Expect(err).ToNot(HaveOccurred())
				}

				By("reconcile with a bounded DTC and multiple DTs that are claiming other DTCs")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(HaveOccurred())
				expectedErr := fmt.Errorf("DeploymentTarget not found for a bounded DeploymentTargetClaim %s in namespace %s", dtc.Name, dtc.Namespace)
				Expect(err.Error()).Should(Equal(expectedErr.Error()))
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the DTC is in Lost phase")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Lost))
			})

			It("should update the provisioner annotation if it doesn't match the class name", func() {
				By("create a bounded DT and DTC with provisioner annotation that doesn't match the class name")
				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnBindCompleted:     appstudiosharedv1.AnnBinderValueTrue,
						appstudiosharedv1.AnnTargetProvisioner: "provisioner-doesn't-exist",
					}
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
				})
				err = k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile and check if the provisioner annotation is updated")
				req := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())

				Expect(dtc.Annotations[appstudiosharedv1.AnnTargetProvisioner]).To(Equal(string(dtc.Spec.DeploymentTargetClassName)))

				By("check if the status of DT and DTC is Bound")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))

			})

			It("should remove the provisioner annotation if the class name is not set", func() {
				By("create a bounded DT and DTC with provisioner annotation")
				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.DeploymentTargetClassName = ""
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnBindCompleted:     appstudiosharedv1.AnnBinderValueTrue,
						appstudiosharedv1.AnnTargetProvisioner: "provisioner-doesn't-exist",
					}
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
				})
				err = k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile and check if the provisioner annotation is removed")
				req := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())

				_, found := dtc.Annotations[appstudiosharedv1.AnnTargetProvisioner]
				Expect(found).To(BeFalse())

				By("check if the status of DT and DTC is Bound")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))

			})
		})

		Context("Test DeploymentTargetClaim with no target and the binding controller finds a matching target", func() {
			It("should mark the DTC for dynamic provisioning if a matching DT is not found", func() {
				By("create a DTC without specifying the DT")
				dtc := generateDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile and check if it is marked for dynamic provisioning")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if the provisioner annotation is set and the DTC status is set to pending phase")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Annotations).ShouldNot(BeNil())
				Expect(dtc.Annotations[appstudiosharedv1.AnnTargetProvisioner]).To(Equal(string(dtc.Spec.DeploymentTargetClassName)))
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))

				By("verify if the bound-by-controller annotation is not set")
				Expect(dtc.Annotations[appstudiosharedv1.AnnBoundByController]).ToNot(Equal(string(appstudiosharedv1.AnnBinderValueTrue)))
			})

			It("shouldn't mark the DTC for dynamic provisioning if a matching DT is found", func() {
				By("create a DTC without specifying the DT")
				dtc := generateDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("create a DT that is available and claims the above DTC")
				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err = k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile a DTC and verify that it isn't marked for dynamic provisioning")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if both DTC and DT are bound together")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))

				By("check if the provisioner annotation is not set")
				Expect(dtc.Annotations).ShouldNot(BeNil())
				_, found := dtc.Annotations[appstudiosharedv1.AnnTargetProvisioner]
				Expect(found).To(BeFalse())

				By("check if the bound-by-controller annotation is set")
				val, found := dtc.Annotations[appstudiosharedv1.AnnBoundByController]
				Expect(found).To(BeTrue())
				Expect(val).To(Equal(appstudiosharedv1.AnnBinderValueTrue))
			})

			It("should mark the DTC as pending if the DT isn't found and DTClass is not set", func() {
				By("create a DTC without specifying the DeploymentTargetClass")
				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.DeploymentTargetClassName = ""
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile a DTC and verify that it isn't marked for dynamic provisioning")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if the DTC is set to pending phase")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))
				// check if the provision annotation is not set
				Expect(dtc.Annotations).Should(BeNil())
			})
		})

		Context("Test DeploymentTargetClaim with a user provided DeploymentTarget", func() {
			It("should return an error and requeue if DT pointed by DTC is not found", func() {
				By("create a DTC that points to a DT that doesn't exist")
				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = "random"
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile and verify if it returns an error with requeue")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if the DTC is set to pending phase")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))
			})

			It("should bind if the DT is found and if it refers back to the DTC", func() {
				By("create a DTC and DT that refer each other")
				dtc := generateDeploymentTargetClaim()

				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err := k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				dtc.Spec.TargetName = dt.Name
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile and verify if it returns an error with requeue")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if both DTC and DT are bound together")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Annotations).ShouldNot(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))
			})

			It("should bind if the target DT is not claimed by anyone", func() {
				By("create a DT and a DTC that claims it")
				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					// DT is available for claiming
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err := k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = dt.Name
				})
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile and verify if it returns an error with requeue")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if both DTC and DT are bound together")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Annotations).ShouldNot(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))
			})

			It("shouldn't bind if the DT is already claimed", func() {
				By("create a DTC and DT that is already claimed")
				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = "random-dtc"
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Bound
				})
				err := k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = dt.Name
				})
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile and verify if the DTC is not bound")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if DTC has pending state")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))
			})

			It("should return an error if the DT doesn't match the DTC", func() {
				By("create a DTC and DT that refer each other but belong to different classes")
				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err := k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = dt.Name
					dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("random")
				})
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				By("reconcile and verify if it returns an error with requeue")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(HaveOccurred())
				expectedErr := fmt.Errorf("DeploymentTarget %s does not match DeploymentTargetClaim %s in namespace %s: deploymentTargetClassName does not match", dt.Name, dtc.Name, dtc.Namespace)
				Expect(err.Error()).To(Equal(expectedErr.Error()))
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		Context("Test GetBoundByDTC function", func() {
			It("get the DT specified as a target in the DTC", func() {
				dt := generateDeploymentTarget()
				err := k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = dt.Name
				})
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				boundedDT, err := getDTBoundByDTC(ctx, k8sClient, dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(boundedDT).ToNot(BeNil())
				Expect(client.ObjectKeyFromObject(boundedDT)).To(Equal(client.ObjectKeyFromObject(&dt)))
			})

			It("shouldn't return a DT if it absent", func() {
				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = "random-dt"
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				boundedDT, err := getDTBoundByDTC(ctx, k8sClient, dtc)
				Expect(apierr.IsNotFound(err)).To(BeTrue())
				Expect(boundedDT).To(BeNil())
			})

			It("get the DT that refers the DTC in a claim ref", func() {
				dtc := generateDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
				})
				err = k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				// create another DT that doesn't refer the DTC
				fakedt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Name = "fake-dt"
				})
				err = k8sClient.Create(ctx, &fakedt)
				Expect(err).ToNot(HaveOccurred())

				boundedDT, err := getDTBoundByDTC(ctx, k8sClient, dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(boundedDT).ToNot(BeNil())
				Expect(client.ObjectKeyFromObject(boundedDT)).To(Equal(client.ObjectKeyFromObject(&dt)))
			})

			It("should return an error if there are multiple DTs associated with a DTC", func() {
				dtc := generateDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dt1 := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Name = "test-dt-1"
					dt.Spec.ClaimRef = dtc.Name
				})
				err = k8sClient.Create(ctx, &dt1)
				Expect(err).ToNot(HaveOccurred())

				By("create a second DT that claims the same DTC")
				dt2 := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Name = "test-dt-2"
					dt.Spec.ClaimRef = dtc.Name
				})
				err = k8sClient.Create(ctx, &dt2)
				Expect(err).ToNot(HaveOccurred())

				By("check if an error is a second DT is detected and an error is returned")
				dt, err := getDTBoundByDTC(ctx, k8sClient, dtc)
				Expect(dt).To(BeNil())
				Expect(err).To(HaveOccurred())
				expectedErr := fmt.Errorf("multiple DeploymentTargets found for a bounded DeploymentTargetClaim %s", dtc.Name)
				Expect(err.Error()).To(Equal(expectedErr.Error()))

			})
		})

		Context("Test doesDTMatchDTC function", func() {
			var (
				dtc         appstudiosharedv1.DeploymentTargetClaim
				dt          appstudiosharedv1.DeploymentTarget
				mismatchErr func(string) error
				conflictErr func(string) error
			)
			BeforeEach(func() {
				dtc = generateDeploymentTargetClaim()
				dt = generateDeploymentTarget()
				mismatchErr = mismatchErrWrap(dt.Name, dtc.Name, dtc.Namespace)
				conflictErr = conflictErrWrapper(dtc.Name, dtc.Spec.TargetName, dt.Name, dt.Spec.ClaimRef)
			})

			It("DT and DTC match", func() {
				dt.Spec.ClaimRef = dtc.Name
				dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available

				err := doesDTMatchDTC(dt, dtc)
				Expect(err).ToNot(HaveOccurred())

				// check if it still matches without phase
				dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available

				err = doesDTMatchDTC(dt, dtc)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return an error if the classes don't match", func() {
				dt.Spec.ClaimRef = dtc.Name
				dt.Spec.DeploymentTargetClassName = "different-class"
				dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available

				expecterErr := mismatchErr("deploymentTargetClassName does not match")
				err := doesDTMatchDTC(dt, dtc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(expecterErr.Error()))
			})

			It("should return an error if DT is not in available phase", func() {
				dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Bound

				expecterErr := mismatchErr("DeploymentTarget is not in Available phase")
				err := doesDTMatchDTC(dt, dtc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(expecterErr.Error()))
			})

			It("should return an error if DT doesn't have cluter credentials", func() {
				dt.Spec.ClaimRef = dtc.Name
				dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				dt.Spec.KubernetesClusterCredentials = appstudiosharedv1.DeploymentTargetKubernetesClusterCredentials{}

				expecterErr := mismatchErr("DeploymentTarget does not have cluster credentials")
				err := doesDTMatchDTC(dt, dtc)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(expecterErr.Error()))
			})

			It("should return an error if there is a binding conflict", func() {
				dt.Spec.ClaimRef = dtc.Name
				dtc.Spec.TargetName = "different-dt"
				dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available

				err := doesDTMatchDTC(dt, dtc)
				conflictErr = conflictErrWrapper(dtc.Name, dtc.Spec.TargetName, dt.Name, dt.Spec.ClaimRef)
				expecterErr := conflictErr("DeploymentTargetClaim targets a DeploymentTarget that is already claimed")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(expecterErr.Error()))
			})

		})

		Context("Test checkForBindingConflict function", func() {
			It("should return an error if DTC targets a DT that has a claimRef to a different DTC.", func() {
				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = "random-dtc"
				})

				dtc := generateDeploymentTargetClaim(
					func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
						dtc.Spec.TargetName = dt.Name
					},
				)

				err := checkForBindingConflict(dtc, dt)
				Expect(err).To(HaveOccurred())
				errWrap := conflictErrWrapper(dtc.Name, dtc.Spec.TargetName, dt.Name, dt.Spec.ClaimRef)
				expectedErr := errWrap("DeploymentTarget has a claimRef to another DeploymentTargetClaim")
				Expect(err.Error()).To(Equal(expectedErr.Error()))
			})

			It("should return an error if DTC has empty target but DT has a claimRef to a different DTC.", func() {
				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = "random-dtc"
				})
				dtc := generateDeploymentTargetClaim()

				err := checkForBindingConflict(dtc, dt)
				Expect(err).To(HaveOccurred())
				errWrap := conflictErrWrapper(dtc.Name, dtc.Spec.TargetName, dt.Name, dt.Spec.ClaimRef)
				expectedErr := errWrap("DeploymentTarget has a claimRef to another DeploymentTargetClaim")
				Expect(err.Error()).To(Equal(expectedErr.Error()))
			})

			It("should return an error if DT has a claim ref to a DTC but the DTC targets a different DT", func() {
				dtc := generateDeploymentTargetClaim(
					func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
						dtc.Spec.TargetName = "random-dt"
					},
				)
				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
				})

				err := checkForBindingConflict(dtc, dt)
				Expect(err).To(HaveOccurred())
				errWrap := conflictErrWrapper(dtc.Name, dtc.Spec.TargetName, dt.Name, dt.Spec.ClaimRef)
				expectedErr := errWrap("DeploymentTargetClaim targets a DeploymentTarget that is already claimed")
				Expect(err.Error()).To(Equal(expectedErr.Error()))
			})

			It("should return an error if DT has empty claim ref but DTC targets a different DT.", func() {
				dtc := generateDeploymentTargetClaim(
					func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
						dtc.Spec.TargetName = "random-dt"
					},
				)
				dt := generateDeploymentTarget()

				err := checkForBindingConflict(dtc, dt)
				errWrap := conflictErrWrapper(dtc.Name, dtc.Spec.TargetName, dt.Name, dt.Spec.ClaimRef)
				expectedErr := errWrap("DeploymentTargetClaim targets a DeploymentTarget that is already claimed")
				Expect(err.Error()).To(Equal(expectedErr.Error()))
			})

			It("should not return an error if there are no conflicts", func() {
				dtc := generateDeploymentTargetClaim()
				dt := generateDeploymentTarget()

				err := checkForBindingConflict(dtc, dt)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Test findMatchingDTForDTC function", func() {
			It("should match a dynamically provisioned matching DT if found", func() {
				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnTargetProvisioner: "sandbox-provisioner",
					}
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				expected := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err = k8sClient.Create(ctx, &expected)
				Expect(err).ToNot(HaveOccurred())

				dt, err := findMatchingDTForDTC(ctx, k8sClient, dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(client.ObjectKeyFromObject(dt)).To(Equal(client.ObjectKeyFromObject(&expected)))
			})

			It("should match a user created matching DT if found", func() {
				dtc := generateDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				expected := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err = k8sClient.Create(ctx, &expected)
				Expect(err).ToNot(HaveOccurred())

				dt, err := findMatchingDTForDTC(ctx, k8sClient, dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(client.ObjectKeyFromObject(dt)).To(Equal(client.ObjectKeyFromObject(&expected)))
			})

			It("no matching DT is found", func() {
				dtc := generateDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dt, err := findMatchingDTForDTC(ctx, k8sClient, dtc)
				Expect(err).ToNot(HaveOccurred())
				Expect(dt).To(BeNil())
			})
		})

		Context("Test bindDeploymentTargetCliamToTarget function", func() {
			var (
				dt     appstudiosharedv1.DeploymentTarget
				dtc    appstudiosharedv1.DeploymentTargetClaim
				logger logr.Logger
			)

			BeforeEach(func() {
				dtc = generateDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())
				dt = generateDeploymentTarget()
				err = k8sClient.Create(ctx, &dt)
				Expect(err).ToNot(HaveOccurred())

				logger = log.FromContext(ctx)
			})

			It("should bind DT and DTC with bound-by-controller annotation", func() {
				err := bindDeploymentTargetClaimToTarget(ctx, k8sClient, &dtc, &dt, true, logger)
				Expect(err).ToNot(HaveOccurred())

				// verify if bound complete and bound-by-controller annotations are set
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())

				Expect(isDTCBindingCompleted(dtc)).To(BeTrue())
				Expect(dtc.Annotations[appstudiosharedv1.AnnBoundByController]).To(Equal(appstudiosharedv1.AnnBinderValueTrue))
			})

			It("should bind DT and DTC without bound-by-controller annotation", func() {
				err := bindDeploymentTargetClaimToTarget(ctx, k8sClient, &dtc, &dt, false, logger)
				Expect(err).ToNot(HaveOccurred())

				// verify if bound complete and bound-by-controller annotations are set
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).ToNot(HaveOccurred())

				Expect(isDTCBindingCompleted(dtc)).To(BeTrue())
				_, found := dtc.Annotations[appstudiosharedv1.AnnBoundByController]
				Expect(found).To(BeFalse())
			})
		})

		Context("Test findObjectsForDeploymentTarget function", func() {
			It("shouldn't return requests if the incoming DT didn't match any DTC", func() {
				dtc := generateDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dt := generateDeploymentTarget()

				reqs := reconciler.findObjectsForDeploymentTarget(&dt)
				Expect(reqs).To(Equal([]reconcile.Request{}))
			})

			It("should return a DTC request if the incoming DT claims it", func() {
				dtc := generateDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				dt := generateDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
				})

				reqs := reconciler.findObjectsForDeploymentTarget(&dt)
				Expect(reqs).To(Equal([]reconcile.Request{
					newRequest(dtc.Namespace, dtc.Name),
				}))
			})

			It("should return a DTC request if it targets the incoming DT", func() {
				dt := generateDeploymentTarget()

				dtc := generateDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = dt.Name
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				reqs := reconciler.findObjectsForDeploymentTarget(&dt)
				Expect(reqs).To(Equal([]reconcile.Request{
					newRequest(dtc.Namespace, dtc.Name),
				}))
			})

			It("shouldn't return any request if no DTC was found while handling the DT event", func() {
				dt := generateDeploymentTarget()

				reqs := reconciler.findObjectsForDeploymentTarget(&dt)
				Expect(reqs).To(Equal([]reconcile.Request{}))
			})

			It("shouldn't return any request if an object of different type is passed", func() {
				dtc := generateDeploymentTargetClaim()

				reqs := reconciler.findObjectsForDeploymentTarget(&dtc)
				Expect(reqs).To(Equal([]reconcile.Request{}))
			})
		})

	})

	Context("Testing creation of SpaceRequest for dynamic DTC", func() {

		var (
			ctx        context.Context
			k8sClient  client.Client
			reconciler DeploymentTargetClaimReconciler
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

			reconciler = DeploymentTargetClaimReconciler{
				Client: k8sClient,
				Scheme: scheme,
			}
		})

		It("should skip handling a DTC that has a DTCLS that doesn't use a Sandbox provisioner", func() {
			By("create a DTC that targets a DTCLS that exists")
			dtcls := generateDeploymentTargetClass(appstudiosharedv1.ReclaimPolicy_Retain,
				func(dtcls *appstudiosharedv1.DeploymentTargetClass) {
					dtcls.Spec.Provisioner = ""
				})

			err := k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

			dtc := generateDeploymentTargetClaim(
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
			dtc := generateDeploymentTargetClaim(
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

			dtcls := generateDeploymentTargetClass(appstudiosharedv1.ReclaimPolicy_Retain)

			err := k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

			dtc := generateDeploymentTargetClaim(
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
			dtcls := generateDeploymentTargetClass(appstudiosharedv1.ReclaimPolicy_Retain)

			err := k8sClient.Create(ctx, &dtcls)
			Expect(err).ToNot(HaveOccurred())

			dtc := generateDeploymentTargetClaim(
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

			expected := generateSandboxSpaceRequest(func(spaceRequest *codereadytoolchainv1alpha1.SpaceRequest) {
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
				expected := generateDeploymentTargetClass(appstudiosharedv1.ReclaimPolicy_Retain)

				err := k8sClient.Create(ctx, &expected)
				Expect(err).ToNot(HaveOccurred())

				dtc := generateSandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
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
				dtc := generateSandboxDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						appstudiosharedv1.AnnTargetProvisioner: "sandbox-provisioner",
					}
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).ToNot(HaveOccurred())

				expected := generateSandboxSpaceRequest(func(spaceRequest *codereadytoolchainv1alpha1.SpaceRequest) {
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

func generateDeploymentTargetClaim(ops ...func(dtc *appstudiosharedv1.DeploymentTargetClaim)) appstudiosharedv1.DeploymentTargetClaim {
	dtc := appstudiosharedv1.DeploymentTargetClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-dtc",
			Namespace:   "test-ns",
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

func generateDeploymentTarget(ops ...func(dt *appstudiosharedv1.DeploymentTarget)) appstudiosharedv1.DeploymentTarget {
	dt := appstudiosharedv1.DeploymentTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-dt",
			Namespace:   "test-ns",
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

func generateDeploymentTargetClass(reclaimPolicy appstudiosharedv1.ReclaimPolicy, ops ...func(dtc *appstudiosharedv1.DeploymentTargetClass)) appstudiosharedv1.DeploymentTargetClass {
	dtcls := appstudiosharedv1.DeploymentTargetClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-sandbox-class",
			Annotations: map[string]string{},
		},
		Spec: appstudiosharedv1.DeploymentTargetClassSpec{
			Provisioner:   appstudiosharedv1.Provisioner_Devsandbox,
			ReclaimPolicy: reclaimPolicy,
		},
	}

	for _, o := range ops {
		o(&dtcls)
	}

	return dtcls
}

func generateSandboxDeploymentTargetClaim(ops ...func(dtc *appstudiosharedv1.DeploymentTargetClaim)) appstudiosharedv1.DeploymentTargetClaim {
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

func generateSandboxSpaceRequest(ops ...func(spaceRequest *codereadytoolchainv1alpha1.SpaceRequest)) codereadytoolchainv1alpha1.SpaceRequest {
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
