package argocdmetrics

import (
	"context"
	"time"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func createNamespaces(names ...string) []client.Object {
	var result []client.Object
	for _, name := range names {
		namespace := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				UID:  uuid.NewUUID(),
			},
			Spec: corev1.NamespaceSpec{},
		}
		result = append(result, &namespace)
	}
	return result
}

var _ = Describe("Argo CD Metrics", func() {
	Context("Argo CD Metrics Test", func() {
		var ctx context.Context
		var updater reconciliationMetricsUpdater
		var err error
		var testNamespaceNames []string

		BeforeEach(func() {
			scheme, argocdNamespace, kubesystemNamespace, _, err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			testNamespaceNames = []string{"argocd", "argocd-01"}
			namespaces := createNamespaces(testNamespaceNames...)

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(argocdNamespace, kubesystemNamespace).
				WithObjects(namespaces...).
				Build()

			ctx = context.Background()
			updater = reconciliationMetricsUpdater{
				client:             k8sClient,
				ctx:                ctx,
				timeWindow:         time.Now().Add(-1 * windowSize),
				testNamespaceNames: testNamespaceNames,
			}
		})

		AfterEach(func() {
			ReconciledArgoAppsPercent.Set(0.0)
			testNamespaceNames = []string{}
		})

		It("Reconciliation metrics are zero if there are no argocd applications", func() {
			By("Not creating any ArgoCD applications")
			By("Calling the method to update the metric")
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
			updater.updateReconciliationMetrics()
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
		})

		It("Reconciliation metrics are zero if the argocd application has not been reconciled", func() {
			By("Creating an ArgoCD application that has not been reconciled")
			app := &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "application-01",
					Namespace: testNamespaceNames[0],
				},
			}
			err = updater.client.Create(ctx, app)
			Expect(err).To(BeNil())

			By("Calling the method to update the metric")
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
			updater.updateReconciliationMetrics()
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
		})

		It("Reconciliation metrics are zero if the argocd application has been reconciled more than 3 minutes ago", func() {
			By("Creating an ArgoCD application with a reconcile time greater than three minutes ago")
			app := &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "application-01",
					Namespace: testNamespaceNames[0],
				},
				Status: appv1.ApplicationStatus{
					ReconciledAt: &metav1.Time{
						Time: time.Now().Add(-4 * time.Minute),
					},
				},
			}
			err = updater.client.Create(ctx, app)
			Expect(err).To(BeNil())

			By("Calling the method to update the metric")
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
			updater.updateReconciliationMetrics()
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
		})

		It("Argocd applications in the other namespaces do not contribute to the metrics", func() {
			By("Creating an ArgoCD application in another namespace")
			app := &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "application-01",
					Namespace: "bogus",
				},
				Status: appv1.ApplicationStatus{
					ReconciledAt: &metav1.Time{
						Time: time.Now().Add(-2 * time.Minute),
					},
				},
			}
			err = updater.client.Create(ctx, app)
			Expect(err).To(BeNil())

			By("Calling the method to update the metric")
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
			updater.updateReconciliationMetrics()
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
		})

		It("Reconciliation metrics are 100% if the argocd application has been reconciled withing the last 3 minutes", func() {
			By("Creating an ArgoCD application with a reconcile time within the last three minutes")
			app := &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "application-01",
					Namespace: testNamespaceNames[0],
				},
				Status: appv1.ApplicationStatus{
					ReconciledAt: &metav1.Time{
						Time: time.Now().Add(-2 * time.Minute),
					},
				},
			}
			err = updater.client.Create(ctx, app)
			Expect(err).To(BeNil())

			By("Calling the method to update the metric")
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
			updater.updateReconciliationMetrics()
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(100.0))
		})

		It("Reconciliation metrics are 50% if one of two argocd applications have been reconciled withing the last 3 minutes", func() {
			By("Creating an ArgoCD application reconciled within the last three minutes")
			app := &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "application-01",
					Namespace: testNamespaceNames[0],
				},
				Status: appv1.ApplicationStatus{
					ReconciledAt: &metav1.Time{
						Time: time.Now().Add(-2 * time.Minute),
					},
				},
			}
			err = updater.client.Create(ctx, app)
			Expect(err).To(BeNil())

			By("Creating an ArgoCD application reconciled more than three minutes ago")
			app = &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "application-02",
					Namespace: testNamespaceNames[0],
				},
				Status: appv1.ApplicationStatus{
					ReconciledAt: &metav1.Time{
						Time: time.Now().Add(-4 * time.Minute),
					},
				},
			}
			err = updater.client.Create(ctx, app)
			Expect(err).To(BeNil())

			By("Calling the method to update the metric")
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
			updater.updateReconciliationMetrics()
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(50.0))
		})

		It("Reconciliation metrics are 100% if two argocd applications in different gitops argocd namespaces have been reconciled withing the last 3 minutes", func() {
			By("Creating an ArgoCD application reconciled within the last three minutes")
			app := &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "application-01",
					Namespace: testNamespaceNames[0],
				},
				Status: appv1.ApplicationStatus{
					ReconciledAt: &metav1.Time{
						Time: time.Now().Add(-2 * time.Minute),
					},
				},
			}
			err = updater.client.Create(ctx, app)
			Expect(err).To(BeNil())

			By("Creating a second ArgoCD application reconciled within the last three minutes")
			app = &appv1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "application-02",
					Namespace: testNamespaceNames[1],
				},
				Status: appv1.ApplicationStatus{
					ReconciledAt: &metav1.Time{
						Time: time.Now().Add(-1 * time.Minute),
					},
				},
			}
			err = updater.client.Create(ctx, app)
			Expect(err).To(BeNil())

			By("Calling the method to update the metric")
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))
			updater.updateReconciliationMetrics()
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(100.0))
		})
	})
})
