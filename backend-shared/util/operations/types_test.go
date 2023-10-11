package operations

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operation "github.com/redhat-appstudio/managed-gitops/backend-shared/apis/managed-gitops/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
	sharedutil "github.com/redhat-appstudio/managed-gitops/backend-shared/util"
	"github.com/redhat-appstudio/managed-gitops/backend-shared/util/tests"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Testing CreateOperation function", func() {
	Context("Testing CreateOperation function", func() {
		var ctx context.Context
		var dbq db.AllDatabaseQueries
		var dbOperationFirst *db.Operation
		var k8sOperationFirst *operation.Operation
		var dbOperationSecond *db.Operation
		var k8sOperationSecond *operation.Operation

		AfterEach(func() {
			defer dbq.CloseDatabase()

			rowsAffected, err := dbq.DeleteOperationById(ctx, dbOperationFirst.Operation_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))
		})

		It("It should create new Operation in DB and Cluster and if existing Operation in not in Completed/Failed state, it should return it, instead of creating new.", func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()
			log := log.FromContext(ctx)

			k8sClientOuter := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, argocdNamespace, kubesystemNamespace).
				Build()

			k8sClient := &sharedutil.ProxyClient{
				InnerClient: k8sClientOuter,
			}

			dbq, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			applicationput := db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationput)
			Expect(err).ToNot(HaveOccurred())

			dbOperationInput := db.Operation{
				Instance_id:   applicationput.Engine_instance_inst_id,
				Resource_id:   applicationput.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			// Create new Operation
			k8sOperationFirst, dbOperationFirst, err = CreateOperation(ctx, false, dbOperationInput, "test-user", gitopsEngineInstance.Namespace_name, dbq, k8sClient, log)
			Expect(err).ToNot(HaveOccurred())
			Expect(k8sOperationFirst).NotTo(BeNil())
			Expect(dbOperationFirst).NotTo(BeNil())

			// Try to recreate same Operation it should return existing one.
			k8sOperationSecond, dbOperationSecond, err = CreateOperation(ctx, false, dbOperationInput, "test-user", gitopsEngineInstance.Namespace_name, dbq, k8sClient, log)
			Expect(err).ToNot(HaveOccurred())
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

	Context("Testing waitForOperationToComplete and IsOperationComplete function.", func() {
		var ctx context.Context
		var dbq db.AllDatabaseQueries
		var k8sClient client.Client
		var logger logr.Logger

		simulateClusterAgent := func(operation *db.Operation, k8sClient client.Client, gitopsEngineInstance db.GitopsEngineInstance) {
			defer GinkgoRecover()

			Eventually(func() bool {
				operationget := db.Operation{
					Operation_id: operation.Operation_id,
				}

				err := dbq.GetOperationById(ctx, &operationget)
				// The operation might not be created yet
				if err != nil {
					return false
				}

				if operationget.State != db.OperationState_Waiting {
					return false
				}

				// Update the operation state to Completed
				operationget.State = db.OperationState_Completed
				err = dbq.UpdateOperation(ctx, &operationget)
				if err != nil {
					return false
				}

				// Get the operation again and check state
				err = dbq.GetOperationById(ctx, &operationget)
				if err != nil {
					return false
				}
				if operationget.State != db.OperationState_Completed {
					return false
				}

				return true
			}, "5s", "10ms").Should(BeTrue())
		}

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()
			logger = log.FromContext(ctx)

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, argocdNamespace, kubesystemNamespace).
				Build()

			dbq, err = db.NewUnsafePostgresDBQueries(false, true)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			dbq.CloseDatabase()
		})

		It("should create a new DB Operation and test the waitForOperationToComplete function.", func() {
			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			By("define a sample operation")
			sampleOperation := &db.Operation{
				Operation_id:            "test-engine-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				Operation_owner_user_id: "test-user",
			}

			// Create the Operation which replaces the CreateOperation step in types' CreateOperation function
			err = dbq.CreateOperation(ctx, sampleOperation, sampleOperation.Operation_owner_user_id)
			Expect(err).ToNot(HaveOccurred())

			go func() {
				simulateClusterAgent(sampleOperation, k8sClient, *gitopsEngineInstance)
			}()

			// Now directly call waitForOperationToComplete which calls isOperationComplete to test these functions
			err = waitForOperationToComplete(ctx, sampleOperation, dbq, logger)
			Expect(err).ToNot(HaveOccurred())

			rowsAffected, err := dbq.DeleteOperationById(ctx, sampleOperation.Operation_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

		})

		It("should pass a non-existent DB Operation to isOperationComplete, to test error handling.", func() {
			_, _, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			By("define a sample operation")
			sampleOperation := &db.Operation{
				Operation_id:            "test-engine-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				Operation_owner_user_id: "test-user",
			}

			// Now directly call isOperationComplete to test the error handling
			isComplete, err := IsOperationComplete(ctx, sampleOperation, dbq)
			Expect(isComplete).To(BeFalse())
			Expect(err).To(HaveOccurred())

		})

		It("should test CreateOperation with waitForOperation set to true to indirectly test the waitForOperationToComplete function.", func() {
			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			applicationInput := db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &applicationInput)
			Expect(err).ToNot(HaveOccurred())

			By("define two sample operations to pass to CreateOperation")
			sampleGitopsEngineTypeOperation := &db.Operation{
				Operation_id:            "test-engine-operation",
				Instance_id:             gitopsEngineInstance.Gitopsengineinstance_id,
				Resource_id:             "test-fake-resource-id",
				Resource_type:           "GitopsEngineInstance",
				Operation_owner_user_id: "test-user",
			}

			sampleApplicationTypeOperation := &db.Operation{
				Operation_id:            "test-application-operation",
				Instance_id:             applicationInput.Engine_instance_inst_id,
				Resource_id:             applicationInput.Application_id,
				Resource_type:           db.OperationResourceType_Application,
				Operation_owner_user_id: "test-user",
			}

			go func() {
				simulateClusterAgent(sampleApplicationTypeOperation, k8sClient, *gitopsEngineInstance)
			}()

			// Directly call CreateOperation with waitForOperation set to true, to create the new Operation
			k8sOperationFirst, dbOperationFirst, err := doCreateOperation(ctx, true, *sampleApplicationTypeOperation, "test-user", gitopsEngineInstance.Namespace_name, dbq, k8sClient, logger, "test-application-operation")
			Expect(err).ToNot(HaveOccurred())
			Expect(k8sOperationFirst).NotTo(BeNil())
			Expect(dbOperationFirst).NotTo(BeNil())

			go func() {
				simulateClusterAgent(sampleGitopsEngineTypeOperation, k8sClient, *gitopsEngineInstance)
			}()

			k8sOperationSecond, dbOperationSecond, err := doCreateOperation(ctx, true, *sampleGitopsEngineTypeOperation, "test-user", gitopsEngineInstance.Namespace_name, dbq, k8sClient, logger, "test-engine-operation")
			Expect(err).ToNot(HaveOccurred())
			Expect(k8sOperationSecond).NotTo(BeNil())
			Expect(dbOperationSecond).NotTo(BeNil())

			rowsAffected, err := dbq.DeleteOperationById(ctx, sampleApplicationTypeOperation.Operation_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

			rowsAffected, err = dbq.DeleteOperationById(ctx, sampleGitopsEngineTypeOperation.Operation_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))

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

var _ = Describe("Testing CleanupOperation function", func() {
	Context("CleanupOperation should remove Operation CR and DB entry", func() {

		var (
			ctx          context.Context
			dbq          db.AllDatabaseQueries
			logr         logr.Logger
			k8sClient    client.Client
			k8sOperation *operation.Operation
			dbOperation  *db.Operation
		)

		BeforeEach(func() {
			scheme,
				argocdNamespace,
				kubesystemNamespace,
				workspace,
				err := tests.GenericTestSetup()
			Expect(err).ToNot(HaveOccurred())

			err = db.SetupForTestingDBGinkgo()
			Expect(err).ToNot(HaveOccurred())

			ctx = context.Background()
			logr = log.FromContext(ctx)

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, argocdNamespace, kubesystemNamespace).
				Build()

			dbq, err = db.NewUnsafePostgresDBQueries(true, true)
			Expect(err).ToNot(HaveOccurred())

			_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
			Expect(err).ToNot(HaveOccurred())

			app := db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			err = dbq.CreateApplication(ctx, &app)
			Expect(err).ToNot(HaveOccurred())

			dbOperationInput := db.Operation{
				Instance_id:   app.Engine_instance_inst_id,
				Resource_id:   app.Application_id,
				Resource_type: db.OperationResourceType_Application,
			}

			// Create new Operation
			k8sOperation, dbOperation, err = CreateOperation(ctx, false, dbOperationInput, "test-user", gitopsEngineInstance.Namespace_name, dbq, k8sClient, logr)
			Expect(err).ToNot(HaveOccurred())
			Expect(k8sOperation).NotTo(BeNil())
			Expect(dbOperation).NotTo(BeNil())
		})

		It("should remove CR and DB entry", func() {
			err := CleanupOperation(ctx, *dbOperation, *k8sOperation, dbq, k8sClient, true, logr)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetOperationById(ctx, dbOperation)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOperation), k8sOperation)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("shouldn't remove DB entry when deleteDBOperation is disabled", func() {
			err := CleanupOperation(ctx, *dbOperation, *k8sOperation, dbq, k8sClient, false, logr)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetOperationById(ctx, dbOperation)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(k8sOperation), k8sOperation)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})
})
