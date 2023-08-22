package db_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Application Test", func() {
	var seq = 101
	var ctx context.Context
	var dbq db.AllDatabaseQueries
	var applicationput db.Application
	var managedEnvironment *db.ManagedEnvironment
	var gitopsEngineInstance *db.GitopsEngineInstance

	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		ctx = context.Background()
		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		_, managedEnvironment, _, gitopsEngineInstance, _, err = db.CreateSampleData(dbq)
		Expect(err).ToNot(HaveOccurred())

		applicationput = db.Application{
			Application_id:          "test-my-application",
			Name:                    "my-application",
			Spec_field:              "{}",
			Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id:  managedEnvironment.Managedenvironment_id,
		}

		err = dbq.CreateApplication(ctx, &applicationput)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		dbq.CloseDatabase()
	})

	It("Should Create, Get, Update and Delete an Application", func() {
		applicationget := db.Application{
			Application_id: applicationput.Application_id,
		}

		err := dbq.GetApplicationById(ctx, &applicationget)
		Expect(err).ToNot(HaveOccurred())
		Expect(applicationput.Created_on.After(time.Now().Add(time.Minute*-5))).To(BeTrue(), "Created on should be within the last 5 minutes")
		applicationput.Created_on = applicationget.Created_on
		Expect(applicationput).Should(Equal(applicationget))

		applicationupdate := db.Application{

			Application_id:          applicationput.Application_id,
			Name:                    "test-application-update",
			Spec_field:              "{}",
			Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			SeqID:                   int64(seq),
			Created_on:              applicationget.Created_on,
		}

		err = dbq.UpdateApplication(ctx, &applicationupdate)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.GetApplicationById(ctx, &applicationupdate)
		Expect(err).ToNot(HaveOccurred())
		Expect(applicationupdate).ShouldNot(Equal(applicationget))

		rowsAffected, err := dbq.DeleteApplicationById(ctx, applicationput.Application_id)
		Expect(err).ToNot(HaveOccurred())
		Expect(rowsAffected).Should(Equal(1))

		err = dbq.GetApplicationById(ctx, &applicationput)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		applicationget = db.Application{
			Application_id: "does-not-exist",
		}
		err = dbq.GetApplicationById(ctx, &applicationget)
		Expect(true).To(Equal(db.IsResultNotFoundError(err)))

		applicationupdate.Name = strings.Repeat("abc", 100)
		err = dbq.UpdateApplication(ctx, &applicationupdate)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

		applicationput.Name = strings.Repeat("abc", 100)
		err = dbq.CreateApplication(ctx, &applicationput)
		Expect(true).To(Equal(db.IsMaxLengthError(err)))

	})

	It("Should Get Application in batch.", func() {

		By("Create multiple Application entries.")

		applicationput.Application_id = "test-my-application-2"
		err := dbq.CreateApplication(ctx, &applicationput)
		Expect(err).ToNot(HaveOccurred())

		applicationput.Application_id = "test-my-application-3"
		err = dbq.CreateApplication(ctx, &applicationput)
		Expect(err).ToNot(HaveOccurred())

		applicationput.Application_id = "test-my-application-4"
		err = dbq.CreateApplication(ctx, &applicationput)
		Expect(err).ToNot(HaveOccurred())

		applicationput.Application_id = "test-my-application-5"
		err = dbq.CreateApplication(ctx, &applicationput)
		Expect(err).ToNot(HaveOccurred())

		By("Get data in batch.")

		var listOfApplicationsFromDB []db.Application
		err = dbq.GetApplicationBatch(ctx, &listOfApplicationsFromDB, 2, 0)
		Expect(err).ToNot(HaveOccurred())
		Expect(listOfApplicationsFromDB).To(HaveLen(2))

		err = dbq.GetApplicationBatch(ctx, &listOfApplicationsFromDB, 3, 1)
		Expect(err).ToNot(HaveOccurred())
		Expect(listOfApplicationsFromDB).To(HaveLen(3))
	})

	Context("Test DisposeAppScoped function for Application", func() {
		It("Should test DisposeAppScoped function with missing database interface for Application", func() {

			var dbq db.AllDatabaseQueries

			err := applicationput.DisposeAppScoped(ctx, dbq)
			Expect(err).To(HaveOccurred())

		})

		It("Should test DisposeAppScoped function for Application", func() {

			err := applicationput.DisposeAppScoped(ctx, dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetApplicationById(ctx, &applicationput)
			Expect(err).To(HaveOccurred())
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

		})
	})

	Context("Test ListApplicationsForManagedEnvironment function", func() {
		It("should list Applications for a given ManagedEnvironment", func() {
			app := &db.Application{
				Application_id:          "test-my-application",
				Name:                    "my-application",
				Spec_field:              "{}",
				Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
				Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			}

			expectedRows := 4
			for i := 1; i <= expectedRows; i++ {
				app.Application_id = fmt.Sprintf("test-app-%d", i)
				err := dbq.CreateApplication(ctx, app)
				Expect(err).ToNot(HaveOccurred())
			}

			apps := []db.Application{}
			count, err := dbq.ListApplicationsForManagedEnvironment(ctx, managedEnvironment.Managedenvironment_id, &apps)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(Equal(expectedRows))
		})

		It("should return an error if the ManagedEnvironment ID is empty", func() {
			apps := []db.Application{}
			count, err := dbq.ListApplicationsForManagedEnvironment(ctx, "", &apps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("primary key is empty"))
			Expect(count).To(BeZero())
		})

		It("should return an empty slice if there are no Applications for a given Environment", func() {
			apps := []db.Application{}
			count, err := dbq.ListApplicationsForManagedEnvironment(ctx, "invalid-id", &apps)
			Expect(err).ToNot(HaveOccurred())
			Expect(count).To(BeZero())
			Expect(apps).To(BeEmpty())
		})
	})
})
