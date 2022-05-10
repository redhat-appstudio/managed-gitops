package db_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	db "github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
)

var _ = Describe("Application Test", func() {
	var seq = 101
	It("Should Create, Get, Update and Delete an Application", func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).To(BeNil())

		ctx := context.Background()
		dbq, err := db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).To(BeNil())
		defer dbq.CloseDatabase()

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

		applicationget := db.Application{
			Application_id: applicationput.Application_id,
		}

		err = dbq.GetApplicationById(ctx, &applicationget)
		Expect(err).To(BeNil())
		Expect(applicationput).Should(Equal(applicationget))

		applicationupdate := db.Application{

			Application_id:          applicationput.Application_id,
			Name:                    "test-application-update",
			Spec_field:              "{}",
			Engine_instance_inst_id: gitopsEngineInstance.Gitopsengineinstance_id,
			Managed_environment_id:  managedEnvironment.Managedenvironment_id,
			SeqID:                   int64(seq),
		}

		err = dbq.UpdateApplication(ctx, &applicationupdate)
		Expect(err).To(BeNil())

		err = dbq.GetApplicationById(ctx, &applicationupdate)
		Expect(err).To(BeNil())
		Expect(applicationupdate).ShouldNot(Equal(applicationget))

		rowsAffected, err := dbq.DeleteApplicationById(ctx, applicationput.Application_id)
		Expect(err).To(BeNil())
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
})
