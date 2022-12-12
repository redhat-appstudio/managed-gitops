package operations

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Testing CreateOperation function.", func() {
	Context("Testing CreateOperation function.", func() {
		var ctx context.Context
		var dbq db.AllDatabaseQueries
		var dbOperationFirst *db.Operation
		var k8sOperationFirst *operation.Operation
		var dbOperationSecond *db.Operation
		var k8sOperationSecond *operation.Operation

		AfterEach(func() {
			defer dbq.CloseDatabase()

			rowsAffected, err := dbq.DeleteOperationById(ctx, dbOperationFirst.Operation_id)
			Expect(err).To(BeNil())
			Expect(rowsAffected).Should(Equal(1))
		})

		It("It should create new Operation in DB and Cluster and if existing Operation in not in Completed/Failed state, it should return it, instead of creating new.", func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err := tests.GenericTestSetup()
			Expect(err).To(BeNil())

			err = db.SetupForTestingDBGinkgo()
			Expect(err).To(BeNil())

			ctx = context.Background()
			log := log.FromContext(ctx)

			k8sClientOuter := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, argocdNamespace, kubesystemNamespace).
				Build()

			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).To(BeNil())

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).To(BeNil())

			applicationput := db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationput)
			Expect(err).To(BeNil())

			dbOperationInput := db.Operation{
				Instance_id:   applicationput.Engine_instance_inst_id,
				Resource_id:   applicationput.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			// Create new Operation
			k8sOperationFirst, dbOperationFirst, err = CreateOperation(ctx, false, dbOperationInput, "test-user", gitopsEngineInstance.Namespace_name, dbq, k8sClient, log)
			Expect(err).To(BeNil())
			Expect(k8sOperationFirst).NotTo(BeNil())
			Expect(dbOperationFirst).NotTo(BeNil())

			// Try to recreate same Operation it should return existing one.
			k8sOperationSecond, dbOperationSecond, err = CreateOperation(ctx, false, dbOperationInput, "test-user", gitopsEngineInstance.Namespace_name, dbq, k8sClient, log)
			Expect(err).To(BeNil())
			Expect(k8sOperationSecond).NotTo(BeNil())
			Expect(dbOperationSecond).NotTo(BeNil())

			// Verify existing Operation is returned.
			Expect(k8sOperationFirst.Name).To(Equal(k8sOperationSecond.Name))
			Expect(k8sOperationFirst.Namespace).To(Equal(k8sOperationSecond.Namespace))
			Expect(k8sOperationFirst.Spec.OperationID).To(Equal(k8sOperationSecond.Spec.OperationID))

			Expect(dbOperationFirst.Operation_id).To(Equal(dbOperationSecond.Operation_id))
			Expect(dbOperationFirst.Instance_id).To(Equal(dbOperationSecond.Instance_id))
			Expect(dbOperationFirst.Resource_id).To(Equal(dbOperationSecond.Resource_id))
			Expect(dbOperationFirst.SeqID).To(Equal(dbOperationSecond.SeqID))
		})
	})
})

// TestGenerateOperatorCRName tests the GetOperatorCRName function
// with different possible db.Operation
func TestGenerateOperatorCRName(t *testing.T) {
	tests := []struct {
		name           string
		dbOperation    db.Operation
		expectedString string
	}{
		{
			"test with a db.Operation having a valid operation id",
			db.Operation{Operation_id: "1"},
			"operation-1",
		},
		{
			"test with a db.Operation containing an empty operation id",
			db.Operation{Operation_id: ""},
			"operation-",
		},
		{
			"test with a zero value of db.Operation",
			db.Operation{},
			"operation-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testOperatorCRName := GenerateOperationCRName(tt.dbOperation)
			assert.Equal(t, tt.expectedString, testOperatorCRName)
		})
	}

}

func TestGenerateUniqueOperatorCRName(t *testing.T) {
	t.Run("with a custom unique id generator function", func(t *testing.T) {
		crName := generateUniqueOperationCRName(db.Operation{Operation_id: "1"}, func(db.Operation) string {
			return "customtestid"
		})
		assert.Equal(t, "operation-customtestid", crName)
	})

}
