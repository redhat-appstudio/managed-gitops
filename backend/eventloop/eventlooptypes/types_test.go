package eventlooptypes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("GitOpsEngine Client Test", Ordered, func() {
	Context("getGitOpsEngineWorkloadClient returns a client for accessing GitOpsEngine cluster", func() {

		var k8sClient client.Client
		var ctx context.Context
		var fakeServer *httptest.Server

		BeforeAll(func() {
			scheme, _, _, _, err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			gitopsNamespace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gitops",
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(&gitopsNamespace).
				Build()

			ctx = context.Background()

			// create a fake server to validate the client
			fakeServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var obj interface{}
				switch req.URL.Path {
				case "/api":
					obj = &metav1.APIVersions{
						Versions: []string{
							"v1",
						},
					}
				default:
					w.WriteHeader(http.StatusNotFound)
					return
				}
				output, err := json.Marshal(obj)
				if err != nil {
					GinkgoT().Fatalf("unexpected encoding error: %v", err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, err = w.Write(output)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
			}))
		})

		getClusterSecret := func(host, token string) *corev1.Secret {
			clusterSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitops-engine-cluster",
					Namespace: "gitops",
				},
				Data: map[string][]byte{},
			}

			if host != "" {
				clusterSecret.Data["host"] = []byte(host)
			}
			if token != "" {
				clusterSecret.Data["bearer_token"] = []byte(token)
			}

			return &clusterSecret
		}

		AfterEach(func() {
			err := k8sClient.Delete(ctx, getClusterSecret("", ""))
			if err != nil {
				Expect(errors.IsNotFound(err)).To(BeTrue())
			}
		})

		It("should return an error if the cluster secret is missing", func() {
			_, err := getGitOpsEngineWorkloadClient(ctx, k8sClient, nil)
			Expect(err).ShouldNot(BeNil())
			expectedErr := "unable to find cluster credentials for GitOpsEngine cluster: secrets \"gitops-engine-cluster\" not found"
			Expect(err.Error()).Should(Equal(expectedErr))
		})

		It("should return an error if the cluster secret is missing host URL field", func() {
			err := k8sClient.Create(ctx, getClusterSecret("", "sample_token"))
			Expect(err).To(BeNil())

			_, err = getGitOpsEngineWorkloadClient(ctx, k8sClient, nil)
			Expect(err).ShouldNot(BeNil())
			expectedErr := "missing host API URL in the GitOpsEngine cluster secret"
			Expect(err.Error()).Should(Equal(expectedErr))
		})

		It("should return an error if the cluster secret is missing bearer token field", func() {
			err := k8sClient.Create(ctx, getClusterSecret(fakeServer.URL, ""))
			Expect(err).To(BeNil())

			_, err = getGitOpsEngineWorkloadClient(ctx, k8sClient, nil)
			Expect(err).ShouldNot(BeNil())
			expectedErr := "missing bearer token in the GitOpsEngine cluster secret"
			Expect(err.Error()).Should(Equal(expectedErr))
		})

		It("should return a valid client if a valid cluster secret is found", func() {
			err := k8sClient.Create(ctx, getClusterSecret(fakeServer.URL, "sample_token"))
			Expect(err).To(BeNil())

			gitopsEngineClient, err := getGitOpsEngineWorkloadClient(ctx, k8sClient, nil)
			Expect(err).Should(BeNil())
			Expect(gitopsEngineClient).ShouldNot(BeNil())

		})

	})
})
