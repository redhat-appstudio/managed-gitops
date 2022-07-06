package eventloop

import (
	"context"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

			By("Get a clusterEngineInstance")
			scheme, argocdNamespace, kubesystemNamespace, workspace, err := eventlooptypes.GenericTestSetup()
			Expect(err).To(BeNil())

			_, _, _, _, _, err = db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbq, logger)
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(err).To(BeNil())

			gitopsEngineInstance = &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance",
				Namespace_name:          argocdNamespace.Name,
				Namespace_uid:           string(argocdNamespace.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbq.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
			Expect(err).To(BeNil())

			By("Satisfying the foreign key constraint 'fk_clusteruser_id'")
			clusterUser = &db.ClusterUser{
				Clusteruser_id: "test-repocred-user-id",
				User_name:      "test-repocred-user",
			}
			err = dbq.CreateClusterUser(ctx, clusterUser)
			Expect(err).To(BeNil())

			// To work with Kubernetes you need a client and a task
			By("Initialize fake kube client")

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

		It("Ensure that calling perform task on an operation CR for RepositoryCredential that doesn't exist, it doesn't return an error, and retry is false", func() {
			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-my-repo-creds", // this needs to be unique for this testcase!
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

			// Create a fake RepositoryCredential (make sure you don't violate the constraints or the test will fail)
			By("Creating a fake RepositoryCredential")
			repositoryCredential := db.RepositoryCredentials{
				RepositoryCredentialsID: operationDB.Resource_id,
				UserID:                  clusterUser.Clusteruser_id, // comply with the constraint 'fk_clusteruser_id'
				PrivateURL:              "test-fake-private-url",
				AuthUsername:            "test-fake-auth-username",
				AuthPassword:            "test-fake-auth-password",
				AuthSSHKey:              "",
				SecretObj:               "test-fake-secret-obj",
				EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id, // comply with the constraint 'fk_gitopsengineinstance_id'
			}

			// Inject the fake RepositoryCredential into the database
			By("Injecting the fake RepositoryCredential into the database")
			err = dbq.CreateRepositoryCredentials(ctx, &repositoryCredential)
			Expect(err).To(BeNil())

			By("Getting the RepositoryCredentials object from the database")
			fetch, err := dbq.GetRepositoryCredentialsByID(ctx, operationDB.Resource_id)
			Expect(err).To(BeNil())
			Expect(fetch).Should(Equal(repositoryCredential))

			// Do it
			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeFalse())

			// Get the secret and check it
			By("Get the secret")
			secret := &corev1.Secret{}
			err = task.event.client.Get(ctx, types.NamespacedName{Name: repositoryCredential.SecretObj, Namespace: namespace}, secret)
			Expect(err).To(BeNil())
			Expect(secret.Data).Should(HaveKey("url"))
			Expect(secret.Data).Should(HaveKey("username"))
			Expect(secret.Data).Should(HaveKey("password"))
			Expect(string(secret.Data["url"])).Should(Equal(repositoryCredential.PrivateURL))
			Expect(string(secret.Data["username"])).Should(Equal(repositoryCredential.AuthUsername))
			Expect(string(secret.Data["password"])).Should(Equal(repositoryCredential.AuthPassword))

			//// New Test Case: Change the secret
			//// I have to somehow change the status back to "Progressing"
			//// otherwise: --> Skipping Operation with state of completed/failed	{"operationId": "test-operation"}
			//By("Changing the status of the operation to Waiting")
			//operationDB.State = db.OperationState_Waiting
			//err = dbq.UpdateOperation(ctx, operationDB)
			//Expect(err).To(BeNil())
			//
			//By("creating Operation CR")
			//operationCR = &operation.Operation{
			//	ObjectMeta: metav1.ObjectMeta{
			//		Name:      name,
			//		Namespace: namespace,
			//	},
			//	Spec: operation.OperationSpec{
			//		OperationID: operationDB.Operation_id,
			//	},
			//}
			//
			//err = task.event.client.Update(ctx, operationCR)
			//Expect(err).To(BeNil())
			//
			//By("Changing the secret")
			//secret.Data["url"] = []byte("test-fake-private-url-2")
			//secret.Data["username"] = []byte("test-fake-auth-username-2")
			//secret.Data["password"] = []byte("test-fake-auth-password-2")
			//err = task.event.client.Update(ctx, secret)
			//
			//// Run again the task, it should fix the secret
			//By("Running again the task, it should fix the secret")
			//retry, err = task.PerformTask(ctx)
			//Expect(err).To(BeNil())
			//Expect(retry).To(BeFalse())
			//
			//// Check the secret if it's OK (in sync with the database)
			//By("Get the secret")
			//err = task.event.client.Get(ctx, types.NamespacedName{Name: repositoryCredential.SecretObj, Namespace: namespace}, secret)
			//Expect(err).To(BeNil())
			//By("Check the secret if it's OK (in sync with the database)")
			//Expect(secret.Data).Should(HaveKey("url"))
			//Expect(secret.Data).Should(HaveKey("username"))
			//Expect(secret.Data).Should(HaveKey("password"))
			//Expect(string(secret.Data["url"])).Should(Equal(repositoryCredential.PrivateURL))
			//Expect(string(secret.Data["username"])).Should(Equal(repositoryCredential.AuthUsername))
			//Expect(string(secret.Data["password"])).Should(Equal(repositoryCredential.AuthPassword))

		})
	})
})
