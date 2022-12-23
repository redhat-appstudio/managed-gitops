package core

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"github.com/redhat-appstudio/managed-gitops/backend/util"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture/k8s"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = FDescribe("ArgoCD instance via GitOpsEngineInstance Operations Test", func() {

	const (
		argocdNamespace      = fixture.NewArgoCDInstanceNamespace
		argocdCRName         = "argocd"
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
			// err = db.SetupForTestingDBGinkgo()
			// Expect(err).To(BeNil())
			err = fixture.DeleteNamespace(argocdNamespace, config)
			Expect(err).To(BeNil())
			// err = fixture.DeleteNamespace(argocdCRName, config)
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
				Clusteruser_id: "test-user-234",
				User_name:      "test-user",
			}

			// task := eventloop.processOperationEventTask{
			// 	log: logger,
			// 	event: eventloop.operationEventLoopEvent{
			// 		request: newRequest(argocdNamespace, argocdNamespace),
			// 		client:  k8sClient,
			// 	},
			// }

			By("create a clusterUser and namespace for GitOpsEngineInstance where ArgoCD will be created")
			ctx := context.Background()
			log := log.FromContext(ctx)

			By("Creating gitopsengine cluster,cluster user and namespace")
			err = dbq.GetOrCreateSpecialClusterUser(ctx, testClusterUser)
			Expect(err).To(BeNil())

			namespaceCR := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocdNamespace,
					Namespace: argocdNamespace,
				},
			}
			err = k8sClient.Create(ctx, &namespaceCR)
			Expect(err).To(BeNil())

			// kubeSystemNamespace := &corev1.Namespace{
			// 	ObjectMeta: metav1.ObjectMeta{
			// 		Name: "kube-system",
			// 	},
			// }

			// gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, kubeSystemNamespace.Name, dbq, log)
			// Expect(err).To(BeNil())
			// gitopsEngineInstanceput := db.GitopsEngineInstance{
			// 	Gitopsengineinstance_id: "test-fake-engine-instance-id-6543",
			// 	Namespace_name:          namespaceCR.Name,
			// 	Namespace_uid:           kubeSystemNamespace.Name,
			// 	EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			// }

			// err = dbq.CreateGitopsEngineInstance(ctx, &gitopsEngineInstanceput)
			// Expect(err).To(BeNil())

			err = util.CreateNewArgoCDInstance(&namespaceCR, *testClusterUser, "test-operation", k8sClient, log, dbq)
			Expect(err).To(BeNil())

			By("creating Operation row in database")
			// operationDB := &db.Operation{
			// 	Operation_id:            "test-operation",
			// 	Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
			// 	Resource_id:             "test-fake-resource-id",
			// 	Resource_type:           db.OperationResourceType_GitOpsEngineInstance,
			// 	State:                   db.OperationState_Waiting,
			// 	Operation_owner_user_id: testClusterUser.Clusteruser_id,
			// }

			// err = dbq.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			// Expect(err).To(BeNil())

			By("creating Operation CR")
			operationCR := &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      argocdNamespace,
					Namespace: argocdNamespace,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: "test-operation",
				},
			}

			err = k8s.Create(operationCR, k8sClient)
			Expect(err).To(BeNil())

			By("ensuring ArgoCD service resource exists")
			argocdInstance := &apps.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: argocdNamespace + "-server", Namespace: argocdNamespace},
			}

			Eventually(argocdInstance, "60s", "5s").Should(k8s.ExistByName(k8sClient))
			Expect(err).To(BeNil())

		})
	})
})
