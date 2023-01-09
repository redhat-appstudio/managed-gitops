package core

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/redhat-appstudio/managed-gitops/backend/util"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = FDescribe("ArgoCD instance via GitOpsEngineInstance Operations Test", func() {

	const (
		workspace = "my-user"
	)

	Context("ArgoCD instance gets created from an operation's gitopsEngineInstance resource-type", func() {

		BeforeEach(func() {

			By("deleting the namespace before the test starts, so that the code can create it")
			config, err := fixture.GetSystemKubeConfig()
			if err != nil {
				panic(err)
			}

			err = fixture.DeleteNamespace(workspace, config)
			Expect(err).To(BeNil())

			// _, argocdNamespace, _, workspace, err = tests.GenericTestSetup()
			// Expect(err).To(BeNil())

		})

		It("ensures that a standalone ArgoCD gets created successfully when an operation CR of resource-type GitOpsEngineInstance is created", func() {
			// var logger logr.Logger

			if fixture.IsRunningAgainstKCP() {
				Skip("Skipping this test until we support running gitops operator with KCP")
			}

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			testClusterUser := &db.ClusterUser{
				Clusteruser_id: "test-user-new",
				User_name:      "test-user-new",
			}

			By("create a clusterUser and namespace for GitOpsEngineInstance where ArgoCD will be created")
			ctx := context.Background()
			log := log.FromContext(ctx)

			By("Creating gitopsengine cluster,cluster user and namespace")
			err = dbq.CreateClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())

			workspace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspace,
					UID:  uuid.NewUUID(),
				},
				Spec: corev1.NamespaceSpec{},
			}

			err = k8sClient.Create(ctx, workspace)
			Expect(err).To(BeNil())

			err = util.CreateNewArgoCDInstance(ctx, workspace, *testClusterUser, "test-operation", k8sClient, log, dbq)
			Expect(err).To(BeNil())

			By("ensuring ArgoCD service resource exists")
			argocdInstance := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: workspace.Name + "-server", Namespace: workspace.Name},
			}

			Eventually(argocdInstance, "60s", "5s").Should(k8s.ExistByName(k8sClient))
			Expect(err).To(BeNil())

		})
	})
})
