package core

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	"github.com/redhat-appstudio/managed-gitops/backend/util"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("ArgoCD instance via GitOpsEngineInstance Operations Test", func() {

	const (
		argocdNamespace = fixture.NewArgoCDInstanceNamespace
	)

	Context("ArgoCD instance gets created from an operation's gitopsEngineInstance resource-type", func() {

		BeforeEach(func() {
			By("Delete old namespaces, and kube-system resources")
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

		})

		It("ensures that a standalone ArgoCD gets created successfully when an operation CR of resource-type GitOpsEngineInstance is created", func() {

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()
			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			testClusterUser := &db.ClusterUser{
				Clusteruser_id: "test-user",
				User_name:      "test-user",
			}
			ctx := context.Background()
			log := log.FromContext(ctx)

			By("Creating gitopsengine cluster,cluster user and namespace")

			newArgoCDNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: argocdNamespace,
				},
			}
			err = k8sClient.Create(ctx, newArgoCDNamespace)
			Expect(err).To(BeNil())

			err = util.CreateNewArgoCDInstance(ctx, newArgoCDNamespace, *testClusterUser, k8sClient, log, dbq)
			Expect(err).To(BeNil())

			By("ensuring ArgoCD service resource exists")
			argocdInstance := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: newArgoCDNamespace.Name + "-server", Namespace: newArgoCDNamespace.Name},
			}

			Eventually(argocdInstance, "10m", "5s").Should(k8s.ExistByName(k8sClient), "Argo CD server Deployment should exist")
		})
	})
})
