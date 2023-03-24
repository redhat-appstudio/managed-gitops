package eventloop

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	dbutil "github.com/redhat-appstudio/managed-gitops/backend-shared/db/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/redhat-appstudio/managed-gitops/cluster-agent/metrics"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Test for Operation metrics counter", func() {
	Context("Prometheus metrics responds to number of operations CR on a cluster", func() {

		var ctx context.Context
		var dbq db.AllDatabaseQueries
		var k8sClient client.WithWatch
		var gitopsEngineInstance *db.GitopsEngineInstance
		var testClusterUser *db.ClusterUser
		var managedEnvronment *db.ManagedEnvironment
		var log logr.Logger

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				apiNamespace,
				err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			testClusterUser = &db.ClusterUser{
				Clusteruser_id: "test-user",
				User_name:      "test-user",
			}

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(apiNamespace, argocdNamespace, kubesystemNamespace).
				Build()

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx = context.Background()
			log = logger.FromContext(ctx)
			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, managedEnvronment, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

		})

		It("List operations CR on a cluster", func() {
			defer dbq.CloseDatabase()

			metrics.ClearOperationMetrics()

			numberOfOperationsCR := testutil.ToFloat64(metrics.OperationCR)

			By("creating first Operation row")
			firstOperationDB := &db.Operation{
				Operation_id:            "test-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             managedEnvronment.Managedenvironment_id,
				Resource_type:           db.OperationResourceType_ManagedEnvironment,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err := dbq.CreateOperation(ctx, firstOperationDB, firstOperationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("creating Operation CR pointing to first Operation row")
			firstOperationCR := &operation.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-operation",
					Namespace: dbutil.GetGitOpsEngineSingleInstanceNamespace(),
				},
				Spec: operation.OperationSpec{
					OperationID: firstOperationDB.Operation_id,
				},
			}
			err = k8sClient.Create(ctx, firstOperationCR)
			Expect(err).To(BeNil())

			By("creating second Operation row")
			secondOperationDB := &db.Operation{
				Operation_id:            "test-operation-1",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             managedEnvronment.Managedenvironment_id,
				Resource_type:           db.OperationResourceType_ManagedEnvironment,
				State:                   db.OperationState_Waiting,
				Operation_owner_user_id: testClusterUser.Clusteruser_id,
			}

			err = dbq.CreateOperation(ctx, secondOperationDB, secondOperationDB.Operation_owner_user_id)
			Expect(err).To(BeNil())

			By("creating Operation CR pointing to second Operation row")
			secondOperationCR := &operation.Operation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-operation-1",
					Namespace: dbutil.GetGitOpsEngineSingleInstanceNamespace(),
				},
				Spec: operation.OperationSpec{
					OperationID: secondOperationDB.Operation_id,
				},
			}
			err = k8sClient.Create(ctx, secondOperationCR)
			Expect(err).To(BeNil())

			updateOperationCRMetrics(ctx, k8sClient, log)

			newNumberOfOperationsCR := testutil.ToFloat64(metrics.OperationCR)

			Expect(newNumberOfOperationsCR).To(Equal(numberOfOperationsCR + 2))

		})

	})
})
