package eventloop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"

	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("ClusterReconciler tests", func() {
	Context("Test getAllAPIResources", func() {

		It("should return all namespaced scoped API resourcs in the cluster", func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			clusterResources := []client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-3",
					},
				},
			}

			namespacedResources := []client.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-1",
						Namespace: apiNamespace.Name,
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-2",
						Namespace: argocdNamespace.Name,
					},
				},
				&rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-4",
					},
				}}

			// Create fake client
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				WithObjects(namespacedResources...).
				WithObjects(clusterResources...).
				Build()

			server := createFakeCluster()
			defer server.Close()

			discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(&rest.Config{
				Host: server.URL,
			})

			cr := NewClusterReconciler(k8sClient, discoveryClient)
			ctx := context.Background()
			logger := log.FromContext(ctx)

			By("verify if only namespaced resources are returned")
			objs, err := cr.getAllNamespacedAPIResources(ctx, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(objs).To(HaveLen(len(namespacedResources)))

			for _, expectedObj := range namespacedResources {
				found := false
				for _, obj := range objs {
					if obj.GetName() == expectedObj.GetName() &&
						obj.GetNamespace() == expectedObj.GetNamespace() &&
						obj.GetUID() == expectedObj.GetUID() {
						found = true
						break
					}
				}
				if !found {
					Expect(found).To(BeTrue(), "expected a namespace scoped resource")
				}
			}

			By("delete all namespace scoped resources and verify if it returns an empty list")
			for _, obj := range namespacedResources {
				err = k8sClient.Delete(ctx, obj)
				Expect(err).ToNot(HaveOccurred())
			}

			objs, err = cr.getAllNamespacedAPIResources(ctx, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(objs).To(BeEmpty())

			By("ensure that a resource without the required verbs is not returned")
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sample-pod",
					Namespace: apiNamespace.Name,
				},
			}

			err = k8sClient.Create(ctx, pod)
			Expect(err).ToNot(HaveOccurred())

			objs, err = cr.getAllNamespacedAPIResources(ctx, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(objs).To(BeEmpty())
		})
	})

	Context("Test cleanOrphanedResources()", func() {

		var (
			k8sClient  client.Client
			reconciler *ClusterReconciler
			ctx        context.Context
			logger     logr.Logger
			server     *httptest.Server
			namespace  *corev1.Namespace
		)

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			namespace = argocdNamespace

			ctx = context.Background()
			logger = log.FromContext(ctx)

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			server = createFakeCluster()

			discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(&rest.Config{
				Host: server.URL,
			})

			reconciler = NewClusterReconciler(k8sClient, discoveryClient)

		})

		AfterEach(func() {
			server.Close()
		})

		It("should delete orphaned resources", func() {
			namespacedObj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-1",
					Namespace: namespace.Name,
					Labels: map[string]string{
						"app.kubernetes.io/instance": "gitopsdepl-08745631-43fe-41c3-9bd8-a2cf347a04c2",
					},
				},
			}

			err := k8sClient.Create(ctx, namespacedObj)
			Expect(err).ToNot(HaveOccurred())

			reconciler.cleanOrphanedResources(ctx, logger)

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(namespacedObj), namespacedObj)
			Expect(apierr.IsNotFound(err)).To(BeTrue())
		})

		It("should not delete a resource that is still managed by GitOpsDeployment", func() {
			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sample",
					Namespace: namespace.Name,
				},
			}

			err := k8sClient.Create(ctx, gitopsDepl)
			Expect(err).ToNot(HaveOccurred())

			namespacedObj := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-4",
					Labels: map[string]string{
						"app.kubernetes.io/instance": fmt.Sprintf("gitopsdepl-%s", gitopsDepl.UID),
					},
				},
			}

			reconciler.cleanOrphanedResources(ctx, logger)

			err = k8sClient.Create(ctx, namespacedObj)
			Expect(err).ToNot(HaveOccurred())

		})

		It("should verify that a resource not managed by GitOpsDeployment is not deleted", func() {
			namespacedObj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-2",
					Namespace: namespace.Name,
				},
			}

			err := k8sClient.Create(ctx, namespacedObj)
			Expect(err).ToNot(HaveOccurred())

			reconciler.cleanOrphanedResources(ctx, logger)

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(namespacedObj), namespacedObj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should not delete cluster scoped resources", func() {
			clusterObj := &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-3",
				},
			}

			err := k8sClient.Create(ctx, clusterObj)
			Expect(err).ToNot(HaveOccurred())

			reconciler.cleanOrphanedResources(ctx, logger)

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(clusterObj), clusterObj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should not delete a resource that has a finalizer", func() {
			namespacedObj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-1",
					Namespace: namespace.Name,
					Labels: map[string]string{
						"app.kubernetes.io/instance": "gitopsdepl-08745631-43fe-41c3-9bd8-a2cf347a04c2",
					},
					Finalizers: []string{"sample.finalizer"},
				},
			}

			err := k8sClient.Create(ctx, namespacedObj)
			Expect(err).ToNot(HaveOccurred())

			reconciler.cleanOrphanedResources(ctx, logger)

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(namespacedObj), namespacedObj)
			Expect(err).ToNot(HaveOccurred())
			Expect(namespacedObj.DeletionTimestamp).ToNot(BeNil())
		})

		It("should not delete a resource that already has a deletion timestamp", func() {
			namespacedObj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-1",
					Namespace: namespace.Name,
					Labels: map[string]string{
						"app.kubernetes.io/instance": "gitopsdepl-08745631-43fe-41c3-9bd8-a2cf347a04c2",
					},
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
			}

			err := k8sClient.Create(ctx, namespacedObj)
			Expect(err).ToNot(HaveOccurred())

			reconciler.cleanOrphanedResources(ctx, logger)

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(namespacedObj), namespacedObj)
			Expect(err).ToNot(HaveOccurred())
		})

	})
})

func createFakeCluster() *httptest.Server {
	verbs := []string{"get", "list", "delete"}
	fakeServer := func(w http.ResponseWriter, req *http.Request) {
		var list interface{}
		switch req.URL.Path {
		case "/api/v1":
			list = &metav1.APIResourceList{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "pods", Namespaced: true, Kind: "Pod"},
					{Name: "services", Namespaced: true, Kind: "Service", Verbs: verbs},
					{Name: "namespaces", Namespaced: false, Kind: "Namespace"},
				},
			}
		case "/apis/rbac.authorization.k8s.io/v1":
			list = &metav1.APIResourceList{
				GroupVersion: "rbac.authorization.k8s.io/v1",
				APIResources: []metav1.APIResource{
					{Name: "rolebindings", Namespaced: true, Kind: "RoleBinding", Verbs: verbs},
					{Name: "clusterroles", Namespaced: false, Kind: "ClusterRole"},
				},
			}
		case "/api":
			list = &metav1.APIVersions{
				Versions: []string{
					"v1",
					"rbac.authorization.k8s.io/v1",
				},
			}
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}

		output, err := json.Marshal(list)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(output)
		Expect(err).ToNot(HaveOccurred())
	}

	return httptest.NewServer(http.HandlerFunc(fakeServer))
}
