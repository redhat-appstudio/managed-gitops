package operations

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
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

// Test the GetOperatorCRName function with different possible values of db.Operation
var _ = Describe("Testing GenerateOperatorCRName function", func() {
	Context("Testing GenerateOperatorCRName function", func() {
		When("GenerateOperatorCRName is invoked with a db.Operation having a valid operation id 1", func() {
			It("should return a CR name of value operation-1", func() {
				Expect(GenerateOperationCRName(db.Operation{Operation_id: "1"})).To(Equal("operation-1"))
			})
		})
		When("GenerateOperatorCRName is invoked with a db.Operation having an empty operation id", func() {
			It("should return a CR name of value operation-", func() {
				Expect(GenerateOperationCRName(db.Operation{Operation_id: ""})).To(Equal("operation-"))
			})
		})
		When("GenerateOperatorCRName is invoked with a zero value of db.Operation", func() {
			It("should return a CR name of value operation-", func() {
				Expect(GenerateOperationCRName(db.Operation{})).To(Equal("operation-"))
			})
		})
	})
})

// Test the generateUniqueOperationCRName function with a custom unique ID generator function.
var _ = Describe("Testing generateUniqueOperationCRName function", func() {
	Context("Testing generateUniqueOperationCRName function", func() {
		When("generateUniqueOperationCRName is invoked with a custom unique id generator function", func() {
			It("should return a CR name of value operation-customtestid", func() {
				crName := generateUniqueOperationCRName(db.Operation{Operation_id: "1"}, func(db.Operation) string {
					return "customtestid"
				})
				Expect(crName).To(Equal("operation-customtestid"))

			})
		})
	})
})
