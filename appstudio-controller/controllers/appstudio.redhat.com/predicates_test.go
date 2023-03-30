package appstudioredhatcom

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("Test Predicates", func() {
	Context("Testing SandboxProvisionerController", func() {

		var (
			dtc    *appstudiosharedv1.DeploymentTargetClaim
			dtcNew *appstudiosharedv1.DeploymentTargetClaim
		)

		BeforeEach(func() {
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
					Name: "test-predicates",
				},
			}

			dtc = &appstudiosharedv1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "dtc-",
					Namespace:    testNS.Name,
					Annotations: map[string]string{
						appstudiosharedv1.AnnBindCompleted:     appstudiosharedv1.AnnBinderValueTrue,
						appstudiosharedv1.AnnTargetProvisioner: "sandbox-provisioner",
					},
				},
				Spec: appstudiosharedv1.DeploymentTargetClaimSpec{
					TargetName:                "random-dt",
					DeploymentTargetClassName: "random-dtcls",
				},
			}
			dtcNew = dtc.DeepCopy()

		})
		Context("Test DTCPendingDynamicProvisioningBySandbox predicate", func() {
			instance := DTCPendingDynamicProvisioningBySandbox()

			It("should ignore creating events", func() {
				contextEvent := event.CreateEvent{
					Object: dtc,
				}
				Expect(instance.Create(contextEvent)).To(BeFalse())
			})
			It("should ignore deleting events", func() {
				contextEvent := event.DeleteEvent{
					Object: dtc,
				}
				Expect(instance.Delete(contextEvent)).To(BeFalse())
			})

			It("should ignore generic events", func() {
				contextEvent := event.GenericEvent{
					Object: dtc,
				}
				Expect(instance.Generic(contextEvent)).To(BeFalse())
			})

			It("should ignore generic events", func() {
				contextEvent := event.GenericEvent{
					Object: dtc,
				}
				Expect(instance.Generic(contextEvent)).To(BeFalse())
			})

			It("should pick up on the Sandbox deploymentTargetClaim going into Pending phase", func() {
				dtcNew.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Pending
				contextEvent := event.UpdateEvent{
					ObjectOld: dtc,
					ObjectNew: dtcNew,
				}
				Expect(instance.Update(contextEvent)).To(BeTrue())
			})

			It("shouldn ignore the Sandbox deploymentTargetClaim going into Lost phase", func() {
				dtcNew.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Lost
				contextEvent := event.UpdateEvent{
					ObjectOld: dtc,
					ObjectNew: dtcNew,
				}
				Expect(instance.Update(contextEvent)).To(BeFalse())
			})

			It("should ignore when the Sandbox deploymentTargetClaim keeps the Pending phase", func() {
				dtc.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Pending
				dtcNew.Status.Phase = appstudiosharedv1.DeploymentTargetClaimPhase_Pending
				contextEvent := event.UpdateEvent{
					ObjectOld: dtc,
					ObjectNew: dtcNew,
				}
				Expect(instance.Update(contextEvent)).To(BeFalse())
			})
		})
	})
})
