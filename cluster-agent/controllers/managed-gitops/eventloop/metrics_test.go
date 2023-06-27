package eventloop

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	managedgitopsv1alpha1 "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"

	argocdoperatorv1alph1 "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/metrics"
)

var _ = Describe("OperationDB Metrics Controller", func() {
	const (
		name      = "operation"
		namespace = "argocd"
	)
	Context("OperationDB Metrics Test", func() {

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
		var operationDB *db.Operation
		var gitopsEngineInstance *db.GitopsEngineInstance
		var err error

		BeforeEach(func() {
			ctx = context.Background()
			logger = log.FromContext(ctx)

			testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user",
				User_name:      "test-user",
			}

			dbQueries, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).To(BeNil())

			scheme, argocdNamespace, kubesystemNamespace, workspace, err = tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = appv1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			err = argocdoperatorv1alph1.AddToScheme(scheme)
			Expect(err).To(BeNil())

			gitopsDepl := &managedgitopsv1alpha1.GitOpsDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gitops-depl",
					Namespace: workspace.Name,
					UID:       uuid.NewUUID(),
				},
			}

			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(gitopsDepl, workspace, argocdNamespace, kubesystemNamespace).Build()

			namespaceToCreate := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
					UID:  workspace.UID,
				},
			}
			err = k8sClient.Create(ctx, namespaceToCreate)
			Expect(err).To(BeNil())

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			gitopsEngineCluster, _, err := dbutil.GetOrCreateGitopsEngineClusterByKubeSystemNamespaceUID(ctx, string(kubesystemNamespace.UID), dbQueries, logger)
			Expect(gitopsEngineCluster).ToNot(BeNil())
			Expect(err).To(BeNil())

			By("creating a gitops engine instance")
			gitopsEngineInstance = &db.GitopsEngineInstance{
				Gitopsengineinstance_id: "test-engine-instance",
				Namespace_name:          namespace,
				Namespace_uid:           string(workspace.UID),
				EngineCluster_id:        gitopsEngineCluster.Gitopsenginecluster_id,
			}

			err = dbQueries.CreateGitopsEngineInstance(ctx, gitopsEngineInstance)
			Expect(err).To(BeNil())

			By("Creating new operation row in database")
			operationDB = &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-application",
				Resource_type:           "Application",
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbQueries.CreateOperation(ctx, operationDB, operationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			metrics.TestOnly_resetAllMetricsCount()

			task = processOperationEventTask{
				log: logger,
				event: operationEventLoopEvent{
					request: newRequest(namespace, name),
					client:  k8sClient,
				},
			}

		})
		It("should count total number of operation DB rows in completed state", func() {
			defer dbQueries.CloseDatabase()

			appProject := &appv1.AppProject{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AppProject",
					APIVersion: "argoproj.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      appProjectPrefix + testClusterUser.Clusteruser_id,
					Namespace: namespace,
				},
				Spec: appv1.AppProjectSpec{
					SourceRepos: []string{"test-url"},
				},
			}

			err = task.event.client.Create(ctx, appProject)
			Expect(err).To(BeNil())

			By("creating Operation CR")
			operationCR := &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}

			err = task.event.client.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			retry, err := task.PerformTask(ctx)
			Expect(err).To(BeNil())
			Expect(retry).To(BeFalse())

			metrics.TestOnly_runCollectOperationMetrics()

			Expect(testutil.ToFloat64(metrics.OperationStateCompleted)).To(Equal(float64(1)))
			Expect(testutil.ToFloat64(metrics.OperationStateFailed)).To(Equal(float64(0)))
		})

		It("should count total number of operation DB rows in failed state", func() {
			defer dbQueries.CloseDatabase()

			By("fetch operation DB by ID to update the operation DB row as required by test")
			err = dbQueries.GetOperationById(ctx, operationDB)
			Expect(err).To(BeNil())

			By("updating operation row to Resource_type SyncOperation in the database")
			operationDB.Resource_type = db.OperationResourceType_SyncOperation
			err = dbQueries.UpdateOperation(ctx, operationDB)
			Expect(err).To(BeNil())

			By("creating Operation CR")
			operationCR := &managedgitopsv1alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: managedgitopsv1alpha1.OperationSpec{
					OperationID: operationDB.Operation_id,
				},
			}

			err = task.event.client.Create(ctx, operationCR)
			Expect(err).To(BeNil())

			By("error is not nil because syncOperation is not created")
			retry, err := task.PerformTask(ctx)
			Expect(err).ToNot(BeNil())
			Expect(retry).To(BeFalse())

			metrics.TestOnly_runCollectOperationMetrics()

			Expect(testutil.ToFloat64(metrics.OperationStateFailed)).To(Equal(float64(1)))
			Expect(testutil.ToFloat64(metrics.OperationStateCompleted)).To(Equal(float64(0)))

		})

	})
})
