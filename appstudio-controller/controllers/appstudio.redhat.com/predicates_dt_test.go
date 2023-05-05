package appstudioredhatcom

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	applicationv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("Test Predicates", func() {
	Context("Testing DevsandboxDeploymentConciler", func() {

		var (
			dt    *applicationv1alpha1.DeploymentTarget
			dtnew *applicationv1alpha1.DeploymentTarget
		)

		BeforeEach(func() {
			scheme,
				_,
				_,
				_,
				err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = applicationv1alpha1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			dt = &applicationv1alpha1.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-dt",
					Namespace:   "test-namespace",
					Annotations: map[string]string{},
				},
				Spec: applicationv1alpha1.DeploymentTargetSpec{
					DeploymentTargetClassName: applicationv1alpha1.DeploymentTargetClassName("test-sandbox-class"),
					KubernetesClusterCredentials: applicationv1alpha1.DeploymentTargetKubernetesClusterCredentials{
						DefaultNamespace:         "test-ns",
						APIURL:                   "https://api-url/api",
						ClusterCredentialsSecret: "test-secret",
					},
				},
				Status: applicationv1alpha1.DeploymentTargetStatus{
					Phase: applicationv1alpha1.DeploymentTargetPhase_Pending,
				},
			}
			dtnew = dt.DeepCopy()

		})
		Context("Test DeploymentTargetReleasedPredicate predicate", func() {
			instance := DeploymentTargetDeletePredicate()

			It("should ignore when creating a DeploymentTarget", func() {
				dt.Status.Phase = applicationv1alpha1.DeploymentTargetPhase_Released
				contextEvent := event.CreateEvent{
					Object: dt,
				}
				Expect(instance.Create(contextEvent)).To(BeFalse())
			})

			It("should pick up on deletion events", func() {
				contextEvent := event.DeleteEvent{
					Object: dt,
				}
				Expect(instance.Delete(contextEvent)).To(BeTrue())
			})

			It("should ignore generic events", func() {
				contextEvent := event.GenericEvent{
					Object: dt,
				}
				Expect(instance.Generic(contextEvent)).To(BeFalse())
			})

			It("should pick up on the DeploymentTarget with DeletionTimeStamp", func() {
				now := metav1.NewTime(metav1.Now().Add(time.Second * 1))
				dtnew.SetDeletionTimestamp(&now)
				contextEvent := event.UpdateEvent{
					ObjectOld: dt,
					ObjectNew: dtnew,
				}
				Expect(isDTDeletionTimestampNonNil(dt, dtnew)).To(BeTrue())
				Expect(instance.Update(contextEvent)).To(BeTrue())
			})
		})
	})
})
