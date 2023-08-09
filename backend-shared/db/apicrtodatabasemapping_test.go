package db_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	db "github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

var _ = Describe("Apicrtodatabasemapping Tests", func() {
	var item db.APICRToDatabaseMapping
	var dbq db.AllDatabaseQueries
	var ctx context.Context
	var err error

	BeforeEach(func() {
		err := db.SetupForTestingDBGinkgo()
		Expect(err).ToNot(HaveOccurred())

		item = db.APICRToDatabaseMapping{
			APIResourceType:      db.APICRToDatabaseMapping_ResourceType_GitOpsDeploymentSyncRun,
			APIResourceUID:       "test-k8s-uid",
			APIResourceName:      "test-k8s-name",
			APIResourceNamespace: "test-k8s-namespace",
			NamespaceUID:         "test-namespace-uid",
			DBRelationType:       db.APICRToDatabaseMapping_DBRelationType_SyncOperation,
			DBRelationKey:        "test-key",
		}
		ctx = context.Background()
		dbq, err = db.NewUnsafePostgresDBQueries(true, true)
		Expect(err).ToNot(HaveOccurred())

		err = dbq.CreateAPICRToDatabaseMapping(ctx, &item)
		Expect(err).ToNot(HaveOccurred())

	})

	AfterEach(func() {
		defer dbq.CloseDatabase()
	})
	Context("Tests all the DB functions for Apicrtodatabasemapping", func() {
		It("Should execute all Apicrtodatabasemapping Functions", func() {

			fetchRow := db.APICRToDatabaseMapping{
				APIResourceType: item.APIResourceType,
				APIResourceUID:  item.APIResourceUID,
				DBRelationKey:   item.DBRelationKey,
				DBRelationType:  item.DBRelationType,
			}

			err = dbq.GetDatabaseMappingForAPICR(ctx, &fetchRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchRow).Should(Equal(item))

			var items []db.APICRToDatabaseMapping

			err = dbq.ListAPICRToDatabaseMappingByAPINamespaceAndName(ctx, item.APIResourceType, item.APIResourceName, item.APIResourceNamespace, item.NamespaceUID, item.DBRelationType, &items)
			Expect(err).ToNot(HaveOccurred())
			Expect(items[0]).Should(Equal(item))

			rowsAffected, err := dbq.DeleteAPICRToDatabaseMapping(ctx, &fetchRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(rowsAffected).To(Equal((1)))
			fetchRow = db.APICRToDatabaseMapping{
				APIResourceType: item.APIResourceType,
				APIResourceUID:  item.APIResourceUID,
				DBRelationKey:   item.DBRelationKey,
				DBRelationType:  item.DBRelationType,
			}
			err = dbq.GetDatabaseMappingForAPICR(ctx, &fetchRow)
			Expect(db.IsResultNotFoundError(err)).To(BeTrue())

			// Set the invalid value
			item.APIResourceName = strings.Repeat("abc", 100)
			err = dbq.CreateAPICRToDatabaseMapping(ctx, &item)
			Expect(db.IsMaxLengthError(err)).To(BeTrue())

		})
	})

	Context("Test DisposeAppScoped function for Apicrtodatabasemapping", func() {
		It("Should test DisposeAppScoped function with missing database interface for Apicrtodatabasemapping", func() {

			var dbq db.AllDatabaseQueries

			err := item.DisposeAppScoped(ctx, dbq)
			Expect(err).To(HaveOccurred())

		})

		It("Should test DisposeAppScoped function for Apicrtodatabasemapping", func() {

			err := item.DisposeAppScoped(ctx, dbq)
			Expect(err).ToNot(HaveOccurred())

			err = dbq.GetAPICRForDatabaseUID(ctx, &item)
			Expect(err).To(HaveOccurred())

		})
	})
})
