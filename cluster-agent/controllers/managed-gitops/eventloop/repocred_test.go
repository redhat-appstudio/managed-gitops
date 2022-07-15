package eventloop

import (
	"context"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("RepositoryCredentials Operation Tests", func() {
	const (
		name      = "operation"
		namespace = "gitops-service-argocd"
	)
	var (
		err                  error
		ctx                  context.Context
		clusterUser          *db.ClusterUser
		task                 processOperationEventTask
		gitopsEngineInstance *db.GitopsEngineInstance
		dbq                  db.AllDatabaseQueries
		k8sClient            client.WithWatch
		logger               logr.Logger
	)

	When("When ClusterUser, ClusterCredentials, GitopsEngine and GitopsInstance exist", func() {
		BeforeEach(func() {
			By("Connecting to the database")
			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			ctx = context.Background()
			logger = log.FromContext(ctx) // do this, or invalid memory address or nil pointer dereference will occur

			_, _, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			By("Satisfying the foreign key constraint 'fk_clusteruser_id'")
			// aka: ClusterUser.Clusteruser_id) required for 'repo_cred_user_id'

			clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-repocred-user-id",
				User_name:      "test-repocred-user",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			// To work with Kubernetes you need a client and a task
			By("Initialize fake kube client")
			//var workspace *v1.Namespace
			//var argocdNamespace *v1.Namespace
			//var kubesystemNamespace *v1.Namespace

			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()

			task = processOperationEventTask{
				log: logger,
				event: operationEventLoopEvent{
					request: newRequest(namespace, name),
					client:  k8sClient,
				},
			}

		})

		AfterEach(func() {
			By("Closing the database connection")
			dbq.CloseDatabase() // Close the database connection.
		})

		FIt("Ensure that calling perform task on an operation CR for RepositoryCredential that doesn't exist, it doesn't return an error, and retry is false", func() {
			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           db.OperationResourceType_RepositoryCredentials,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: clusterUser.Clusteruser_id,
			}

			err = dbq.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("creating Operation CR")
			operationCR := &operation.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: operation.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}
			err = task.event.client.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeFalse())
		})
	})
})
