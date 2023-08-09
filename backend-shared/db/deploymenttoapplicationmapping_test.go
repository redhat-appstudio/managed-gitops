package db_test

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

func generateUuid() string {
	return uuid.New().String()
}

// Create entry for Application and DeploymentToApplicationMapping tables
func createAppAndDtamEntry(ctx context.Context, dbq db.AllDatabaseQueries, application *db.Application, deploymentToApplicationMapping *db.DeploymentToApplicationMapping) {
	application.Application_id = "test-app-" + generateUuid()
	application.Name = "test-app-" + generateUuid()
	err := dbq.CreateApplication(ctx, application)
	Expect(err).ToNot(HaveOccurred())

	deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id = "test-" + generateUuid()
	deploymentToApplicationMapping.Application_id = application.Application_id
	err = dbq.CreateDeploymentToApplicationMapping(ctx, deploymentToApplicationMapping)
	Expect(err).ToNot(HaveOccurred())
}

var _ = Describe("DeploymentToApplicationMapping Tests", func() {
	var ctx context.Context
	var dbq db.AllDatabaseQueries
	var application *db.Application
	var deploymentToApplicationMapping *db.DeploymentToApplicationMapping

	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()
		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		_, managedEnvironment, _, gitopsEngineInstance, _, err := db.CreateSampleData(dbq)
		Expect(err).ToNot(HaveOccurred())

		application = &db.Application{
			Spec_field:              "{}",
			Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id:  managedEnvironment.Managedenvironment_id,
		}

		deploymentToApplicationMapping = &db.DeploymentToApplicationMapping{
			DeploymentName:      "test-deployment",
			DeploymentNamespace: "test-namespace",
			NamespaceUID:        "demo-namespace",
		}

		createAppAndDtamEntry(ctx, dbq, application, deploymentToApplicationMapping)
	})

	Context("It should execute all DeploymentToApplicationMapping Functions", func() {

		It("Should execute all DeploymentToApplicationMapping Functions", func() {
			defer dbq.CloseDatabase()
			fetchRow := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
			}
			err := dbq.GetDeploymentToApplicationMappingByDeplId(ctx, fetchRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchRow).To(Equal(deploymentToApplicationMapping))

			rowsAffected, err := dbq.DeleteDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).To(Equal(1))
			fetchRow = &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
			}
			err = dbq.GetDeploymentToApplicationMappingByDeplId(ctx, fetchRow)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})

		It("Should Successfully Test ListAll Function", func() {
			defer dbq.CloseDatabase()
			var dbResults []db.DeploymentToApplicationMapping
			dbResult := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
			}

			err := dbq.ListDeploymentToApplicationMappingByNamespaceUID(ctx, "demo-namespace", &dbResults)
			Expect(err).ToNot(HaveOccurred())
			Expect(dbResults).Should(HaveLen(1))
			Expect(dbResults[0]).Should(Equal(*deploymentToApplicationMapping))

			err = dbq.ListDeploymentToApplicationMappingByNamespaceAndName(ctx, deploymentToApplicationMapping.DeploymentName, deploymentToApplicationMapping.DeploymentNamespace, deploymentToApplicationMapping.NamespaceUID, &dbResults)
			Expect(err).ToNot(HaveOccurred())
			Expect(dbResults[0]).Should(Equal(*deploymentToApplicationMapping))

			err = dbq.GetDeploymentToApplicationMappingByDeplId(ctx, dbResult)
			Expect(err).ToNot(HaveOccurred())
			Expect(dbResult).Should(Equal(deploymentToApplicationMapping))
			err = dbq.GetDeploymentToApplicationMappingByApplicationId(ctx, dbResult)
			Expect(err).ToNot(HaveOccurred())
			Expect(dbResult).Should(Equal(deploymentToApplicationMapping))

			rowsAffected, err := dbq.DeleteDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).Should(Equal(1))
			fetchRow := &db.DeploymentToApplicationMapping{
				Deploymenttoapplicationmapping_uid_id: deploymentToApplicationMapping.Deploymenttoapplicationmapping_uid_id,
			}
			err = dbq.GetDeploymentToApplicationMappingByDeplId(ctx, fetchRow)
			Expect(true).To(Equal(db.IsResultNotFoundError(err)))
		})

		It("Should Get DeploymentToApplicationMapping in batch.", func() {
			defer dbq.CloseDatabase()
			// Create multiple entries in table
			for i := 0; i < 6; i++ {
				createAppAndDtamEntry(ctx, dbq, application, deploymentToApplicationMapping)
			}

			// Fetch entries in batches
			var listOfDeploymentToApplicationMappingFromDB []db.DeploymentToApplicationMapping
			err := dbq.GetDeploymentToApplicationMappingBatch(ctx, &listOfDeploymentToApplicationMappingFromDB, 2, 0)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfDeploymentToApplicationMappingFromDB).To(HaveLen(2))

			err = dbq.GetDeploymentToApplicationMappingBatch(ctx, &listOfDeploymentToApplicationMappingFromDB, 3, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(listOfDeploymentToApplicationMappingFromDB).To(HaveLen(3))
		})
	})

	Context("Test Dispose function for DeploymentToApplicationMapping", func() {
		It("Should test Dispose function with missing database interface", func() {

			var dbq db.AllDatabaseQueries

			err := deploymentToApplicationMapping.Dispose(ctx, dbq)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing database interface in DeploymentToApplicationMapping dispose"))

		})

		It("Should test Dispose function for DeploymentToApplicationMapping", func() {

			err := deploymentToApplicationMapping.Dispose(context.Background(), dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetDeploymentToApplicationMappingByDeplId(ctx, deploymentToApplicationMapping)
			Expect(err).To(HaveOccurred())

		})
	})
})
