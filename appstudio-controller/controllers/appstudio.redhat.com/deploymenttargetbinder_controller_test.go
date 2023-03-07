package appstudioredhatcom

import (
	"context"
	"fmt"

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

var _ = Describe("Test DeploymentTargetClaimBinderController", func() {
	Context("Testing DeploymentTargetClaimBinderController", func() {

		var (
			ctx        context.Context
			k8sClient  client.Client
			reconciler DeploymentTargetClaimReconciler
			request    ctrl.Request
		)

		// 1. Handle Bounded DTC
		// 2. Handle DTC for dynamic provisioning
		// 3. Handle DTC for non-dynamic

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

		Context("Test binder controller with a bounded DeploymentTargetClaim", func() {
			It("should handle already bounded DTC and DT", func() {
				By("create both DT and DTC bounded to each other")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: request.Namespace,
						Annotations: map[string]string{
							annBindCompleted: annBinderValueYes,
						},
					},
					Status: appstudiosharedv1.DeploymentTargetClaimStatus{
						Phase: appstudiosharedv1.DeploymentTargetClaimPhase_Pending,
					},
				}
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				dt := appstudiosharedv1.DeploymentTarget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dt",
						Namespace: request.Namespace,
					},
					Spec: appstudiosharedv1.DeploymentTargetSpec{
						ClaimRef: dtc.Name,
					},
					Status: appstudiosharedv1.DeploymentTargetStatus{
						Phase: appstudiosharedv1.DeploymentTargetPhase_Pending,
					},
				}
				err = k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				By("reconcile with a bounded DT and DTC")
				request = newRequest(dtc.Namespace, dtc.Name)
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
				By("create only DTC with bounded annotation")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: request.Namespace,
						Annotations: map[string]string{
							annBindCompleted: annBinderValueYes,
						},
					},
				}
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile with a bounded DT and DTC")
				request = newRequest(dtc.Namespace, dtc.Name)
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

			It("should return an error and set the DTC status to Lost if no DT claims DTC", func() {
				By("creates DTC with bounded annotation")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: request.Namespace,
						Annotations: map[string]string{
							annBindCompleted: annBinderValueYes,
						},
					},
				}
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("create multiple DTs that don't claim the given DTC")
				for i := 0; i < 3; i++ {
					dt := appstudiosharedv1.DeploymentTarget{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test-dt-%d", i),
							Namespace: dtc.Namespace,
						},
						Spec: appstudiosharedv1.DeploymentTargetSpec{
							ClaimRef: fmt.Sprintf("test-dtc-%d", i),
						},
						Status: appstudiosharedv1.DeploymentTargetStatus{
							Phase: appstudiosharedv1.DeploymentTargetPhase_Bound,
						},
					}
					err = k8sClient.Create(ctx, &dt)
					Expect(err).To(BeNil())
				}

				By("reconcile with a bounded DTC and multiple DTs that are claiming other DTCs")
				request = newRequest(dtc.Namespace, dtc.Name)
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

		Context("Handle DeploymentTargetClaim for Dynamic Provisioning", func() {
			// 1. DT was already found
			// 2. DT with not class
			// 3. DT has a class
			It("should mark the DTC for dynamic provisioning if the target name is not set", func() {
				By("create a DTC without specifying the DT")
				provisionerName := appstudiosharedv1.DeploymentTargetClassName("sandbox-class")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: request.Namespace,
					},
					Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
						DeploymentTargetClassName: provisionerName,
					},
				}
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile and check if it is marked for dynamic provisioning")
				request = newRequest(dtc.Namespace, dtc.Name)
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
				Expect(dtc.Annotations[annTargetProvisioner]).To(Equal(string(provisionerName)))
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))
			})

			It("shouldn't mark the DTC for dynamic provisioning and use an existing DT if it exists", func() {
				By("create a DTC without specifying the DT")
				provisionerName := appstudiosharedv1.DeploymentTargetClassName("sandbox-class")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: request.Namespace,
					},
					Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
						DeploymentTargetClassName: provisionerName,
					},
				}
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("create a DT that claims the above DTC")
				dt := appstudiosharedv1.DeploymentTarget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dt",
						Namespace: dtc.Namespace,
					},
					Spec: appstudiosharedv1.DeploymentTargetSpec{
						ClaimRef:                  dtc.Name,
						DeploymentTargetClassName: provisionerName,
					},
				}
				err = k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				By("reconcile a DTC and verify that it isn't marked for dynamic provisioning")
				request = newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if both DTC and DT are bound together")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Annotations).ShouldNot(BeNil())
				// check if the provision annotation is not set
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Bound))
				_, found := dtc.Annotations[annTargetProvisioner]
				Expect(found).To(BeFalse())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dt), &dt)
				Expect(err).To(BeNil())
				Expect(dt.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetPhase_Bound))
			})

			It("should mark the DTC as pending if the DT isn't found and DTClass is not set", func() {
				By("create a DTC without specifying the DeploymentTargetClass")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: request.Namespace,
					},
				}
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile a DTC and verify that it isn't marked for dynamic provisioning")
				request = newRequest(dtc.Namespace, dtc.Name)
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

		Context("Handle DeploymentTargetClaim with non-dynamic provisioning", func() {
			It("should return an error and requeue if DT pointed by DTC is not found", func() {
				By("create a DTC that points to a DT that doesn't exist")
				provisionerName := appstudiosharedv1.DeploymentTargetClassName("sandbox-class")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: request.Namespace,
					},
					Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
						DeploymentTargetClassName: provisionerName,
						TargetName:                "random",
					},
				}
				err := k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile and verify if it returns an error with requeue")
				request = newRequest(dtc.Namespace, dtc.Name)
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

			It("should bind if the DT is found and if it refers the DTC", func() {
				By("create a DTC and DT that refer each other")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: request.Namespace,
					},
				}

				dt := appstudiosharedv1.DeploymentTarget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dt",
						Namespace: dtc.Namespace,
					},
					Spec: appstudiosharedv1.DeploymentTargetSpec{
						ClaimRef: dtc.Name,
					},
				}
				err := k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				dtc.Spec.TargetName = dt.Name
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile and verify if it returns an error with requeue")
				request = newRequest(dtc.Namespace, dtc.Name)
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
				By("create a DTC and DT that refer each other")
				dtc := appstudiosharedv1.DeploymentTargetClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dtc",
						Namespace: request.Namespace,
					},
				}

				dt := appstudiosharedv1.DeploymentTarget{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dt",
						Namespace: dtc.Namespace,
					},
					Spec: appstudiosharedv1.DeploymentTargetSpec{
						ClaimRef: "random-dtc",
					},
					Status: appstudiosharedv1.DeploymentTargetStatus{
						Phase: appstudiosharedv1.DeploymentTargetPhase_Bound,
					},
				}
				err := k8sClient.Create(ctx, &dt)
				Expect(err).To(BeNil())

				dtc.Spec.TargetName = dt.Name
				err = k8sClient.Create(ctx, &dtc)
				Expect(err).To(BeNil())

				By("reconcile and verify if it returns an error with requeue")
				request = newRequest(dtc.Namespace, dtc.Name)
				res, err := reconciler.Reconcile(ctx, request)
				Expect(err).ToNot(BeNil())
				alreadyClaimedErr := fmt.Errorf("DeploymentTargetClaim %s wants to claim DeploymentTarget %s that was already claimed in namespace %s", dtc.Name, dt.Name, dtc.Namespace)
				Expect(err.Error()).To(Equal(alreadyClaimedErr.Error()))
				Expect(res).To(Equal(ctrl.Result{}))

				By("verify if DTC has pending state")
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&dtc), &dtc)
				Expect(err).To(BeNil())
				Expect(dtc.Status.Phase).To(Equal(appstudiosharedv1.DeploymentTargetClaimPhase_Pending))
			})

		})
	})
})

func createDTC(name, ns, class, target string) appstudiosharedv1.DeploymentTargetClaim {
	return appstudiosharedv1.DeploymentTargetClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
			DeploymentTargetClassName: appstudiosharedv1.DeploymentTargetClassName(class),
			TargetName:                target,
		},
	}
}

func createDT(name, ns, class, claimRef string) appstudiosharedv1.DeploymentTarget {
	return appstudiosharedv1.DeploymentTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: appstudiosharedv1.DeploymentTargetSpec{
			DeploymentTargetClassName: appstudiosharedv1.DeploymentTargetClassName(class),
			ClaimRef:                  claimRef,
		},
	}
}
