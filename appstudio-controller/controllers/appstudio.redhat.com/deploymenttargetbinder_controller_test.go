package appstudioredhatcom

import (
	"context"
	"fmt"

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

var _ = Describe("Test DeploymentTargetClaimBinderController", func() {
	Context("Testing DeploymentTargetClaimBinderController", func() {

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
			Expect(err).To(BeNil())

			err = appstudiosharedv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

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
			It("should handle the deletion of a bounded DeploymentTargetClaim", func() {
				dt := getDeploymentTarget()
				err := k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				dtc := getDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.ObjectMeta.Annotations = map[string]string{
						annBindCompleted: annBinderValueYes,
					}
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Bound
					dtc.Spec.TargetName = dt.Name
				},
				)
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile with a DT and DTC that refer each other")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the status of DT and DTC is Bound")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).To(BeNil())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))

				By("check if the binding controller has set the finalizer")
				finalizerFound := false
				for _, f := range dtc.GetFinalizers() {
					if f == finalizerBinder {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())

				By("delete the DTC and verify if the DT is moved to Released phase")
				err = k8sClient.Delete(ctx, &dtc)
				Expect(err).To(BeNil())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeFalse())

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeTrue())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).To(BeNil())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Released))
			})

			It("should handle the deletion of a bounded DeploymentTargetClaim with deleted DeploymentTarget", func() {
				dt := getDeploymentTarget()
				err := k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				dtc := getDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.ObjectMeta.Annotations = map[string]string{
						annBindCompleted: annBinderValueYes,
					}
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Bound
					dtc.Spec.TargetName = dt.Name
				},
				)
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile with a DT and DTC that refer each other")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the status of DT and DTC is Bound")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).To(BeNil())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))

				By("check if the binding controller has set the finalizer")
				finalizerFound := false
				for _, f := range dtc.GetFinalizers() {
					if f == finalizerBinder {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())

				By("delete both DTC and DT and verify if the DTC is removed")
				err = k8sClient.Delete(ctx, &dtc)
				Expect(err).To(BeNil())

				err = k8sClient.Delete(ctx, &dt)
				Expect(err).To(BeNil())

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeTrue())
			})

			It("should handle the deletion of an unbounded DeploymentTargetClaim", func() {
				dtc := getDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile with an unbounded DTC")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{
					Requeue:      true,
					RequeueAfter: binderRequeueDuration,
				}))

				By("check if the status of DTC is Pending")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))

				By("check if the binding controller has set the finalizer")
				finalizerFound := false
				for _, f := range dtc.GetFinalizers() {
					if f == finalizerBinder {
						finalizerFound = true
						break
					}
				}
				Expect(finalizerFound).To(BeTrue())

				By("delete the DTC and verify if it is removed")
				err = k8sClient.Delete(ctx, &dtc)
				Expect(err).To(BeNil())

				res, err = reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(apierr.IsNotFound(err)).To(BeTrue())
			})
		})

		Context("Test with DeploymentTargetClaim that was previously binded by the binding controller", func() {
			It("should handle already bounded DTC and DT", func() {
				By("create a DTC with binding completed annotation but in Pending phase")
				dtc := getDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{annBindCompleted: annBinderValueYes}
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Pending
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				dt := getDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err = k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				By("reconcile with a bounded DT and DTC")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the status of DT and DTC is Bound")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).To(BeNil())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))
			})

			It("should return an error and set the DTC status to Lost if DT is not found", func() {
				By("create a DTC that targets a DT that doesn't exist")
				dtc := getDeploymentTargetClaim(
					func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
						dtc.Annotations = map[string]string{
							annBindCompleted: annBinderValueYes,
						}
						dtc.Spec.TargetName = "random-dt"
						dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Bound
					})

				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile with a bounded DT and DTC")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).NotTo(BeNil())
				expectedErr := fmt.Errorf("DeploymentTarget not found for bounded DeploymentTargetClaim %s in namespace %s", dtc.Name, dtc.Namespace)
				Expect(err.Error()).Should(Equal(expectedErr.Error()))
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the DTC is in Lost phase")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Lost))
			})

			It("should return an error and set the DTC status to Lost if no matching DT is found", func() {
				By("creates DTC with bounded annotation")
				dtc := getDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Annotations = map[string]string{
						annBindCompleted: annBinderValueYes,
					}
					dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Bound
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("create multiple DTs that doesn't match the given DTC")
				for i := 0; i < 3; i++ {
					dt := getDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
						dt.Name = fmt.Sprintf("test-dt-%d", i)
						dt.Spec.ClaimRef = fmt.Sprintf("test-dtc-%d", i)
						dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
					})
					err = k8sClient.Create(ctx, &dt)
					Expect(err).To(BeNil())
				}

				By("reconcile with a bounded DTC and multiple DTs that are claiming other DTCs")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).NotTo(BeNil())
				expectedErr := fmt.Errorf("DeploymentTarget not found for bounded DeploymentTargetClaim %s in namespace %s", dtc.Name, dtc.Namespace)
				Expect(err.Error()).Should(Equal(expectedErr.Error()))
				Expect(res).To(Equal(ctrl.Result{}))

				By("check if the DTC is in Lost phase")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Lost))
			})
		})

		Context("Test DeploymentTargetClaim with no target and the binding controller finds a matching target", func() {
			It("should mark the DTC for dynamic provisioning if a matching DT is not found", func() {
				By("create a DTC without specifying the DT")
				dtc := getDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile and check if it is marked for dynamic provisioning")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{
					Requeue:      true,
					RequeueAfter: binderRequeueDuration,
				}))

				By("verify if the provisioner annotation is set and the DTC status is set to pending phase")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Annotations).ShouldNot(BeNil())
				Expect(dtc.Annotations[annTargetProvisioner]).To(Equal(string(dtc.Spec.DeploymentTargetClassName)))
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))

				By("verify if the bound-by-controller annotation is not set")
				Expect(dtc.Annotations[annBoundByController]).ToNot(Equal(string(annBoundByController)))
			})

			It("shouldn't mark the DTC for dynamic provisioning if a matching DT is found", func() {
				By("create a DTC without specifying the DT")
				dtc := getDeploymentTargetClaim()
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("create a DT that is available and claims the above DTC")
				dt := getDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err = k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				By("reconcile a DTC and verify that it isn't marked for dynamic provisioning")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if both DTC and DT are bound together")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).To(BeNil())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))

				By("check if the provisioner annotation is not set")
				Expect(dtc.Annotations).ShouldNot(BeNil())
				_, found := dtc.Annotations[annTargetProvisioner]
				Expect(found).To(BeFalse())

				By("check if the bound-by-controller annotation is set")
				val, found := dtc.Annotations[annBoundByController]
				Expect(found).To(BeTrue())
				Expect(val).To(Equal(annBinderValueYes))
			})

			It("should mark the DTC as pending if the DT isn't found and DTClass is not set", func() {
				By("create a DTC without specifying the DeploymentTargetClass")
				dtc := getDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.DeploymentTargetClassName = ""
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile a DTC and verify that it isn't marked for dynamic provisioning")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{
					Requeue:      true,
					RequeueAfter: binderRequeueDuration,
				}))

				By("verify if the DTC is set to pending phase")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))
				// check if the provision annotation is not set
				Expect(dtc.Annotations).Should(BeNil())
			})
		})

		Context("Test DeploymentTargetClaim with a user provided DeploymentTarget", func() {
			It("should return an error and requeue if DT pointed by DTC is not found", func() {
				By("create a DTC that points to a DT that doesn't exist")
				dtc := getDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = "random"
				})
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile and verify if it returns an error with requeue")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{
					Requeue:      true,
					RequeueAfter: binderRequeueDuration,
				}))

				By("verify if the DTC is set to pending phase")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))
			})

			It("should bind if the DT is found and if it refers back to the DTC", func() {
				By("create a DTC and DT that refer each other")
				dtc := getDeploymentTargetClaim()

				dt := getDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = dtc.Name
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err := k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				dtc.Spec.TargetName = dt.Name
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile and verify if it returns an error with requeue")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if both DTC and DT are bound together")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Annotations).ShouldNot(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).To(BeNil())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))
			})

			It("should bind if the target DT is not claimed by anyone", func() {
				By("create a DT and a DTC that claims it")
				dt := getDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					// DT is available for claiming
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err := k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				dtc := getDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = dt.Name
				})
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile and verify if it returns an error with requeue")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if both DTC and DT are bound together")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Annotations).ShouldNot(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).To(BeNil())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))
			})

			It("should return an error if the DT is already claimed", func() {
				By("create a DTC and DT that is already claimed")
				dt := getDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Spec.ClaimRef = "random-dtc"
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Bound
				})
				err := k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				dtc := getDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = dt.Name
				})
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile and verify if it returns an error with requeue")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(BeNil())
				alreadyClaimedErr := fmt.Errorf("DeploymentTargetClaim %s wants to claim DeploymentTarget %s that is already claimed in namespace %s", dtc.Name, dt.Name, dtc.Namespace)
				Expect(err.Error()).To(Equal(alreadyClaimedErr.Error()))
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if DTC has pending state")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))
			})

			It("should return an error if the DT doesn't match the DTC", func() {
				By("create a DTC and DT that refer each other but belong to different classes")
				dt := getDeploymentTarget(func(dt *appstudiosharedv1.DeploymentTarget) {
					dt.Status.Phase = appstudiosharedv1.DeploymentTargetPhase_Available
				})
				err := k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				dtc := getDeploymentTargetClaim(func(dtc *appstudiosharedv1.DeploymentTargetClaim) {
					dtc.Spec.TargetName = dt.Name
					dtc.Spec.DeploymentTargetClassName = appstudiosharedv1.DeploymentTargetClassName("random")
				})
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile and verify if it returns an error with requeue")
				request := newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(BeNil())
				expectedErr := fmt.Errorf("DeploymentTarget %s does not match DeploymentTargetClaim %s in namespace %s: deploymentTargetClassName does not match", dt.Name, dtc.Name, dtc.Namespace)
				Expect(err.Error()).To(Equal(expectedErr.Error()))
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})
	})
})

func getDeploymentTargetClaim(ops ...func(dtc *appstudiosharedv1.DeploymentTargetClaim)) appstudiosharedv1.DeploymentTargetClaim {
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

func getDeploymentTarget(ops ...func(dt *appstudiosharedv1.DeploymentTarget)) appstudiosharedv1.DeploymentTarget {
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
