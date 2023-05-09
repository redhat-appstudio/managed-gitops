package appstudioredhatcom

import (
	codereadytoolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("Test Predicates", func() {
	Context("Testing DevsandboxDeploymentConciler", func() {

		var (
			spacerequest    *codereadytoolchainv1alpha1.SpaceRequest
			spacerequestnew *codereadytoolchainv1alpha1.SpaceRequest
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

			spacerequest = &codereadytoolchainv1alpha1.SpaceRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-spacerequest",
					Namespace:   "test-namespace",
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
							Name:      "",
							SecretRef: "",
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
			spacerequestnew = spacerequest.DeepCopy()

		})
		Context("Test SpaceRequestReadyPredicate predicate", func() {
			instance := spaceRequestReadyPredicate()

			It("should ignore when creating a SpaceRequest", func() {
				spacerequest.Status.Conditions[0].Status = corev1.ConditionTrue
				contextEvent := event.CreateEvent{
					Object: spacerequest,
				}
				Expect(instance.Create(contextEvent)).To(BeFalse())
			})

			It("should ignore deleting events", func() {
				contextEvent := event.DeleteEvent{
					Object: spacerequest,
				}
				Expect(instance.Delete(contextEvent)).To(BeFalse())
			})

			It("should ignore generic events", func() {
				contextEvent := event.GenericEvent{
					Object: spacerequest,
				}
				Expect(instance.Generic(contextEvent)).To(BeFalse())
			})

			It("should ignore when the NamespaceAccess Not setted", func() {
				Expect(doesSpaceRequestHaveReadyTrue(spacerequestnew)).To(BeFalse())
				spacerequestnew.Status.Conditions[0].Status = corev1.ConditionTrue
				namespaceaccess := []codereadytoolchainv1alpha1.NamespaceAccess{}
				spacerequestnew.Status.NamespaceAccess = namespaceaccess
				Expect(doesSpaceRequestHaveReadyTrue(spacerequestnew)).To(BeTrue())
				contextEvent := event.UpdateEvent{
					ObjectOld: spacerequest,
					ObjectNew: spacerequestnew,
				}
				Expect(predicateIsReadySpaceRequest(spacerequest, spacerequestnew)).To(BeFalse())
				Expect(instance.Update(contextEvent)).To(BeFalse())
			})

			It("should pick up on the SpaceRequest into Ready Status", func() {
				spacerequestnew.Status.Conditions[0].Status = corev1.ConditionTrue
				Expect(doesSpaceRequestHaveReadyTrue(spacerequestnew)).To(BeTrue())
				spacerequestnew.Status.NamespaceAccess[0].Name = "test-ns"
				spacerequestnew.Status.NamespaceAccess[0].SecretRef = "test-secret"
				Expect(predicateIsReadySpaceRequest(spacerequest, spacerequestnew)).To(BeTrue())
				contextEvent := event.UpdateEvent{
					ObjectOld: spacerequest,
					ObjectNew: spacerequestnew,
				}
				Expect(instance.Update(contextEvent)).To(BeTrue())
			})
		})
	})
})
