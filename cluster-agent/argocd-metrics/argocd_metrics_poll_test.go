package argocdmetrics

import (
	"context"
	"time"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Argo CD Metrics Polling", func() {
	Context("Argo CD Metrics Polling Test", func() {
		var ctx context.Context
		var k8sClient client.Client
		var updater ReconciliationMetricsUpdater
		var err error

		BeforeEach(func() {
			scheme, argocdNamespace, kubesystemNamespace, _, err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			testNamespaceNames = []string{"argocd"}
			namespaces := createNamespaces(testNamespaceNames...)

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(argocdNamespace, kubesystemNamespace).
				WithObjects(namespaces...).
				Build()

			ctx = context.Background()
			updater = ReconciliationMetricsUpdater{
				Client: k8sClient,
			}
		})

		AfterEach(func() {
			ReconciledArgoAppsPercent.Set(0.0)
			testNamespaceNames = []string{}
		})

		It("Tests the polling method", func() {
			By("Starting the polling goroutine to wait two seconds, then poll once")
			go updater.poll(time.Second * 2)

			By("Checking the metric is zero at this point")
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))

			By("Creating an argocd application")
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
						Time: time.Now(),
					},
				},
			}
			err = updater.Create(ctx, app)
			Expect(err).To(BeNil())

			By("Checking the metric is still zero at this point")
			Expect(testutil.ToFloat64(ReconciledArgoAppsPercent)).To(Equal(0.0))

			By("Waiting for the metric to update")
			Eventually(func() float64 {
				return testutil.ToFloat64(ReconciledArgoAppsPercent)
			}, "5s", "250ms").Should(Equal(100.0))
		})
	})
})
