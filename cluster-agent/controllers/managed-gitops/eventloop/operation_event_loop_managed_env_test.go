package eventloop

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	argosharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util/argocd"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/controllers"
)

var _ = Describe("Managed Environment Operation Tests", func() {
	const (
		operationName      = "operation"
		operationNamespace = "argocd"
	)
	Context("Reconciling Operation CRs pointing to Managed Environments", func() {

		var ctx context.Context
		var dbQueries db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var task processOperationEventTask
		var logger logr.Logger
		var kubesystemNamespace *corev1.Namespace
		var argocdNamespace *corev1.Namespace
		var workspace *corev1.Namespace
		var scheme *runtime.Scheme
		var testClusterUser *db.ClusterUser
		var err error
		var gitopsEngineCluster *db.GitopsEngineCluster
		var gitopsEngineInstance *db.GitopsEngineInstance

		BeforeEach(func() {
			ctx = context.Background()
			logger = log.FromContext(ctx)

			testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user",
				User_name:      "test-user",
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			scheme, argocdNamespace, kubesystemNamespace, workspace, err = tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			By("Initialize fake kube client")
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, argocdNamespace, kubesystemNamespace).Build()

			task = processOperationEventTask{
				log: logger,
				event: operationEventLoopEvent{
					request: newRequest(operationNamespace, operationName),
					client:  k8sClient,
				},
			}

			gitopsEngineCluster, _, err = dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(err).To(BeNil())

			By("creating a gitops engine instance")
			gitopsEngineInstance = &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance",
				Namespace_name:          argocdNamespace.Name,
				Namespace_uid:           string(argocdNamespace.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
			Expect(err).To(BeNil())

		})

		It("reconciles an operation that points to a managed environment that still exists, to ensure no action is taken in this case", func() {

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: string(uuid.NewUUID()),
			}

			err = dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			managedEnvRow := db.ManagedEnvironment{
				Managedenvironment_id: "test-fake-managed-env",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
				Name:                  "my-managed-env",
			}

			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvRow)
			Expect(err).To(BeNil())

			By("creating Operation row pointing to ManagedEnvironment")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             managedEnvRow.Managedenvironment_id,
				Resource_type:           db.OperationResourceType_ManagedEnvironment,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("creating Operation CR pointing to Operation row")
			operationCR := &operation.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operationName,
					Namespace: operationNamespace,
				},
				Spec: operation.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}
			err = task.event.client.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			By("creating an Argo CD Cluster secret, which we will will test to make sure it has not been deleted.")
			clusterSecretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: managedEnvRow.Managedenvironment_id})
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterSecretName,
					Namespace: argocdNamespace.Name,
					Labels: map[string]string{
						ArgoCDSecretTypeKey:                            ArgoCDSecretTypeValue_ClusterSecret,
						controllers.ArgoCDClusterSecretDatabaseIDLabel: managedEnvRow.Managedenvironment_id,
					},
				},
				Data: map[string][]byte{},
			}

			err = task.event.client.Create(ctx, secret)
			Expect(err).To(BeNil())

			retry, err := task.PerformTask(ctx)
			Expect(err).ToNot(BeNil(), "an error is expected here, because an operation on a managedenvironment is only supported if the managed environment doesn't exist")
			Expect(retry).To(BeFalse())

			err = task.event.client.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			Expect(err).To(BeNil(), "the Argo CD cluster secret should not have been deleted.")

		})

		It("Reconciling a deleted managed environment, to ensure the corresponding Argo CD cluster secret is deleted", func() {
			defer dbQueries.CloseDatabase()

			// We are simulated a managed environment whose database row was already deleted. This
			// was the primary key of that deleted row.
			deletedManagedEnvId := "test-managed-env-id"

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             deletedManagedEnvId,
				Resource_type:           db.OperationResourceType_ManagedEnvironment,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("creating Operation CR")
			operationCR := &operation.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operationName,
					Namespace: operationNamespace,
				},
				Spec: operation.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}
			err = task.event.client.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			By("creating an Argo CD Cluster secret, which we will will test to make sure it is deleted")
			clusterSecretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: deletedManagedEnvId})
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterSecretName,
					Namespace: argocdNamespace.Name,
					Labels: map[string]string{
						ArgoCDSecretTypeKey:                            ArgoCDSecretTypeValue_ClusterSecret,
						controllers.ArgoCDClusterSecretDatabaseIDLabel: deletedManagedEnvId,
					},
				},
				Data: map[string][]byte{},
			}

			err = task.event.client.Create(ctx, secret)
			Expect(err).To(BeNil())

			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeFalse())

			err = task.event.client.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			Expect(apierr.IsNotFound(err)).To(BeTrue(), "the Argo CD cluster secret should have been deleted.")

			err = expectOperationIsComplete(ctx, operationDB.Operation_id, dbQueries)
			Expect(err).To(BeNil())
		})
	})

	Context("Operations on an Application that reference managed environments", func() {

		var ctx context.Context
		var dbQueries db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var task processOperationEventTask
		var logger logr.Logger
		var kubesystemNamespace *corev1.Namespace
		var argocdNamespace *corev1.Namespace
		var workspace *corev1.Namespace
		var scheme *runtime.Scheme
		var testClusterUser *db.ClusterUser
		var err error
		var gitopsEngineCluster *db.GitopsEngineCluster
		var gitopsEngineInstance *db.GitopsEngineInstance

		BeforeEach(func() {
			ctx = context.Background()
			logger = log.FromContext(ctx)

			testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user",
				User_name:      "test-user",
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			scheme, argocdNamespace, kubesystemNamespace, workspace, err = tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			By("Initialize fake kube client")
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, argocdNamespace, kubesystemNamespace).Build()

			task = processOperationEventTask{
				log: logger,
				event: operationEventLoopEvent{
					request: newRequest(operationNamespace, operationName),
					client:  k8sClient,
				},
			}

			gitopsEngineCluster, _, err = dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(err).To(BeNil())

			By("creating a gitops engine instance")
			gitopsEngineInstance = &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-fake-engine-instance",
				Namespace_name:          argocdNamespace.Name,
				Namespace_uid:           string(argocdNamespace.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}
			err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
			Expect(err).To(BeNil())

		})

		It("Reconciling a deleted managed environment, to ensure the corresponding Argo CD cluster secret is deleted", func() {
			defer dbQueries.CloseDatabase()

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: string(uuid.NewUUID()),
			}

			err = dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			managedEnvironmentDB := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env",
				Name:                  "test-managed-env-name",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironmentDB)
			Expect(err).To(BeNil())

			applicationDB := db.Application{
				Application_id:          "test-application",
				Name:                    "test-application-name",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironmentDB.Managedenvironment_id,
			}
			err = dbQueries.CreateApplication(ctx, &applicationDB)
			Expect(err).To(BeNil())

			By("creating Operation row in database, pointing to Application")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             applicationDB.Application_id,
				Resource_type:           db.OperationResourceType_Application,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("creating Operation CR, pointing to Operation row")
			operationCR := &operation.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operationName,
					Namespace: operationNamespace,
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

			clusterSecretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: managedEnvironmentDB.Managedenvironment_id})
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterSecretName,
					Namespace: argocdNamespace.Name,
				},
			}
			err = task.event.client.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			Expect(err).To(BeNil(), "The Argo CD Cluster secret for the managed environment should exist")

			err = expectOperationIsComplete(ctx, operationDB.Operation_id, dbQueries)
			Expect(err).To(BeNil())
		})

		It("Reconciling a application pointing to a new managed environment, to ensure the corresponding Argo CD cluster secret is created", func() {
			defer dbQueries.CloseDatabase()

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: string(uuid.NewUUID()),
			}

			err = dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			managedEnvironmentDB := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env",
				Name:                  "test-managed-env-name",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironmentDB)
			Expect(err).To(BeNil())

			applicationDB := db.Application{
				Application_id:          "test-application",
				Name:                    "test-application-name",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironmentDB.Managedenvironment_id,
			}
			err = dbQueries.CreateApplication(ctx, &applicationDB)
			Expect(err).To(BeNil())

			By("creating Operation row in database")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             applicationDB.Application_id,
				Resource_type:           db.OperationResourceType_Application,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("creating Operation CR, pointing to Operation row")
			operationCR := &operation.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operationName,
					Namespace: operationNamespace,
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

			clusterSecretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: managedEnvironmentDB.Managedenvironment_id})
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterSecretName,
					Namespace: argocdNamespace.Name,
				},
			}
			err = task.event.client.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			Expect(err).To(BeNil(), "The Argo CD Cluster secret for the managed environment should exist")

			err = expectOperationIsComplete(ctx, operationDB.Operation_id, dbQueries)
			Expect(err).To(BeNil())
		})

		It("Reconciling a application pointing to a managed environment, with an out-of-date Argo CD cluster secret, to ensure the cluster secret is updated", func() {
			defer dbQueries.CloseDatabase()

			clusterCredentials := db.ClusterCredentials{
				Clustercredentials_cred_id: string(uuid.NewUUID()),
			}

			err = dbQueries.CreateClusterCredentials(ctx, &clusterCredentials)
			Expect(err).To(BeNil())

			managedEnvironmentDB := db.ManagedEnvironment{
				Managedenvironment_id: "test-managed-env",
				Name:                  "test-managed-env-name",
				Clustercredentials_id: clusterCredentials.Clustercredentials_cred_id,
			}
			err = dbQueries.CreateManagedEnvironment(ctx, &managedEnvironmentDB)
			Expect(err).To(BeNil())

			applicationDB := db.Application{
				Application_id:          "test-application",
				Name:                    "test-application-name",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironmentDB.Managedenvironment_id,
			}
			err = dbQueries.CreateApplication(ctx, &applicationDB)
			Expect(err).To(BeNil())

			By("creating Operation row in database, pointing to Application")
			operationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             applicationDB.Application_id,
				Resource_type:           db.OperationResourceType_Application,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}
			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("creating Operation CR, pointing to Operation row")
			operationCR := &operation.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operationName,
					Namespace: operationNamespace,
				},
				Spec: operation.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}
			err = task.event.client.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			By("creating an Argo CD Cluster secret, with out of date contents")
			clusterSecretName := argosharedutil.GenerateArgoCDClusterSecretName(db.ManagedEnvironment{Managedenvironment_id: managedEnvironmentDB.Managedenvironment_id})
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterSecretName,
					Namespace: argocdNamespace.Name,
					Labels: map[string]string{
						ArgoCDSecretTypeKey:                            ArgoCDSecretTypeValue_ClusterSecret,
						controllers.ArgoCDClusterSecretDatabaseIDLabel: applicationDB.Managed_environment_id,
					},
				},
				Data: map[string][]byte{
					"not-a-real-field": ([]byte)("not-a-real-value"),
				},
			}
			err = task.event.client.Create(ctx, secret)
			Expect(err).To(BeNil())

			By("calling perform task")
			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeFalse())
			err = expectOperationIsComplete(ctx, operationDB.Operation_id, dbQueries)
			Expect(err).To(BeNil())

			err = task.event.client.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			Expect(err).To(BeNil())

			By("examining the updated secret, and ensure it has been properly reconciled.")
			_, fieldStillExists := secret.Data["not-a-real-field"]
			Expect(fieldStillExists).To(BeFalse(), "the fake field we added to the secret should have been reconciled away")

			_, expectedFieldExists := secret.Data["config"]
			Expect(expectedFieldExists).To(BeTrue())

			_, expectedFieldExists = secret.Data["name"]
			Expect(expectedFieldExists).To(BeTrue())

			_, expectedFieldExists = secret.Data["server"]
			Expect(expectedFieldExists).To(BeTrue())

		})

	})
})

func expectOperationIsComplete(ctx context.Context, operationID string, dbQueries db.AllDatabaseQueries) error {
	operation := &db.Operation{
		Operation_id: operationID,
	}
	err := dbQueries.GetOperationById(ctx, operation)
	Expect(err).To(BeNil())
	Expect(operation.State).To(Equal(db.OperationState_Completed))

	return nil
}
