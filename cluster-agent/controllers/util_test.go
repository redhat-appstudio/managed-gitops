package controllers

import (
	"context"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Tests for the small number of utility functions in cluster-agent/controllers util.go", func() {

	Context("DeleteArgoCDApplication tests", func() {

		var ctx context.Context
		var k8sClient client.WithWatch
		var logger logr.Logger
		var kubesystemNamespace *corev1.Namespace
		var argocdNamespace *corev1.Namespace
		var workspace *corev1.Namespace
		var scheme *runtime.Scheme
		var err error

		simulateArgoCD := func(goApplication *appv1.Application) {
			Eventually(func() bool {

				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(goApplication), goApplication)
				if err != nil {
					GinkgoWriter.Println("error from client on Get", err)
					return false
				}

				// Wait for the finalizer to be added by 'DeleteArgoCDApplication'
				finalizerFound := false
				for _, finalizer := range goApplication.Finalizers {
					if finalizer == argoCDResourcesFinalizer {
						finalizerFound = true
					}
				}
				if !finalizerFound {
					GinkgoWriter.Println("finalizer not yet set on Application")
					return false
				}

				// Wait for DeleteArgoCDApplication to delete the Application
				if goApplication.DeletionTimestamp == nil {
					GinkgoWriter.Println("DeletionTimestamp is not yet set")
					return false
				}

				// The remaining steps simulate Argo CD deleting the Application

				// Remove the finalizer
				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(goApplication), goApplication)
				if err != nil {
					GinkgoWriter.Println("error from client on second Get", err)
					return false
				}

				goApplication.Finalizers = []string{}
				err = k8sClient.Update(ctx, goApplication)
				if err != nil {
					GinkgoWriter.Println("unable to update Application")
					return false
				}

				err = k8sClient.Delete(ctx, goApplication)
				if err != nil {
					GinkgoWriter.Println("error from client on Delete", err)
					return false
				}

				GinkgoWriter.Println("The Application was successfully deleted.")

				return true
			}, "5s", "10ms").Should(BeTrue())

		}

		BeforeEach(func() {
			ctx = context.Background()
			logger = log.FromContext(ctx)

			scheme, argocdNamespace, kubesystemNamespace, workspace, err = tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			By("Initialize fake kube client")
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, argocdNamespace, kubesystemNamespace).Build()

		})

		It("should not delete an Argo CD Application which is missing the databaseID label", func() {

			By("creating an Argo CD Application without a databaseID label")
			application := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-name",
					Namespace: "my-namespace",
					Labels:    map[string]string{},
				},
			}
			err := k8sClient.Create(ctx, &application)
			Expect(err).To(BeNil())

			By("calling the DeleteArgoCDApplication function")
			err = DeleteArgoCDApplication(ctx, application, k8sClient, logger)
			Expect(err).To(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&application), &application)
			Expect(err).To(BeNil(), "Application should still exist: it should not have been deleted")
			Expect(len(application.Finalizers)).To(BeZero(), "no finalizers should have been added")
		})

		It("should delete an Argo CD Application which has the databaseID label", func() {

			By("creating an Argo CD Application with a databaseID label")
			application := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-name",
					Namespace: "my-namespace",
					Labels: map[string]string{
						ArgoCDApplicationDatabaseIDLabel: "test-my-database-id-label",
					},
				},
			}
			err := k8sClient.Create(ctx, &application)
			Expect(err).To(BeNil())

			By("starting a Goroutine to simulate Argo CD's deletion behaviour")
			goApplication := application.DeepCopy()
			go func() {
				simulateArgoCD(goApplication)
			}()

			By("calling the DeleteArgoCDApplication function")
			err = DeleteArgoCDApplication(ctx, application, k8sClient, logger)
			Expect(err).To(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&application), &application)
			Expect(err).ToNot(BeNil(), "Application should not exist: it should have been deleted")

		})

		It("should delete an Argo CD Application which has the databaseID label, and the Application already has a deletion finalizer set", func() {

			By("creating an Argo CD Application with a finalizer and a databaseID label")
			application := appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-name",
					Namespace: "my-namespace",
					Labels: map[string]string{
						ArgoCDApplicationDatabaseIDLabel: "test-my-database-id-label",
					},
					Finalizers: []string{
						argoCDResourcesFinalizer,
					},
				},
			}
			err := k8sClient.Create(ctx, &application)
			Expect(err).To(BeNil())

			By("starting a Goroutine to simulate Argo CD's deletion behaviour")
			goApplication := application.DeepCopy()
			go func() {
				simulateArgoCD(goApplication)
			}()

			By("calling the DeleteArgoCDApplication function")
			err = DeleteArgoCDApplication(ctx, application, k8sClient, logger)
			Expect(err).To(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(&application), &application)
			Expect(err).ToNot(BeNil(), "Application should not exist: it should have been deleted")

		})

	})

})
