package util

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("ControllerManager test", func() {

	Context("GetControllerManager should return the appropriate manager", func() {

		ctx := context.Background()
		log := log.FromContext(ctx)
		apiExportName := "test"
		var fakeClient client.Client
		var fakeDiscovery *fakediscovery.FakeDiscovery
		var fakeServer *httptest.Server
		var cfg *rest.Config
		var opts ctrl.Options

		BeforeEach(func() {
			scheme := runtime.NewScheme()

			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

			fakeDiscovery = &fakediscovery.FakeDiscovery{
				Fake: &testing.Fake{
					Resources: []*metav1.APIResourceList{
						{
							GroupVersion: "v1",
						},
					},
				},
			}

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

			cfg = &rest.Config{
				Host:     fakeServer.URL,
				Username: "fake",
				Password: "fake",
			}

			opts = ctrl.Options{
				MetricsBindAddress: "0",
			}
		})

		AfterEach(func() {
			fakeServer.Close()
		})

		It("Should return a standard manager", func() {
			gotMgr, err := getControllerManager(ctx, cfg, fakeClient, fakeDiscovery, &log, apiExportName, opts)
			Expect(err).To(BeNil())

			expectedMgr, err := ctrl.NewManager(cfg, opts)
			Expect(err).To(BeNil())
			Expect(gotMgr.GetConfig()).Should(Equal(expectedMgr.GetConfig()))
		})

	})
})
