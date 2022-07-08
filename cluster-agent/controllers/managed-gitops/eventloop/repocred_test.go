package eventloop

import (
	"context"
	"github.com/argoproj/argo-cd/v2/common"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db/util"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend/eventloop/eventlooptypes"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
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

	When("Test Setup for RepositoryCredential workflow for cluster-agent", func() {
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

		It("If there is a valid Operation ID and no secret, then create ArgoCD secret", func() {
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

			By("Get the secret")
			secret := &corev1.Secret{}
			err = task.event.client.Get(ctx, types.NamespacedName{Name: repositoryCredential.SecretObj, Namespace: namespace}, secret)
			Expect(err).To(BeNil())

			By("Checking the secret validity")
			Expect(secret.Data).Should(HaveKey("url"))
			Expect(secret.Data).Should(HaveKey("username"))
			Expect(secret.Data).Should(HaveKey("password"))
			Expect(secret.Labels).Should(HaveKey("argocd.argoproj.io/secret-type"))
			Expect(secret.Labels).Should(HaveKey(controllers.RepoCredDatabaseIDLabel))
			Expect(string(secret.Data["url"])).Should(Equal(repositoryCredential.PrivateURL))
			Expect(string(secret.Data["username"])).Should(Equal(repositoryCredential.AuthUsername))
			Expect(string(secret.Data["password"])).Should(Equal(repositoryCredential.AuthPassword))
			Expect(secret.Labels["argocd.argoproj.io/secret-type"]).Should(Equal("repository"))
			Expect(secret.Labels[controllers.RepoCredDatabaseIDLabel]).Should(Equal(repositoryCredential.RepositoryCredentialsID))
		})
		It("If there is a valid Operation ID for a problematic ArgoCD secret, fix the secret", func() {
			By("Create the ArgoCD secret with wrong values and argocd labels")
			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fake-secret-obj",
					Namespace: namespace,
				},
				Immutable: nil,
				Data: map[string][]byte{
					"url":      []byte("wrong-url"),
					"username": []byte("wrong-username"),
					"password": []byte("wrong-password"),
				},
			}

			err = task.event.client.Create(ctx, secret)
			Expect(err).To(BeNil())

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation-2",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-my-repo-creds-2", // this needs to be unique for this testcase!
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
			// Pass an already existing secret name
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

			By("Get the secret")
			secret = &corev1.Secret{}
			err = task.event.client.Get(ctx, types.NamespacedName{Name: repositoryCredential.SecretObj, Namespace: namespace}, secret)
			Expect(err).To(BeNil())

			By("Checking the secret validity")
			Expect(secret.Data).Should(HaveKey("url"))
			Expect(secret.Data).Should(HaveKey("username"))
			Expect(secret.Data).Should(HaveKey("password"))
			Expect(secret.Labels).Should(HaveKey("argocd.argoproj.io/secret-type"))
			Expect(secret.Labels).Should(HaveKey(controllers.RepoCredDatabaseIDLabel))
			Expect(string(secret.Data["url"])).Should(Equal(repositoryCredential.PrivateURL))
			Expect(string(secret.Data["username"])).Should(Equal(repositoryCredential.AuthUsername))
			Expect(string(secret.Data["password"])).Should(Equal(repositoryCredential.AuthPassword))
			Expect(secret.Labels["argocd.argoproj.io/secret-type"]).Should(Equal("repository"))
			Expect(secret.Labels[controllers.RepoCredDatabaseIDLabel]).Should(Equal(repositoryCredential.RepositoryCredentialsID))
		})
		It("If RepoCred doesn't exist, then delete the related secret", func() {
			By("Create the ArgoCD secret with a DatabaseID label")
			// Setup an ArgoCD Secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fake-secret-obj-2",
					Namespace: namespace,
					Labels: map[string]string{
						common.LabelKeySecretType:           common.LabelValueSecretTypeRepository,
						controllers.RepoCredDatabaseIDLabel: "cheat",
					},
					Annotations: map[string]string{
						common.AnnotationKeyManagedBy: common.AnnotationValueManagedByArgoCD,
					},
				},
			}

			err = task.event.client.Create(ctx, secret, &client.CreateOptions{})
			Expect(err).To(BeNil())

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation-3",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "cheat", // this needs to be unique for this testcase!
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

			// Do it
			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeFalse())

			By("Get the secret")
			err = task.event.client.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: namespace}, secret)
			Expect(err).ToNot(BeNil())

		})
		It("If RepoCred doesn't exist, then delete the related secrets", func() {
			By("Create the ArgoCD secret with a DatabaseID label")
			// Setup an ArgoCD Secret
			secret1 := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fake-secret-obj-1",
					Namespace: namespace,
					Labels: map[string]string{
						common.LabelKeySecretType:           common.LabelValueSecretTypeRepository,
						controllers.RepoCredDatabaseIDLabel: "cheat",
					},
					Annotations: map[string]string{
						common.AnnotationKeyManagedBy: common.AnnotationValueManagedByArgoCD,
					},
				},
			}

			secret2 := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fake-secret-obj-2",
					Namespace: namespace,
					Labels: map[string]string{
						common.LabelKeySecretType:           common.LabelValueSecretTypeRepository,
						controllers.RepoCredDatabaseIDLabel: "cheat", // <-- it has to have the same with secret1
					},
					Annotations: map[string]string{
						common.AnnotationKeyManagedBy: common.AnnotationValueManagedByArgoCD,
					},
				},
			}

			err = task.event.client.Create(ctx, secret1, &client.CreateOptions{})
			Expect(err).To(BeNil())

			err = task.event.client.Create(ctx, secret2, &client.CreateOptions{})
			Expect(err).To(BeNil())

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation-4",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "cheat", // this needs to be unique for this testcase!
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

			// Do it
			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeFalse())

			By("Get the secrets")
			err = task.event.client.Get(ctx, types.NamespacedName{Name: secret1.Name, Namespace: namespace}, secret1)
			Expect(err).ToNot(BeNil())
			err = task.event.client.Get(ctx, types.NamespacedName{Name: secret2.Name, Namespace: namespace}, secret2)
			Expect(err).ToNot(BeNil())

		})
		It("If RepoCred doesn't exist, and no leftovers, then NOP", func() {
			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation-4",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "cheat", // this needs to be unique for this testcase!
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

			// Do it
			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeFalse())

		})
	})
})
