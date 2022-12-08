package core

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	util "github.com/redhat-appstudio/managed-gitops/backend/util"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("ArgoCD instance via GitOpsEngineInstance Operations Test", func() {

	const (
		argocdNamespace      = fixture.NewArgoCDInstanceNamespace
		argocdCRName         = "argocd-gitopsengine-test"
		destinationNamespace = fixture.NewArgoCDInstanceDestNamespace
	)

	Context("ArgoCD instance gets created from an operation's gitopsEngineInstance resource-type", func() {

		BeforeEach(func() {

			By("Delete old namespaces, and kube-system resources")
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			By("deleting the namespace before the test starts, so that the code can create it")
			config, err := fixture.GetSystemKubeConfig()
			if err != nil {
				panic(err)
			}
			err = fixture.DeleteNamespace(argocdNamespace, config)
			Expect(err).To(BeNil())
			err = fixture.DeleteNamespace(argocdCRName, config)
			Expect(err).To(BeNil())

		})

		It("ensures that a standalone ArgoCD gets created successfully when an operation CR of resource-type GitOpsEngineInstance is created", func() {

			if fixture.IsRunningAgainstKCP() {
				Skip("Skipping this test until we support running gitops operator with KCP")
			}

			dbq, err := db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())
			defer dbq.CloseDatabase()
			k8sClient, err := fixture.GetE2ETestUserWorkspaceKubeClient()
			Expect(err).To(Succeed())

			By("create a clusterUser and namespace for GitOpsEngineInstance where ArgoCD will be created")
			ctx := context.Background()
			log := log.FromContext(ctx)

			namespaceCR := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocdCRName,
					Namespace: argocdNamespace,
				},
			}
			err = k8s.Create(namespaceCR, k8sClient)
			Expect(err).To(BeNil())
			clusterUser := db.ClusterUser{User_name: "test-gitops-service-user"}
			dbq.CreateClusterUser(ctx, &clusterUser)

			err = util.CreateNewArgoCDInstance(namespaceCR, clusterUser, k8sClient, log, dbq)
			Expect(err).To(BeNil())

			By("ensuring ArgoCD service resource exists")
			argocdInstance := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: argocdCRName + "-server", Namespace: argocdNamespace},
			}

			Eventually(argocdInstance, "60s", "5s").Should(k8s.ExistByName(k8sClient))
			Expect(err).To(BeNil())

		})
	})
})
