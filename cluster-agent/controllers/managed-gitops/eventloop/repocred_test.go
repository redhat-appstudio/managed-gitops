package eventloop

import (
	"context"
	"time"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/operations"

	"github.com/argoproj/argo-cd/v2/common"
	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Testing Repository Credentials Operation", func() {
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

	// Set up the test
	BeforeEach(func() {
		By("Connecting to the database")
		err = db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())

		ctx = context.Background()
		logger = log.FromContext(ctx) // do this, or invalid memory address or nil pointer dereference will occur

		By("Get a clusterEngineInstance")
		scheme, argocdNamespace, kubesystemNamespace, workspace, err := tests.GenericTestSetup()
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

		gitopsDepl := &operation.GitOpsDeployment{
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
				request: newRequest(namespace, name), // NOTE: Make sure you replace the name with the name of the operation, later in the tests
				client:  k8sClient,
			},
		}

	})

	// Clean up the test
	AfterEach(func() {
		By("Closing the database connection")
		dbq.CloseDatabase() // Close the database connection.
	})

	// Scenario 1
	Describe("Creating an ArgoCD secret for Private Repositories based on RepositoryCredentials DB row", func() {
		When("Operation DB row and its corresponding Operation CR, point to an existing RepositoryCredentials DB row", func() {
			var operationDB *db.Operation
			var operationCR *operation.Operation
			var repositoryCredential db.RepositoryCredentials

			BeforeEach(func() {
				By(" --- creating an Operation DB row and CR with a valid Resource_id ---")
				dbOperationInput := db.Operation{
					Operation_id:            "test-operation",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             "test-my-repo-creds", // this needs to be unique for this testcase!
					Resource_type:           db.OperationResourceType_RepositoryCredentials,
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: clusterUser.Clusteruser_id,
				}

				// Create an Operation DB Row and a corresponding Operation CR
				operationCR, operationDB, err = operations.CreateOperation(ctx, false, dbOperationInput, clusterUser.Clusteruser_id, namespace, dbq, k8sClient, logger)
				Expect(err).To(BeNil())
				task.event.request.Name = operationCR.Name // Correct the task's Operation CR name request
				logger.Info("operationCR", "operationCR", operationCR)
				logger.Info("operationDB", "operationDB", operationDB)

				By(" --- creating a RepositoryCredentials DB row with Resource_id as its PrimaryKey ---")
				repositoryCredential = db.RepositoryCredentials{
					RepositoryCredentialsID: operationDB.Resource_id,
					UserID:                  clusterUser.Clusteruser_id, // comply with the constraint 'fk_clusteruser_id'
					PrivateURL:              "test-fake-private-url",
					AuthUsername:            "test-fake-auth-username",
					AuthPassword:            "test-fake-auth-password",
					AuthSSHKey:              "test-fake-ssh-key",
					SecretObj:               "test-fake-secret-obj",
					EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id, // comply with the constraint 'fk_gitopsengineinstance_id'
				}

				By(" --- injecting the fake RepositoryCredential into the database ---")
				err = dbq.CreateRepositoryCredentials(ctx, &repositoryCredential)
				Expect(err).To(BeNil())

				By(" --- getting the RepositoryCredentials object from the database ---")
				fetch, err := dbq.GetRepositoryCredentialsByID(ctx, operationDB.Resource_id)
				Expect(err).To(BeNil())
				Expect(fetch.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
				fetch.Created_on = repositoryCredential.Created_on
				Expect(fetch).Should(Equal(repositoryCredential))
			})

			It("Should create an ArgoCD Secret for Private Repository based on the RepositoryCredentials DB row data", func() {

				// Create a fake RepositoryCredential (make sure you don't violate the constraints or the test will fail)

				By(" --- calling processOperation_RepositoryCredentials() ---")
				retry, err := task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				By(" --- getting the secret ---")
				secret := &corev1.Secret{}
				err = task.event.client.Get(ctx, types.NamespacedName{Name: repositoryCredential.SecretObj, Namespace: namespace}, secret)
				Expect(err).To(BeNil())

				By(" --- checking secret compatibility with ArgoCD ---")
				Expect(secret.Data).Should(HaveKey("url"))
				Expect(secret.Data).Should(HaveKey("username"))
				Expect(secret.Data).Should(HaveKey("password"))
				Expect(secret.Labels).Should(HaveKey(common.LabelKeySecretType))
				Expect(secret.Annotations).Should(HaveKey(common.AnnotationKeyManagedBy))
				Expect(secret.Annotations[common.AnnotationKeyManagedBy]).Should(Equal(common.AnnotationValueManagedByArgoCD))
				Expect(secret.Labels[common.LabelKeySecretType]).Should(Equal(common.LabelValueSecretTypeRepository))

				By(" --- checking secret data with RepositoryCredentials DB row ---")
				Expect(secret.Labels).Should(HaveKey(controllers.RepoCredDatabaseIDLabel))
				Expect(string(secret.Data["url"])).Should(Equal(repositoryCredential.PrivateURL))
				Expect(string(secret.Data["username"])).Should(Equal(repositoryCredential.AuthUsername))
				Expect(string(secret.Data["password"])).Should(Equal(repositoryCredential.AuthPassword))
				Expect(secret.Labels[controllers.RepoCredDatabaseIDLabel]).Should(Equal(repositoryCredential.RepositoryCredentialsID))

				By(" --- checking Operation DB status ---")
				err = dbq.GetOperationById(ctx, operationDB)
				Expect(err).To(BeNil())
				Expect(operationDB.State).Should(Equal(db.OperationState_Completed))
			})
		})
	})

	// Scenario 2
	Describe("Fixing an already existing ArgoCD secret for Private Repositories based on RepositoryCredentials DB row", func() {
		When("Operation DB row and its corresponding Operation CR, point to an existing RepositoryCredentials DB row", func() {
			var operationDB *db.Operation
			var operationCR *operation.Operation
			var repositoryCredential db.RepositoryCredentials
			var secret *corev1.Secret

			BeforeEach(func() {
				By(" --- create the ArgoCD secret with wrong values and missing argocd labels & annotations ---")
				secret = &corev1.Secret{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-fake-secret-wrong-values",
						Namespace: namespace,
					},
					Immutable: nil,
					Data: map[string][]byte{
						"url":      []byte("wrong-url"),
						"username": []byte("wrong-username"),
						"password": []byte("wrong-password"),
						"ssh":      []byte("wrong-ssh-key"),
					},
				}

				err = task.event.client.Create(ctx, secret)
				Expect(err).To(BeNil())

				By(" --- creating an Operation DB row and CR with a valid Resource_id ---")
				dbOperationInput := db.Operation{
					Operation_id:            "test-operation-2",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             "test-my-repo-creds-2", // this needs to be unique for this testcase!
					Resource_type:           db.OperationResourceType_RepositoryCredentials,
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: clusterUser.Clusteruser_id,
				}

				operationCR, operationDB, err = operations.CreateOperation(ctx, false, dbOperationInput, clusterUser.Clusteruser_id, namespace, dbq, k8sClient, logger)
				Expect(err).To(BeNil())
				task.event.request.Name = operationCR.Name // Correct the task's Operation CR name request

				By(" --- creating a RepositoryCredentials DB row with Resource_id as its PrimaryKey ---")
				repositoryCredential = db.RepositoryCredentials{
					RepositoryCredentialsID: operationDB.Resource_id,
					UserID:                  clusterUser.Clusteruser_id, // comply with the constraint 'fk_clusteruser_id'
					PrivateURL:              "test-fake-correct-url",
					AuthUsername:            "test-fake-correct-auth-username",
					AuthPassword:            "test-fake-correct-auth-password",
					AuthSSHKey:              "test-fake-correct-auth-ssh-key",
					SecretObj:               "test-fake-secret-wrong-values",
					EngineClusterID:         gitopsEngineInstance.Gitopsengineinstance_id, // comply with the constraint 'fk_gitopsengineinstance_id'
				}

				By(" --- injecting the fake RepositoryCredential into the database ---")
				err = dbq.CreateRepositoryCredentials(ctx, &repositoryCredential)
				Expect(err).To(BeNil())

				By(" --- getting the RepositoryCredentials object from the database ---")
				fetch, err := dbq.GetRepositoryCredentialsByID(ctx, operationDB.Resource_id)
				Expect(err).To(BeNil())
				Expect(fetch.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
				fetch.Created_on = repositoryCredential.Created_on
				Expect(fetch).Should(Equal(repositoryCredential))
			})

			It("Should update the existing ArgoCD Secret for Private Repository based on the RepositoryCredentials DB row data", func() {

				// Create a fake RepositoryCredential (make sure you don't violate the constraints or the test will fail)

				By(" --- calling processOperation_RepositoryCredentials() ---")
				retry, err := task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				By(" --- getting the secret ---")
				secret := &corev1.Secret{}
				err = task.event.client.Get(ctx, types.NamespacedName{Name: repositoryCredential.SecretObj, Namespace: namespace}, secret)
				Expect(err).To(BeNil())

				By(" --- checking secret compatibility with ArgoCD ---")
				Expect(secret.Data).Should(HaveKey("url"))
				Expect(secret.Data).Should(HaveKey("username"))
				Expect(secret.Data).Should(HaveKey("password"))
				Expect(secret.Labels).Should(HaveKey(common.LabelKeySecretType))
				Expect(secret.Annotations).Should(HaveKey(common.AnnotationKeyManagedBy))
				Expect(secret.Annotations[common.AnnotationKeyManagedBy]).Should(Equal(common.AnnotationValueManagedByArgoCD))
				Expect(secret.Labels[common.LabelKeySecretType]).Should(Equal(common.LabelValueSecretTypeRepository))

				By(" --- checking secret data with RepositoryCredentials DB row ---")
				Expect(secret.Labels).Should(HaveKey(controllers.RepoCredDatabaseIDLabel))
				Expect(string(secret.Data["url"])).Should(Equal(repositoryCredential.PrivateURL))
				Expect(string(secret.Data["username"])).Should(Equal(repositoryCredential.AuthUsername))
				Expect(string(secret.Data["password"])).Should(Equal(repositoryCredential.AuthPassword))
				Expect(secret.Labels[controllers.RepoCredDatabaseIDLabel]).Should(Equal(repositoryCredential.RepositoryCredentialsID))

				By(" --- checking Operation DB status ---")
				err = dbq.GetOperationById(ctx, operationDB)
				Expect(err).To(BeNil())
				Expect(operationDB.State).Should(Equal(db.OperationState_Completed))
			})
		})
	})

	// Scenario 3
	Describe("Deleting ArgoCD related leftover", func() {
		When("Operation DB row and its corresponding Operation CR, point to a missing RepositoryCredentials DB row", func() {
			var operationDB *db.Operation
			var operationCR *operation.Operation
			var secret *corev1.Secret

			BeforeEach(func() {
				By(" --- creating the ArgoCD secret with wrong values and missing argocd labels & annotations ---")
				secret = &corev1.Secret{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-fake-obsolete-secret",
						Namespace: namespace,
						Labels: map[string]string{
							common.LabelKeySecretType:           common.LabelValueSecretTypeRepository,
							controllers.RepoCredDatabaseIDLabel: "test-my-repo-creds-3", // has to be the same with the one in the Operation DB
						},
						Annotations: map[string]string{
							common.AnnotationKeyManagedBy: common.AnnotationValueManagedByArgoCD,
						},
					},
					Immutable: nil,
					Data: map[string][]byte{
						"url":      []byte("obsolete-url"),
						"username": []byte("obsolete-username"),
						"password": []byte("obsolete-password"),
						"ssh":      []byte("obsolete-ssh-key"),
					},
				}

				err = task.event.client.Create(ctx, secret)
				Expect(err).To(BeNil())

				By(" --- creating an Operation DB row and CR with a valid Resource_id ---")
				dbOperationInput := db.Operation{
					Operation_id:            "test-operation-3",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             "test-my-repo-creds-3", // this needs to be unique for this testcase!
					Resource_type:           db.OperationResourceType_RepositoryCredentials,
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: clusterUser.Clusteruser_id,
				}

				operationCR, operationDB, err = operations.CreateOperation(ctx, false, dbOperationInput, clusterUser.Clusteruser_id, namespace, dbq, k8sClient, logger)
				Expect(err).To(BeNil())
				task.event.request.Name = operationCR.Name // Correct the task's Operation CR name request
			})

			It("Should delete the existing related ArgoCD Secret for Private Repository", func() {

				By(" --- calling processOperation_RepositoryCredentials() ---")
				retry, err := task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				By(" --- getting the secret ---")
				secret := &corev1.Secret{}
				err = task.event.client.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: namespace}, secret)
				Expect(err).ToNot(BeNil())

				By(" --- checking Operation DB status ---")
				err = dbq.GetOperationById(ctx, operationDB)
				Expect(err).To(BeNil())
				Expect(operationDB.State).Should(Equal(db.OperationState_Completed))
			})
		})
	})

	// Scenario 4
	Describe("Deleting all ArgoCD related leftovers", func() {
		When("Operation DB row and its corresponding Operation CR, point to a missing RepositoryCredentials DB row", func() {
			var operationDB *db.Operation
			var operationCR *operation.Operation
			var secret1 *corev1.Secret
			var secret2 *corev1.Secret

			BeforeEach(func() {
				By(" --- creating the ArgoCD secret 1 with wrong values and missing argocd labels & annotations ---")
				secret1 = &corev1.Secret{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-fake-obsolete-secret-1",
						Namespace: namespace,
						Labels: map[string]string{
							common.LabelKeySecretType:           common.LabelValueSecretTypeRepository,
							controllers.RepoCredDatabaseIDLabel: "test-my-repo-creds-4", // has to be the same with the one in the Operation DB
						},
						Annotations: map[string]string{
							common.AnnotationKeyManagedBy: common.AnnotationValueManagedByArgoCD,
						},
					},
					Immutable: nil,
					Data: map[string][]byte{
						"url":      []byte("obsolete-url"),
						"username": []byte("obsolete-username"),
						"password": []byte("obsolete-password"),
						"ssh":      []byte("obsolete-ssh-key"),
					},
				}

				err = task.event.client.Create(ctx, secret1)
				Expect(err).To(BeNil())

				By(" --- creating the ArgoCD secret 2 with wrong values and missing argocd labels & annotations ---")
				secret2 = &corev1.Secret{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-fake-obsolete-secret-2",
						Namespace: namespace,
						Labels: map[string]string{
							common.LabelKeySecretType:           common.LabelValueSecretTypeRepository,
							controllers.RepoCredDatabaseIDLabel: "test-my-repo-creds-4", // has to be the same with the one in the Operation DB
						},
						Annotations: map[string]string{
							common.AnnotationKeyManagedBy: common.AnnotationValueManagedByArgoCD,
						},
					},
					Immutable: nil,
					Data: map[string][]byte{
						"url":      []byte("obsolete-url"),
						"username": []byte("obsolete-username"),
						"password": []byte("obsolete-password"),
						"ssh":      []byte("obsolete-ssh-key"),
					},
				}

				err = task.event.client.Create(ctx, secret2)
				Expect(err).To(BeNil())

				By(" --- creating an Operation DB row and CR with a valid Resource_id ---")
				dbOperationInput := db.Operation{
					Operation_id:            "test-operation-4",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             "test-my-repo-creds-4", // this needs to be unique for this testcase!
					Resource_type:           db.OperationResourceType_RepositoryCredentials,
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: clusterUser.Clusteruser_id,
				}

				operationCR, operationDB, err = operations.CreateOperation(ctx, false, dbOperationInput, clusterUser.Clusteruser_id, namespace, dbq, k8sClient, logger)
				Expect(err).To(BeNil())
				task.event.request.Name = operationCR.Name // Correct the task's Operation CR name request
			})

			It("Should delete all the existing related ArgoCD Secrets for Private Repository", func() {

				By(" --- calling processOperation_RepositoryCredentials() ---")
				retry, err := task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				By(" --- getting the first secret ---")
				err = task.event.client.Get(ctx, types.NamespacedName{Name: secret1.Name, Namespace: namespace}, secret1)
				Expect(err).ToNot(BeNil())

				By(" --- getting the second secret ---")
				err = task.event.client.Get(ctx, types.NamespacedName{Name: secret2.Name, Namespace: namespace}, secret2)
				Expect(err).ToNot(BeNil())

				By(" --- checking Operation DB status ---")
				err = dbq.GetOperationById(ctx, operationDB)
				Expect(err).To(BeNil())
				Expect(operationDB.State).Should(Equal(db.OperationState_Completed))
			})
		})
	})

	// Scenario 5
	Describe("No action for no ArgoCD leftovers", func() {
		When("Operation DB row and its corresponding Operation CR, point to a missing RepositoryCredentials DB row", func() {
			var operationDB *db.Operation
			var operationCR *operation.Operation

			BeforeEach(func() {

				By(" --- creating an Operation DB row and a CR with a valid Resource_id ---")
				dbOperationInput := db.Operation{
					Operation_id:            "test-operation-5",
					Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
					Resource_id:             "test-my-repo-creds-5", // this needs to be unique for this testcase!
					Resource_type:           db.OperationResourceType_RepositoryCredentials,
					State:                   db.OperationState_Waiting,
					Operation_owner_user_id: clusterUser.Clusteruser_id,
				}

				operationCR, operationDB, err = operations.CreateOperation(ctx, false, dbOperationInput, clusterUser.Clusteruser_id, namespace, dbq, k8sClient, logger)
				Expect(err).To(BeNil())
				task.event.request.Name = operationCR.Name // Correct the task's Operation CR name request
			})

			It("Should do nothing (NOP)", func() {

				By(" --- calling processOperation_RepositoryCredentials() ---")
				retry, err := task.PerformTask(ctx)
				Expect(err).To(BeNil())
				Expect(retry).To(BeFalse())

				By(" --- checking Operation DB status ---")
				err = dbq.GetOperationById(ctx, operationDB)
				Expect(err).To(BeNil())
				Expect(operationDB.State).Should(Equal(db.OperationState_Completed))
			})
		})
	})
})
